//go:build integration

package storefront_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"log/slog"
	"os"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"golang.org/x/sync/singleflight"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/handlers/storefront"
	"github.com/mark8ly/marketplace-api/internal/stockhold"
	"github.com/mark8ly/marketplace-api/internal/stores"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

// #232 — the storefront's server-side hold surface.
//
// The cart was pure client-side state, so there was no server identity to
// attach a hold to (#229). cart_token is that identity: minted by the server,
// carried by the browser as an httpOnly cookie.
//
// Decision recorded 2026-08-30: holds are placed AT CART-ADD for the full 15
// minutes. That is the epic's literal intent — the shopper who adds first
// keeps the unit. The cost is accepted: an abandoned tab parks the last unit
// for 15 minutes, and abandoned carts are the norm.

func postHolds(t *testing.T, r *http.Handler, slug, key, cartToken string, items []map[string]any) *httptest.ResponseRecorder {
	t.Helper()
	body := map[string]any{"items": items}
	if cartToken != "" {
		body["cart_token"] = cartToken
	}
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost,
		fmt.Sprintf("/api/v1/storefront/stores/%s/cart/holds", slug), bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Storefront-Key", key)
	rec := httptest.NewRecorder()
	(*r).ServeHTTP(rec, req)
	return rec
}

type holdsResponse struct {
	Data struct {
		CartToken string `json:"cart_token"`
		ExpiresAt string `json:"expires_at"`
		Items     []struct {
			VariantID string `json:"variant_id"`
			Status    string `json:"status"`
			Available int    `json:"available"`
		} `json:"items"`
	} `json:"data"`
}

func TestCartHolds_PlacesAHoldAndMintsACartToken(t *testing.T) {
	router, db, store, key := setupHoldsRouter(t)
	variantID := seedHoldVariant(t, db, store, 5)

	rec := postHolds(t, &router, store.Slug, key, "", []map[string]any{
		{"variant_id": variantID, "quantity": 2},
	})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	var body holdsResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.NotEmpty(t, body.Data.CartToken, "the server mints the cart identity")
	require.Len(t, body.Data.Items, 1)
	require.Equal(t, "held", body.Data.Items[0].Status)

	// The token must come back as an httpOnly cookie: it identifies a
	// reservation, so script access buys nothing and costs XSS exposure.
	var found bool
	for _, ck := range rec.Result().Cookies() {
		if ck.Name == "mk_cart_token" {
			found = true
			require.True(t, ck.HttpOnly, "cart_token must be httpOnly")
			require.Equal(t, body.Data.CartToken, ck.Value)
		}
	}
	require.True(t, found, "cart_token cookie must be set")

	var held int
	require.NoError(t, db.Raw(
		`SELECT COALESCE(SUM(qty),0) FROM stock_holds WHERE variant_id = ? AND state = 'held'`,
		variantID).Scan(&held).Error)
	require.Equal(t, 2, held)
}

// Per-item status, not all-or-nothing: a cart of five items where one is
// short must tell the shopper WHICH one, not fail opaquely.
func TestCartHolds_ReportsPerItemInsufficientWithoutFailingTheRequest(t *testing.T) {
	router, db, store, key := setupHoldsRouter(t)
	plenty := seedHoldVariant(t, db, store, 10)
	scarce := seedHoldVariant(t, db, store, 1)

	rec := postHolds(t, &router, store.Slug, key, "", []map[string]any{
		{"variant_id": plenty, "quantity": 2},
		{"variant_id": scarce, "quantity": 5},
	})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	var body holdsResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	byVariant := map[string]string{}
	avail := map[string]int{}
	for _, it := range body.Data.Items {
		byVariant[it.VariantID] = it.Status
		avail[it.VariantID] = it.Available
	}
	require.Equal(t, "held", byVariant[plenty])
	require.Equal(t, "insufficient", byVariant[scarce])
	require.Equal(t, 1, avail[scarce], "the shopper is told how many are actually left")

	// The obtainable item is still held — a short line must not cost the
	// customer the rest of their cart.
	var heldPlenty int
	require.NoError(t, db.Raw(
		`SELECT COALESCE(SUM(qty),0) FROM stock_holds WHERE variant_id = ? AND state = 'held'`,
		plenty).Scan(&heldPlenty).Error)
	require.Equal(t, 2, heldPlenty)
}

// Re-posting the same cart refreshes rather than stacking, so a shopper
// adjusting quantities does not consume their own stock twice.
func TestCartHolds_RepostRefreshesTheSameCart(t *testing.T) {
	router, db, store, key := setupHoldsRouter(t)
	variantID := seedHoldVariant(t, db, store, 3)

	first := postHolds(t, &router, store.Slug, key, "", []map[string]any{
		{"variant_id": variantID, "quantity": 1},
	})
	require.Equal(t, http.StatusOK, first.Code)
	var body holdsResponse
	require.NoError(t, json.Unmarshal(first.Body.Bytes(), &body))
	cart := body.Data.CartToken

	second := postHolds(t, &router, store.Slug, key, cart, []map[string]any{
		{"variant_id": variantID, "quantity": 3},
	})
	require.Equal(t, http.StatusOK, second.Code, second.Body.String())

	var rows, total int
	require.NoError(t, db.Raw(`SELECT count(*), COALESCE(SUM(qty),0) FROM stock_holds
	                            WHERE cart_token = ? AND state = 'held'`, cart).Row().Scan(&rows, &total))
	require.Equal(t, 1, rows, "one row per (cart, variant), refreshed")
	require.Equal(t, 3, total, "quantity is replaced, not added to")
}

func TestCartHolds_ReleaseReturnsStockToThePool(t *testing.T) {
	router, db, store, key := setupHoldsRouter(t)
	variantID := seedHoldVariant(t, db, store, 1)

	rec := postHolds(t, &router, store.Slug, key, "", []map[string]any{
		{"variant_id": variantID, "quantity": 1},
	})
	require.Equal(t, http.StatusOK, rec.Code)
	var body holdsResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))

	req := httptest.NewRequest(http.MethodDelete,
		fmt.Sprintf("/api/v1/storefront/stores/%s/cart/holds/%s", store.Slug, body.Data.CartToken), nil)
	req.Header.Set("X-Storefront-Key", key)
	del := httptest.NewRecorder()
	router.ServeHTTP(del, req)
	require.Equal(t, http.StatusNoContent, del.Code, del.Body.String())

	// The unit is obtainable by someone else.
	other := postHolds(t, &router, store.Slug, key, uuid.NewString(), []map[string]any{
		{"variant_id": variantID, "quantity": 1},
	})
	var otherBody holdsResponse
	require.NoError(t, json.Unmarshal(other.Body.Bytes(), &otherBody))
	require.Equal(t, "held", otherBody.Data.Items[0].Status)
}

// A variant belonging to another store must not be holdable through this
// store's slug — the hold surface is storefront-facing and unauthenticated
// beyond the shared key, so it must not become a cross-tenant stock oracle.
func TestCartHolds_RefusesAVariantFromAnotherStore(t *testing.T) {
	router, db, store, key := setupHoldsRouter(t)
	foreign := seedForeignVariant(t, db, 5)

	rec := postHolds(t, &router, store.Slug, key, "", []map[string]any{
		{"variant_id": foreign, "quantity": 1},
	})
	require.Equal(t, http.StatusNotFound, rec.Code, rec.Body.String())

	var held int
	require.NoError(t, db.Raw(
		`SELECT COALESCE(SUM(qty),0) FROM stock_holds WHERE variant_id = ?`, foreign).Scan(&held).Error)
	require.Equal(t, 0, held, "no hold may be placed against another store's variant")
}

func setupHoldsRouter(t *testing.T) (http.Handler, *gorm.DB, *stores.Store, string) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	db := testdb.NewDB(t, append(checkoutTruncateTables, "stock_holds")...)
	storesRepo := stores.NewRepository(db)
	slugCache := stores.NewSlugCache(storesRepo, fakeStoresClient{}, &singleflight.Group{}, 5*time.Minute)
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	r := gin.New()
	storefront.RegisterStorefront(r.Group("/api/v1"), storefront.Deps{
		CartHoldsHandler: storefront.NewCartHoldsHandler(db, stockhold.NewRepository(), logger),
		SlugCache:        slugCache,
		StorefrontKey:    "",
	})
	return r, db, seedCheckoutStore(t, db), ""
}

// seedHoldVariant creates a variant in the given store with qty in stock.
func seedHoldVariant(t *testing.T, db *gorm.DB, s *stores.Store, qty int) string {
	t.Helper()
	return insertVariantWithStock(t, db, s.TenantID, s.ID, qty)
}

// seedForeignVariant creates a variant in a DIFFERENT store.
func seedForeignVariant(t *testing.T, db *gorm.DB, qty int) string {
	t.Helper()
	other := seedCheckoutStore(t, db)
	return insertVariantWithStock(t, db, other.TenantID, other.ID, qty)
}

func insertVariantWithStock(t *testing.T, db *gorm.DB, tenantID, storeID string, qty int) string {
	t.Helper()
	productID := uuid.NewString()
	require.NoError(t, db.Exec(
		`INSERT INTO products (id, tenant_id, store_id, title, handle, status, vendor_id, published_at)
		 VALUES (?, ?, ?, 'Hold Test', ?, 'active', ?, now())`,
		productID, tenantID, storeID, "hold-"+uuid.NewString()[:8], uuid.NewString()).Error)

	variantID := uuid.NewString()
	require.NoError(t, db.Exec(
		`INSERT INTO product_variants (id, product_id, store_id, sku, price, currency_code)
		 VALUES (?, ?, ?, ?, 10.00, 'EUR')`,
		variantID, productID, storeID, "SKU-"+uuid.NewString()[:8]).Error)

	// Stock lives at the store's warehouse, not at a sentinel location.
	// #177 PR 6 retired the sentinel: every write path resolves a real
	// warehouse, and cart holds are placed against it. Seeding the old way
	// here would test a state production can no longer be in.
	locationID := ensureTestWarehouse(t, db, tenantID, storeID)
	require.NoError(t, db.Exec(
		`INSERT INTO variant_stock (variant_id, location_id, quantity, updated_at)
		 VALUES (?, ?, ?, now())`, variantID, locationID, qty).Error)
	return variantID
}

// ensureTestWarehouse returns the store's warehouse, creating one if it has
// none — the same find-or-create the product write paths do.
func ensureTestWarehouse(t *testing.T, db *gorm.DB, tenantID, storeID string) string {
	t.Helper()
	var existing string
	err := db.Raw(
		`SELECT id::text FROM warehouses
		  WHERE store_id = ? AND archived_at IS NULL
		  ORDER BY is_default DESC, priority ASC, created_at ASC LIMIT 1`,
		storeID).Row().Scan(&existing)
	if err == nil && existing != "" {
		return existing
	}
	id := uuid.NewString()
	require.NoError(t, db.Exec(
		`INSERT INTO warehouses (id, tenant_id, store_id, name, line1, city, region,
		                         postal_code, country_code, phone, is_default)
		 VALUES (?, ?, ?, 'Main Warehouse', '1 Dock Rd', 'Mumbai', 'MH', '400001', 'IN',
		         '+912200000000', true)`,
		id, tenantID, storeID).Error)
	return id
}
