//go:build integration

// Own delivery options at the rates endpoint and at checkout: the price
// charged is always the seller's option price, re-read on the server, and
// a shipping choice from the other mode (or a forged id) is refused.
package storefront

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/crypto"
	"github.com/mark8ly/marketplace-api/internal/delivery"
	"github.com/mark8ly/marketplace-api/internal/order"
	"github.com/mark8ly/marketplace-api/internal/outbox"
	"github.com/mark8ly/marketplace-api/internal/shipping"
	"github.com/mark8ly/marketplace-api/internal/stores"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

type ownRig struct {
	db        *gorm.DB
	store     *stores.Store
	variantID string
	carrier   *splitOriginCarrier
	dsvc      *delivery.Service
	// set by seedOwnOptions
	standardID, expressID string
}

func setupOwnRig(t *testing.T) *ownRig {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db := testdb.NewDB(t, append([]string{"store_delivery_options", "order_items", "orders", "outbox_events"}, splitOriginTables...)...)
	store := seedSplitOriginStore(t, db)
	// checkout needs a tax rate; the shared SO row is deleted on cleanup.
	require.NoError(t, db.Exec(`UPDATE supported_countries SET tax_rate = 0 WHERE country_code = 'SO'`).Error)
	wh := seedSplitOriginWarehouse(t, db, store.TenantID, store.ID, "WH Alpha", 1, true)
	variantID := seedSplitOriginVariant(t, db, store.TenantID, store.ID)
	seedSplitOriginStock(t, db, variantID, wh.ID, 1000)
	seedSplitOriginCarrierConfig(t, db, store.ID, store.TenantID, wh.ID)
	carrier := &splitOriginCarrier{ratesFor: map[string][]shipping.Rate{
		wh.Line1: {{Carrier: "delhivery", Service: "Standard", Price: decimal.NewFromInt(5), CurrencyCode: "USD", EstimatedDays: 1}},
	}}
	return &ownRig{db: db, store: store, variantID: variantID, carrier: carrier, dsvc: delivery.NewService(db)}
}

func ownDec(s string) *decimal.Decimal { d := decimal.RequireFromString(s); return &d }

// seedOwnOptions gives the store Standard 100 (on) and Express 200 (off).
func (r *ownRig) seedOwnOptions(t *testing.T, store *stores.Store, freeMin *decimal.Decimal) (standardID, expressID string) {
	t.Helper()
	est := "2-3 days"
	opts, err := r.dsvc.ReplaceOptions(context.Background(), store.TenantID, store.ID, []delivery.OptionInput{
		{Name: "Standard", Price: ownDec("100"), Estimate: &est, Enabled: true},
		{Name: "Express", Price: ownDec("200"), Enabled: false},
	}, freeMin)
	require.NoError(t, err)
	return opts[0].ID, opts[1].ID
}

func (r *ownRig) setMode(t *testing.T, store *stores.Store, mode string) {
	t.Helper()
	require.NoError(t, r.dsvc.SetMode(context.Background(), store.ID, mode))
	store.ShippingMode = mode
}

func (r *ownRig) router(t *testing.T, wired bool) *gin.Engine {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	factory := func(string) (shipping.Carrier, bool) { return r.carrier, true }
	rates := NewShippingRatesHandler(r.db, crypto.NewNoopEncryptor(), logger).WithCarrierFactory(factory)
	co := NewCheckoutExtHandler(r.db, order.NewService(r.db, order.NewRepository(), outbox.NewRepository(r.db)),
		nil, nil, crypto.NewNoopEncryptor(), logger).WithCarrierFactory(factory)
	if wired {
		rates.WithDelivery(r.dsvc)
		co.WithDelivery(r.dsvc)
	}
	e := gin.New()
	e.Use(func(c *gin.Context) { c.Set("store", r.store); c.Next() })
	e.POST("/rates", rates.GetRates)
	e.POST("/checkout", co.Checkout)
	return e
}

func ownPost(t *testing.T, e *gin.Engine, path string, body map[string]any) *httptest.ResponseRecorder {
	t.Helper()
	b, err := json.Marshal(body)
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(string(b)))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	return rec
}

func ratesData(t *testing.T, rec *httptest.ResponseRecorder) []map[string]any {
	t.Helper()
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var out struct {
		Data []map[string]any `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
	require.NotNil(t, out.Data, "data must be [] not null: %s", rec.Body.String())
	return out.Data
}

func ownCheckoutBody(variantID, service string, qty int) map[string]any {
	line := decimal.NewFromInt(int64(10 * qty)).StringFixed(2)
	return map[string]any{
		"idempotency_key":  "own-" + uuid.NewString(),
		"customer_email":   "own@example.com",
		"shipping_service": service,
		"payment_provider": "stripe",
		"items": []map[string]any{{
			"title_snapshot": "Widget", "sku_snapshot": "W-1", "variant_id": variantID,
			"unit_price": "10.00", "quantity": qty, "line_total": line, "currency_code": "USD",
		}},
		"shipping_address": map[string]any{"name": "Buyer", "line1": "1 Buyer St", "city": "Buyer City", "country_code": "SO"},
		"subtotal":         line,
	}
}

type ownOrderRow struct {
	ShippingTotal          decimal.Decimal
	ShippingCarrier        *string
	ShippingService        *string
	ShippingOptionName     *string
	ShippingOptionEstimate *string
}

func loadOwnOrder(t *testing.T, db *gorm.DB, storeID string) ownOrderRow {
	t.Helper()
	var o ownOrderRow
	require.NoError(t, db.Table("orders").
		Select("shipping_total, shipping_carrier, shipping_service, shipping_option_name, shipping_option_estimate").
		Where("store_id = ?", storeID).Take(&o).Error)
	return o
}

func TestGetRates_OwnModeReturnsOnlyEnabledOwnOptions(t *testing.T) {
	r := setupOwnRig(t)
	std, _ := r.seedOwnOptions(t, r.store, nil)
	r.setMode(t, r.store, "own")

	data := ratesData(t, ownPost(t, r.router(t, true), "/rates", rateRequestBody(r.variantID, 1)))
	require.Len(t, data, 1)
	require.Equal(t, "own:"+std, data[0]["service"])
	require.Equal(t, "own", data[0]["carrier"])
	require.Equal(t, "Standard", data[0]["name"])
	require.Equal(t, "2-3 days", data[0]["estimate"])
	require.Equal(t, "100.00", data[0]["price"])
	require.Equal(t, "USD", data[0]["currency_code"])
	require.Zero(t, r.carrier.callCount(), "own mode must never call a carrier")
}

func TestGetRates_OwnModeNoEnabledOptionsIsEmpty(t *testing.T) {
	r := setupOwnRig(t)
	r.setMode(t, r.store, "own")
	require.Empty(t, ratesData(t, ownPost(t, r.router(t, true), "/rates", rateRequestBody(r.variantID, 1))))

	// Delivery service not wired: own mode is "no options", never a carrier quote.
	r.seedOwnOptions(t, r.store, nil)
	require.Empty(t, ratesData(t, ownPost(t, r.router(t, false), "/rates", rateRequestBody(r.variantID, 1))))
	require.Zero(t, r.carrier.callCount())
}

func TestGetRates_CarrierModeIgnoresOwnOptions(t *testing.T) {
	r := setupOwnRig(t)
	r.seedOwnOptions(t, r.store, nil)

	data := ratesData(t, ownPost(t, r.router(t, true), "/rates", rateRequestBody(r.variantID, 1)))
	require.Len(t, data, 1)
	require.Equal(t, "Standard", data[0]["service"])
	require.Equal(t, "delhivery", data[0]["carrier"])
	require.Equal(t, "5.00", data[0]["price"])
	_, hasName := data[0]["name"]
	require.False(t, hasName, "carrier rows carry no own-option name")
}

func TestCheckout_OwnOptionPricedFromServer(t *testing.T) {
	r := setupOwnRig(t)
	std, _ := r.seedOwnOptions(t, r.store, nil)
	r.setMode(t, r.store, "own")

	body := ownCheckoutBody(r.variantID, "own:"+std, 2)
	body["shipping_total"] = "0.01" // ignored: the server prices the option
	rec := ownPost(t, r.router(t, true), "/checkout", body)
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())

	o := loadOwnOrder(t, r.db, r.store.ID)
	require.Equal(t, "100.00", o.ShippingTotal.StringFixed(2))
	require.Equal(t, "own", *o.ShippingCarrier)
	require.Equal(t, "own:"+std, *o.ShippingService)
	require.Equal(t, "Standard", *o.ShippingOptionName)
	require.Equal(t, "2-3 days", *o.ShippingOptionEstimate)
	require.Zero(t, r.carrier.callCount())
}

func TestCheckout_OwnFreeShippingThreshold(t *testing.T) {
	r := setupOwnRig(t)
	std, _ := r.seedOwnOptions(t, r.store, ownDec("1000"))
	r.setMode(t, r.store, "own")
	e := r.router(t, true)

	// Below the threshold the option price applies.
	rec := ownPost(t, e, "/checkout", ownCheckoutBody(r.variantID, "own:"+std, 99))
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	require.Equal(t, "100.00", loadOwnOrder(t, r.db, r.store.ID).ShippingTotal.StringFixed(2))
	require.NoError(t, r.db.Exec("DELETE FROM order_items").Error)
	require.NoError(t, r.db.Exec("DELETE FROM orders").Error)

	// Subtotal 1200 >= 1000: free.
	rec = ownPost(t, e, "/checkout", ownCheckoutBody(r.variantID, "own:"+std, 120))
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	o := loadOwnOrder(t, r.db, r.store.ID)
	require.True(t, o.ShippingTotal.IsZero(), "got %s", o.ShippingTotal)
	require.Equal(t, "Standard", *o.ShippingOptionName)
}

func TestCheckout_RejectsForgedOwnService(t *testing.T) {
	cases := []struct {
		name    string
		mode    string
		wired   bool
		service func(r *ownRig) string
	}{
		{"disabled option", "own", true, func(r *ownRig) string { return "own:" + r.expressID }},
		{"other store's option", "own", true, func(r *ownRig) string {
			other := seedSplitOriginStore(t, r.db)
			id, _ := r.seedOwnOptions(t, other, nil)
			return "own:" + id
		}},
		{"malformed id", "own", true, func(*ownRig) string { return "own:not-a-uuid" }},
		{"own option in carrier mode", "carrier", true, func(r *ownRig) string { return "own:" + r.standardID }},
		{"carrier service in own mode", "own", true, func(*ownRig) string { return "standard" }},
		{"delivery not wired", "own", false, func(r *ownRig) string { return "own:" + r.standardID }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := setupOwnRig(t)
			r.standardID, r.expressID = r.seedOwnOptions(t, r.store, nil)
			r.setMode(t, r.store, tc.mode)

			rec := ownPost(t, r.router(t, tc.wired), "/checkout", ownCheckoutBody(r.variantID, tc.service(r), 1))
			require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
			require.Contains(t, rec.Body.String(), `"validation_failed"`)

			var n int64
			require.NoError(t, r.db.Table("orders").Count(&n).Error)
			require.Zero(t, n, "a refused shipping choice must not create an order")
			require.Zero(t, r.carrier.callCount(), "a refused own choice must not reach a carrier")
		})
	}
}
