//go:build integration

package storefront_test

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/lib/pq"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
	"golang.org/x/sync/singleflight"

	"github.com/mark8ly/marketplace-api/internal/crypto"
	"github.com/mark8ly/marketplace-api/internal/handlers/storefront"
	"github.com/mark8ly/marketplace-api/internal/order"
	"github.com/mark8ly/marketplace-api/internal/outbox"
	"github.com/mark8ly/marketplace-api/internal/stores"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

// GST is on what the buyer pays (#1127): A$30 of goods with A$5 off at a
// flat 10% is A$2.50 of tax, not A$3.00. Shipping tax is not affected.
func TestIntegration_TaxPreview_TaxesTheDiscountedPrice(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testdb.NewDB(t, discountConsumptionTruncateTables...)
	rate := 10.0
	require.NoError(t, db.Exec(`
		INSERT INTO supported_countries
			(country_code, name, currency_code, region, payment_providers, shipping_carriers, tax_strategy, tax_rate, is_active)
		VALUES ('Z9', 'Test Flat 10', 'EUR', 'test', ?, ?, 'flat', ?, true)
		ON CONFLICT (country_code) DO UPDATE SET tax_rate = EXCLUDED.tax_rate, is_active = true`,
		pq.StringArray{}, pq.StringArray{}, rate).Error)
	t.Cleanup(func() { db.Exec("DELETE FROM supported_countries WHERE country_code = 'Z9'") })
	store := seedDiscountConsumptionStore(t, db, "Z9")

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	orderSvc := order.NewService(db, order.NewRepository(), outbox.NewRepository(db))
	r := gin.New()
	storefront.RegisterStorefront(r.Group("/api/v1"), storefront.Deps{
		CheckoutExtHandler: storefront.NewCheckoutExtHandler(db, orderSvc, nil, nil, crypto.NoopEncryptor{}, logger),
		SlugCache:          stores.NewSlugCache(stores.NewRepository(db), fakeStoresClient{}, &singleflight.Group{}, 5*time.Minute),
	})

	preview := func(discount string) decimal.Decimal {
		body := `{"items":[
			{"title_snapshot":"Mug","sku_snapshot":"m","unit_price":"10.00","quantity":2,"line_total":"20.00","currency_code":"EUR"},
			{"title_snapshot":"Tee","sku_snapshot":"t","unit_price":"10.00","quantity":1,"line_total":"10.00","currency_code":"EUR"}],
			"shipping_address":{"name":"A","line1":"1 St","city":"X","country_code":"AU"},
			"subtotal":"30.00","discount_total":"` + discount + `"}`
		req := httptest.NewRequest(http.MethodPost, "/api/v1/storefront/stores/"+store.Slug+"/tax-preview", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		var out struct {
			TaxTotal decimal.Decimal `json:"tax_total"`
		}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &out))
		return out.TaxTotal
	}

	require.Equal(t, "3", preview("0").String(), "no discount: 10% of 30")
	require.Equal(t, "2.5", preview("5.00").String(), "A$5 off: 10% of 25")
	require.True(t, preview("99").IsZero(), "a discount above the subtotal leaves nothing to tax")
}
