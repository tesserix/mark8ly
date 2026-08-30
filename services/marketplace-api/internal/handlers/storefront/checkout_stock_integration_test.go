//go:build integration

package storefront_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
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
	"github.com/mark8ly/marketplace-api/internal/order"
	"github.com/mark8ly/marketplace-api/internal/outbox"
	"github.com/mark8ly/marketplace-api/internal/stockhold"
	"github.com/mark8ly/marketplace-api/internal/stores"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

// #230 — a storefront sale must decrement stock, and two customers must not
// be able to buy the same last unit.
//
// The bug: checkout never read inventory at all. A comment in
// checkout_lowstock.go claimed a decrement-on-sale trigger existed; it never
// did. The only stock trigger mirrors variant_stock into the denormalised
// product_variants.inventory_quantity and does nothing on order insert.

func checkoutBodyForVariant(variantID string, qty int) map[string]any {
	return map[string]any{
		"idempotency_key": "stock-" + uuid.NewString(),
		"customer_email":  "stock@example.com",
		"items": []map[string]any{{
			"title_snapshot": "Stocked Widget",
			"sku_snapshot":   "STK-1",
			"variant_id":     variantID,
			"unit_price":     "10.00",
			"quantity":       qty,
			"line_total":     fmt.Sprintf("%d.00", 10*qty),
			"currency_code":  "EUR",
		}},
		"subtotal":       fmt.Sprintf("%d.00", 10*qty),
		"shipping_total": "0.00",
		"tax_total":      "0.00",
		"discount_total": "0.00",
		"grand_total":    fmt.Sprintf("%d.00", 10*qty),
		"shipping":       checkoutAddress(),
		"billing":        checkoutAddress(),
	}
}

func stockOf(t *testing.T, db *gorm.DB, variantID string) int {
	t.Helper()
	var q int
	require.NoError(t, db.Raw(`SELECT quantity FROM variant_stock WHERE variant_id = ?`, variantID).Scan(&q).Error)
	return q
}

func TestStorefrontCheckout_DecrementsStock(t *testing.T) {
	r, db, store := setupStockCheckoutRouter(t)
	variantID := insertVariantWithStock(t, db, store.TenantID, store.ID, 5)

	rec := doRequest(t, r, http.MethodPost,
		"/api/v1/storefront/stores/"+store.Slug+"/checkout", checkoutBodyForVariant(variantID, 2))
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())

	require.Equal(t, 3, stockOf(t, db, variantID), "a committed sale must decrement variant_stock")
}

// The acceptance test for this issue. Two concurrent checkouts for one unit:
// exactly one succeeds, the other gets a 409 naming the variant.
func TestStorefrontCheckout_ConcurrentBuyersCannotBothTakeTheLastUnit(t *testing.T) {
	r, db, store := setupStockCheckoutRouter(t)
	variantID := insertVariantWithStock(t, db, store.TenantID, store.ID, 1)
	url := "/api/v1/storefront/stores/" + store.Slug + "/checkout"

	var wg sync.WaitGroup
	codes := make(chan int, 2)
	bodies := make(chan string, 2)
	start := make(chan struct{})
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			rec := doRequest(t, r, http.MethodPost, url, checkoutBodyForVariant(variantID, 1))
			codes <- rec.Code
			bodies <- rec.Body.String()
		}()
	}
	close(start)
	wg.Wait()
	close(codes)
	close(bodies)

	var created, conflict int
	for code := range codes {
		switch code {
		case http.StatusCreated:
			created++
		case http.StatusConflict:
			conflict++
		default:
			t.Errorf("unexpected status %d", code)
		}
	}
	require.Equal(t, 1, created, "exactly one buyer may take the last unit")
	require.Equal(t, 1, conflict, "the loser must get a 409, not a 500 and not a second order")

	var sawCode, sawVariant bool
	for b := range bodies {
		if len(b) == 0 {
			continue
		}
		var parsed map[string]any
		if json.Unmarshal([]byte(b), &parsed) == nil {
			if parsed["error"] == "out_of_stock" {
				sawCode = true
			}
			if fmt.Sprint(parsed["variant_id"]) == variantID {
				sawVariant = true
			}
		}
	}
	require.True(t, sawCode, "the refusal must carry a machine-readable out_of_stock code")
	require.True(t, sawVariant, "and must name the variant the shopper has to remove")

	require.Equal(t, 0, stockOf(t, db, variantID), "stock must land at zero, never negative")
}

// A rolled-back order must not consume stock. This is why the decrement runs
// in the order transaction rather than after it.
func TestStorefrontCheckout_RefusedOrderLeavesStockUntouched(t *testing.T) {
	r, db, store := setupStockCheckoutRouter(t)
	variantID := insertVariantWithStock(t, db, store.TenantID, store.ID, 1)
	url := "/api/v1/storefront/stores/" + store.Slug + "/checkout"

	require.Equal(t, http.StatusConflict,
		doRequest(t, r, http.MethodPost, url, checkoutBodyForVariant(variantID, 5)).Code)

	require.Equal(t, 1, stockOf(t, db, variantID), "a refused checkout must not move stock")

	var orders int64
	require.NoError(t, db.Raw(`SELECT count(*) FROM orders WHERE store_id = ?`, store.ID).Scan(&orders).Error)
	require.Equal(t, int64(0), orders, "no order may persist for a refused checkout")
}

// inventory_policy = 'continue' means the merchant sells past zero on
// purpose. The column exists and must be honoured, not ignored.
func TestStorefrontCheckout_ContinuePolicyStillSellsPastZero(t *testing.T) {
	r, db, store := setupStockCheckoutRouter(t)
	variantID := insertVariantWithStock(t, db, store.TenantID, store.ID, 1)
	require.NoError(t, db.Exec(
		`UPDATE product_variants SET inventory_policy = 'continue' WHERE id = ?`, variantID).Error)

	rec := doRequest(t, r, http.MethodPost,
		"/api/v1/storefront/stores/"+store.Slug+"/checkout", checkoutBodyForVariant(variantID, 5))
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())

	// Clamped at zero rather than negative: variant_stock carries a
	// non-negative CHECK (#231) and it cannot be policy-aware, since the
	// policy lives on another table.
	require.Equal(t, 0, stockOf(t, db, variantID))
}

// An item with no variant_id (a custom or unstocked line) must still check
// out — stock enforcement applies to variants, and refusing these would
// break every unstocked sale.
func TestStorefrontCheckout_ItemWithoutAVariantIsUnaffected(t *testing.T) {
	r, _, store := setupStockCheckoutRouter(t)
	body := checkoutBodyForVariant("", 1)
	items := body["items"].([]map[string]any)
	delete(items[0], "variant_id")

	rec := doRequest(t, r, http.MethodPost,
		"/api/v1/storefront/stores/"+store.Slug+"/checkout", body)
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
}

func checkoutAddress() map[string]any {
	return map[string]any{
		"name": "Buyer", "line1": "1 High St", "city": "Dublin", "country_code": "IE",
	}
}

// setupStockCheckoutRouter mirrors setupCheckoutRouter but wires the stock
// holds repository, which is what makes checkout enforce availability.
func setupStockCheckoutRouter(t *testing.T) (*gin.Engine, *gorm.DB, *stores.Store) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	db := testdb.NewDB(t, append(checkoutTruncateTables, "stock_holds")...)
	storesRepo := stores.NewRepository(db)
	slugCache := stores.NewSlugCache(storesRepo, fakeStoresClient{}, &singleflight.Group{}, 5*time.Minute)
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	orderRepo := order.NewRepository()
	outboxRepo := outbox.NewRepository(db)
	orderSvc := order.NewService(db, orderRepo, outboxRepo)
	checkoutHandler := storefront.NewCheckoutHandler(db, orderSvc, orderRepo, logger).
		WithStockHolds(stockhold.NewRepository())

	r := gin.New()
	storefront.RegisterStorefront(r.Group("/api/v1"), storefront.Deps{
		CheckoutHandler: checkoutHandler,
		SlugCache:       slugCache,
		StorefrontKey:   "",
	})
	return r, db, seedCheckoutStore(t, db)
}
