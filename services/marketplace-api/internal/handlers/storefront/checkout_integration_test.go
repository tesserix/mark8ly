//go:build integration

package storefront_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"golang.org/x/sync/singleflight"
	"gorm.io/datatypes"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/handlers/storefront"
	"github.com/mark8ly/marketplace-api/internal/order"
	"github.com/mark8ly/marketplace-api/internal/outbox"
	"github.com/mark8ly/marketplace-api/internal/stores"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

// checkoutTruncateTables is the dependency-ordered truncate list for the
// checkout integration tests. CASCADE handles cross-table cleanup.
var checkoutTruncateTables = []string{
	"order_events",
	"order_addresses",
	"order_items",
	"orders",
	"abandoned_carts",
	"outbox_events",
	"store_watermarks",
	"stores",
}

// setupCheckoutRouter wires the storefront router with the checkout
// handler attached. Returns the router, the *gorm.DB, and the seeded
// store row so tests can reference its slug + IDs.
func setupCheckoutRouter(t *testing.T) (*gin.Engine, *gorm.DB, *stores.Store) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	db := testdb.NewDB(t, checkoutTruncateTables...)
	storesRepo := stores.NewRepository(db)
	slugCache := stores.NewSlugCache(storesRepo, fakeStoresClient{}, &singleflight.Group{}, 5*time.Minute)
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	orderRepo := order.NewRepository()
	outboxRepo := outbox.NewRepository(db)
	orderSvc := order.NewService(db, orderRepo, outboxRepo)
	checkoutHandler := storefront.NewCheckoutHandler(db, orderSvc, orderRepo, logger)

	r := gin.New()
	storefront.RegisterStorefront(r.Group("/api/v1"), storefront.Deps{
		// Read handler is nil — the checkout tests never hit the read
		// routes; setting nil keeps the test surface narrow and any
		// accidental hit panics loudly instead of silently 200ing.
		Handler:         nil,
		CheckoutHandler: checkoutHandler,
		SlugCache:       slugCache,
		StorefrontKey:   "",
	})

	store := seedCheckoutStore(t, db)
	return r, db, store
}

// seedCheckoutStore inserts a fresh stores row and returns the *Store so
// tests can reference its slug, id, and tenant id. The trigger from
// migration 000004 fires automatically and creates the per-store
// sequences for orders and returns.
func seedCheckoutStore(t *testing.T, db *gorm.DB) *stores.Store {
	t.Helper()
	storeID := uuid.NewString()
	tenantID := uuid.NewString()
	s := &stores.Store{
		ID:           storeID,
		TenantID:     tenantID,
		Slug:         "co-" + storeID[:8],
		Name:         "Checkout Test Store",
		CountryCode:  "IE",
		CurrencyCode: "EUR",
		Timezone:     "Europe/Dublin",
		Status:       stores.StatusActive,
		SyncedAt:     time.Now(),
	}
	if err := db.Create(s).Error; err != nil {
		t.Fatalf("seed store: %v", err)
	}
	return s
}

func doRequest(t *testing.T, r http.Handler, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var br io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		br = bytes.NewReader(b)
	}
	req := httptest.NewRequest(method, path, br)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// TestStorefrontCheckout_HappyPath drives a single checkout call end-to-end
// and asserts: (1) HTTP 201, (2) order_number is generated with the
// store's prefix, (3) order_events row landed for the create, (4)
// outbox_events row landed with the order.placed event_type, (5)
// idempotent replay returns 200 with the same id.
func TestStorefrontCheckout_HappyPath(t *testing.T) {
	r, db, store := setupCheckoutRouter(t)
	url := "/api/v1/storefront/stores/" + store.Slug + "/checkout"

	body := map[string]any{
		"idempotency_key": "happy-" + uuid.NewString(),
		"customer_email":  "checkout@example.com",
		"items": []map[string]any{
			{
				"title_snapshot": "Widget",
				"sku_snapshot":   "WID-1",
				"unit_price":     "30.00",
				"quantity":       2,
				"line_total":     "60.00",
				"currency_code":  "EUR",
			},
		},
		"shipping": map[string]any{
			"name": "Buyer", "line1": "1 High St", "city": "Dublin", "country_code": "IE",
		},
		"billing": map[string]any{
			"name": "Buyer", "line1": "1 High St", "city": "Dublin", "country_code": "IE",
		},
		"subtotal":    "60.00",
		"grand_total": "60.00",
	}

	w := doRequest(t, r, http.MethodPost, url, body)
	if w.Code != http.StatusCreated {
		t.Fatalf("checkout: status %d body=%s", w.Code, w.Body.String())
	}
	var resp storefront.CheckoutResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Status != "pending" {
		t.Fatalf("status = %q, want pending", resp.Status)
	}
	if !decimal.NewFromInt(60).Equal(resp.GrandTotal) {
		t.Fatalf("grand_total = %s, want 60", resp.GrandTotal.String())
	}
	if len(resp.OrderNumber) < 5 || resp.OrderNumber[:2] != "M-" {
		t.Fatalf("order_number = %q, want M-… prefix", resp.OrderNumber)
	}

	// order_events row landed.
	var eventCount int64
	if err := db.Model(&order.OrderEvent{}).
		Where("order_id = ?", resp.ID).Count(&eventCount).Error; err != nil {
		t.Fatalf("count order_events: %v", err)
	}
	if eventCount < 1 {
		t.Fatalf("expected ≥1 order_events row, got %d", eventCount)
	}

	// outbox_events row landed with order.placed.
	var outboxCount int64
	if err := db.Model(&outbox.OutboxEvent{}).
		Where("aggregate_id = ? AND event_type = ?", resp.ID, outbox.EventOrderPlaced).
		Count(&outboxCount).Error; err != nil {
		t.Fatalf("count outbox_events: %v", err)
	}
	if outboxCount != 1 {
		t.Fatalf("expected 1 order.placed outbox row, got %d", outboxCount)
	}

	// Idempotent replay returns 200 with the same id.
	w2 := doRequest(t, r, http.MethodPost, url, body)
	if w2.Code != http.StatusOK {
		t.Fatalf("replay: status %d body=%s", w2.Code, w2.Body.String())
	}
	var replay storefront.CheckoutResponse
	_ = json.Unmarshal(w2.Body.Bytes(), &replay)
	if replay.ID != resp.ID {
		t.Fatalf("replay: id %q != original %q", replay.ID, resp.ID)
	}
}

// TestStorefrontCheckout_LinksAbandonedCart verifies that when a checkout
// request carries cart_session_id matching an open abandoned_carts row,
// the row's converted_order_id is set to the new order id.
func TestStorefrontCheckout_LinksAbandonedCart(t *testing.T) {
	ctx := context.Background()
	r, db, store := setupCheckoutRouter(t)

	storeUUID := uuid.MustParse(store.ID)
	tenantUUID := uuid.MustParse(store.TenantID)
	cartSessionID := "sess-" + uuid.NewString()

	// Pre-seed an abandoned cart row scoped to (store, session).
	cart := order.AbandonedCart{
		TenantID:      tenantUUID,
		StoreID:       storeUUID,
		CartSessionID: cartSessionID,
		ItemCount:     2,
		Subtotal:      decimal.NewFromInt(60),
		CurrencyCode:  "EUR",
		ItemsSnapshot: datatypes.JSON([]byte(`[]`)),
		LastActiveAt:  time.Now().UTC(),
	}
	if err := db.WithContext(ctx).Create(&cart).Error; err != nil {
		t.Fatalf("seed abandoned cart: %v", err)
	}

	body := map[string]any{
		"idempotency_key": "link-" + uuid.NewString(),
		"cart_session_id": cartSessionID,
		"customer_email":  "linker@example.com",
		"items": []map[string]any{
			{
				"title_snapshot": "Widget",
				"sku_snapshot":   "WID-1",
				"unit_price":     "30.00",
				"quantity":       2,
				"line_total":     "60.00",
				"currency_code":  "EUR",
			},
		},
		"shipping": map[string]any{
			"name": "Buyer", "line1": "1 High St", "city": "Dublin", "country_code": "IE",
		},
		"billing": map[string]any{
			"name": "Buyer", "line1": "1 High St", "city": "Dublin", "country_code": "IE",
		},
		"subtotal":    "60.00",
		"grand_total": "60.00",
	}
	w := doRequest(t, r, http.MethodPost, "/api/v1/storefront/stores/"+store.Slug+"/checkout", body)
	if w.Code != http.StatusCreated {
		t.Fatalf("checkout: status %d body=%s", w.Code, w.Body.String())
	}
	var resp storefront.CheckoutResponse
	_ = json.Unmarshal(w.Body.Bytes(), &resp)

	// Reload the abandoned cart and assert converted_order_id is set.
	var reloaded order.AbandonedCart
	if err := db.Where("id = ?", cart.ID).First(&reloaded).Error; err != nil {
		t.Fatalf("reload cart: %v", err)
	}
	if reloaded.ConvertedOrderID == nil {
		t.Fatalf("converted_order_id is nil after checkout")
	}
	if reloaded.ConvertedOrderID.String() != resp.ID {
		t.Fatalf("converted_order_id = %s, want %s", reloaded.ConvertedOrderID.String(), resp.ID)
	}
}

// TestStorefrontCheckout_CurrencyMismatch400 verifies that a checkout
// whose item currency differs from the store's currency is rejected with
// 400 (validation_failed family) instead of silently overriding.
func TestStorefrontCheckout_CurrencyMismatch400(t *testing.T) {
	r, _, store := setupCheckoutRouter(t)
	body := map[string]any{
		"idempotency_key": "ccy-" + uuid.NewString(),
		"customer_email":  "ccy@example.com",
		"items": []map[string]any{
			{
				"title_snapshot": "Widget",
				"sku_snapshot":   "WID-1",
				"unit_price":     "30.00",
				"quantity":       1,
				"line_total":     "30.00",
				"currency_code":  "USD", // store is EUR
			},
		},
		"shipping": map[string]any{
			"name": "B", "line1": "1", "city": "C", "country_code": "IE",
		},
		"billing": map[string]any{
			"name": "B", "line1": "1", "city": "C", "country_code": "IE",
		},
		"subtotal":    "30.00",
		"grand_total": "30.00",
	}
	w := doRequest(t, r, http.MethodPost, "/api/v1/storefront/stores/"+store.Slug+"/checkout", body)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("currency mismatch: status %d, want 400 (body=%s)", w.Code, w.Body.String())
	}
}
