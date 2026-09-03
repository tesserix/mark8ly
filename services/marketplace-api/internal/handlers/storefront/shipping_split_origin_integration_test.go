//go:build integration

// Package storefront — coverage for #541: shipping-rate quotes and the
// checkout charge must price per fulfilling warehouse, not from one fixed
// origin, once allocation.Plan splits an order's stock across warehouses.
//
// In-package (not storefront_test) so these can drive
// (*ShippingRatesHandler).GetRates and (*CheckoutExtHandler).calculateShipping
// directly, and inject a stub Carrier via WithCarrierFactory — the same
// pattern (*ShipmentsHandler).WithCarrierFactory uses in
// internal/handlers/admin — so no test hits a live carrier or spins up an
// HTTPS server.
package storefront

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/lib/pq"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/country"
	"github.com/mark8ly/marketplace-api/internal/crypto"
	"github.com/mark8ly/marketplace-api/internal/shipping"
	"github.com/mark8ly/marketplace-api/internal/stores"
	"github.com/mark8ly/marketplace-api/internal/warehouse"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

var splitOriginTables = []string{
	"stock_holds",
	"variant_stock",
	"product_variants",
	"products",
	"warehouses",
	"shipping_carrier_configs",
	"stores",
}

// splitOriginCarrier is a stub shipping.Carrier whose GetRates answer is
// driven entirely by ratesFor, keyed on the FromAddress.Line1 the caller
// passed in — which is exactly what lets a test tell "was this the whA
// call or the whB call" apart without a real HTTP round trip.
type splitOriginCarrier struct {
	mu       sync.Mutex
	calls    []shipping.RateRequest
	ratesFor map[string][]shipping.Rate // keyed by FromAddress.Line1
	err      error
}

func (c *splitOriginCarrier) GetRates(_ context.Context, in shipping.RateRequest) ([]shipping.Rate, error) {
	c.mu.Lock()
	c.calls = append(c.calls, in)
	c.mu.Unlock()
	if c.err != nil {
		return nil, c.err
	}
	return c.ratesFor[in.FromAddress.Line1], nil
}
func (c *splitOriginCarrier) CreateShipment(context.Context, shipping.ShipmentRequest) (*shipping.Shipment, error) {
	return nil, nil
}
func (c *splitOriginCarrier) GetTracking(context.Context, string) (*shipping.Tracking, error) {
	return nil, nil
}
func (c *splitOriginCarrier) CancelShipment(context.Context, string) error { return nil }
func (c *splitOriginCarrier) ProviderName() string                         { return "delhivery" }
func (c *splitOriginCarrier) SupportedCountries() []string                 { return []string{"SO"} }

func (c *splitOriginCarrier) callCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.calls)
}

func (c *splitOriginCarrier) fromAddresses() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]string, 0, len(c.calls))
	for _, call := range c.calls {
		out = append(out, call.FromAddress.Line1)
	}
	return out
}

func seedSplitOriginCountry(t *testing.T, db *gorm.DB) {
	t.Helper()
	require.NoError(t, db.Exec(
		`INSERT INTO supported_countries
		    (country_code, name, currency_code, region, payment_providers, shipping_carriers, tax_strategy)
		 VALUES ('SO', 'Split Origin Test Land', 'USD', 'test', '{}', '{delhivery}', 'flat')
		 ON CONFLICT (country_code) DO NOTHING`).Error)
	t.Cleanup(func() {
		db.Exec(`DELETE FROM supported_countries WHERE country_code = 'SO'`)
	})
}

func seedSplitOriginStore(t *testing.T, db *gorm.DB) *stores.Store {
	t.Helper()
	seedSplitOriginCountry(t, db)
	storeID := uuid.NewString()
	s := &stores.Store{
		ID:           storeID,
		TenantID:     uuid.NewString(),
		Slug:         "split-" + storeID[:8],
		Name:         "Split Origin Test Store",
		CountryCode:  "SO",
		CurrencyCode: "USD",
		Timezone:     "UTC",
		Status:       stores.StatusActive,
		SyncedAt:     time.Now(),
	}
	require.NoError(t, db.Create(s).Error)
	return s
}

func seedSplitOriginWarehouse(t *testing.T, db *gorm.DB, tenantID, storeID, name string, priority int, isDefault bool) warehouse.Warehouse {
	t.Helper()
	wh, err := warehouse.NewRepository().Upsert(context.Background(), db, warehouse.Warehouse{
		TenantID:    tenantID,
		StoreID:     storeID,
		Name:        name,
		Line1:       name + " Line1",
		City:        name + " City",
		Region:      "RG",
		PostalCode:  "00000",
		CountryCode: "SO",
		Phone:       "+10000000000",
		IsDefault:   isDefault,
		Priority:    priority,
	})
	require.NoError(t, err)
	return wh
}

func seedSplitOriginCarrierConfig(t *testing.T, db *gorm.DB, storeID, tenantID, warehouseID string) {
	t.Helper()
	require.NoError(t, db.Exec(
		`INSERT INTO shipping_carrier_configs
		    (id, tenant_id, store_id, provider, api_key_encrypted, mode, is_active, warehouse_id)
		 VALUES (?, ?, ?, 'delhivery', 'test-key', 'test', true, ?)`,
		uuid.NewString(), tenantID, storeID, warehouseID).Error)
}

func seedSplitOriginVariant(t *testing.T, db *gorm.DB, tenantID, storeID string) string {
	t.Helper()
	productID := uuid.NewString()
	require.NoError(t, db.Exec(
		`INSERT INTO products (id, tenant_id, store_id, title, handle, status, vendor_id, published_at)
		 VALUES (?, ?, ?, 'Split Origin Widget', ?, 'active', ?, now())`,
		productID, tenantID, storeID, "split-"+uuid.NewString()[:8], uuid.NewString()).Error)

	variantID := uuid.NewString()
	require.NoError(t, db.Exec(
		`INSERT INTO product_variants (id, product_id, store_id, sku, price, currency_code, weight_grams)
		 VALUES (?, ?, ?, ?, 10.00, 'USD', 100)`,
		variantID, productID, storeID, "SKU-"+uuid.NewString()[:8]).Error)
	return variantID
}

func seedSplitOriginStock(t *testing.T, db *gorm.DB, variantID, locationID string, qty int) {
	t.Helper()
	require.NoError(t, db.Exec(
		`INSERT INTO variant_stock (variant_id, location_id, quantity, updated_at)
		 VALUES (?, ?, ?, now())`, variantID, locationID, qty).Error)
}

// splitOriginRig bundles everything a GetRates test needs.
type splitOriginRig struct {
	router    *gin.Engine
	db        *gorm.DB
	store     *stores.Store
	carrier   *splitOriginCarrier
	whA, whB  warehouse.Warehouse
	variantID string
}

// setupSplitOriginRig seeds a store with TWO warehouses and one variant,
// wires ShippingRatesHandler to the stub carrier, and returns everything
// a test needs to drive it. cfgWarehouse selects which warehouse the
// carrier config (and therefore the single-origin fallback) links to.
func setupSplitOriginRig(t *testing.T, stockA, stockB int, cfgWarehouse string) *splitOriginRig {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db := testdb.NewDB(t, splitOriginTables...)

	store := seedSplitOriginStore(t, db)
	whA := seedSplitOriginWarehouse(t, db, store.TenantID, store.ID, "WH Alpha", 1, true)
	whB := seedSplitOriginWarehouse(t, db, store.TenantID, store.ID, "WH Beta", 2, false)
	variantID := seedSplitOriginVariant(t, db, store.TenantID, store.ID)
	if stockA > 0 {
		seedSplitOriginStock(t, db, variantID, whA.ID, stockA)
	}
	if stockB > 0 {
		seedSplitOriginStock(t, db, variantID, whB.ID, stockB)
	}
	linked := whA.ID
	if cfgWarehouse == whB.ID {
		linked = whB.ID
	}
	seedSplitOriginCarrierConfig(t, db, store.ID, store.TenantID, linked)

	carrier := &splitOriginCarrier{}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	h := NewShippingRatesHandler(db, crypto.NewNoopEncryptor(), logger).WithCarrierFactory(func(string) (shipping.Carrier, bool) {
		return carrier, true
	})

	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("store", store)
		c.Next()
	})
	r.POST("/rates", h.GetRates)

	return &splitOriginRig{router: r, db: db, store: store, carrier: carrier, whA: whA, whB: whB, variantID: variantID}
}

func (rig *splitOriginRig) postRates(t *testing.T, body map[string]any) *httptest.ResponseRecorder {
	t.Helper()
	b, err := json.Marshal(body)
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, "/rates", strings.NewReader(string(b)))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	rig.router.ServeHTTP(rec, req)
	return rec
}

func rateRequestBody(variantID string, qty int) map[string]any {
	return map[string]any{
		"items": []map[string]any{{
			"product_id":   "prod-1",
			"variant_id":   variantID,
			"quantity":     qty,
			"weight_grams": 100,
		}},
		"ship_to": map[string]any{
			"line1":        "1 Buyer St",
			"city":         "Buyer City",
			"country_code": "SO",
		},
	}
}

// TestGetRates_SplitOriginSumsPricesAndReportsShipmentCount is the
// acceptance test for #541: a variant split across two warehouses must
// produce two carrier calls from different origins, a combined price
// that SUMS both origins' rate, and an additive shipment_count.
func TestGetRates_SplitOriginSumsPricesAndReportsShipmentCount(t *testing.T) {
	rig := setupSplitOriginRig(t, 3, 4, "")
	rig.carrier.ratesFor = map[string][]shipping.Rate{
		rig.whA.Line1: {{Carrier: "delhivery", Service: "Standard", Price: decimal.NewFromInt(5), CurrencyCode: "USD", EstimatedDays: 1}},
		rig.whB.Line1: {{Carrier: "delhivery", Service: "Standard", Price: decimal.NewFromInt(7), CurrencyCode: "USD", EstimatedDays: 2}},
	}

	// Requesting 5 units against 3@whA + 4@whB forces allocation.Plan to
	// split: 3 from whA (higher priority, filled first), 2 from whB.
	rec := rig.postRates(t, rateRequestBody(rig.variantID, 5))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	require.Equal(t, 2, rig.carrier.callCount(), "must call the carrier once per fulfilling warehouse")
	addrs := rig.carrier.fromAddresses()
	require.ElementsMatch(t, []string{rig.whA.Line1, rig.whB.Line1}, addrs, "each call's FromAddress must be a different origin")

	var parsed struct {
		Data []map[string]any `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &parsed))
	require.Len(t, parsed.Data, 1)
	require.Equal(t, "12.00", parsed.Data[0]["price"], "price must be the SUM of both origins' rate")
	require.Equal(t, float64(2), parsed.Data[0]["estimated_days"], "estimated_days must be the MAX across origins")
	require.Equal(t, float64(2), parsed.Data[0]["shipment_count"])
}

// TestGetRates_SingleWarehouseUnchanged is the regression guard for the
// overwhelmingly common case: a store with only one warehouse must see
// EXACTLY one carrier call and a response with no shipment_count field at
// all — identical to the pre-#541 behaviour.
func TestGetRates_SingleWarehouseUnchanged(t *testing.T) {
	rig := setupSplitOriginRig(t, 5, 0, "")
	rig.carrier.ratesFor = map[string][]shipping.Rate{
		rig.whA.Line1: {{Carrier: "delhivery", Service: "Standard", Price: decimal.NewFromInt(5), CurrencyCode: "USD", EstimatedDays: 1}},
	}

	rec := rig.postRates(t, rateRequestBody(rig.variantID, 3))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	require.Equal(t, 1, rig.carrier.callCount(), "a single-warehouse store must make exactly one carrier call")
	require.Equal(t, []string{rig.whA.Line1}, rig.carrier.fromAddresses())

	var parsed struct {
		Data []map[string]any `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &parsed))
	require.Len(t, parsed.Data, 1)
	require.Equal(t, "5.00", parsed.Data[0]["price"])
	_, hasShipmentCount := parsed.Data[0]["shipment_count"]
	require.False(t, hasShipmentCount, "shipment_count must be omitted on the single-origin path")
}

// TestGetRates_ServiceMissingFromOneOriginIsExcluded pins the intersection
// rule: a service only one origin can provide cannot ship the whole
// order, so it must not appear in the combined result.
func TestGetRates_ServiceMissingFromOneOriginIsExcluded(t *testing.T) {
	rig := setupSplitOriginRig(t, 3, 4, "")
	rig.carrier.ratesFor = map[string][]shipping.Rate{
		rig.whA.Line1: {
			{Carrier: "delhivery", Service: "Standard", Price: decimal.NewFromInt(5), CurrencyCode: "USD", EstimatedDays: 1},
			{Carrier: "delhivery", Service: "Express", Price: decimal.NewFromInt(9), CurrencyCode: "USD", EstimatedDays: 1},
		},
		rig.whB.Line1: {
			{Carrier: "delhivery", Service: "Standard", Price: decimal.NewFromInt(7), CurrencyCode: "USD", EstimatedDays: 2},
		},
	}

	rec := rig.postRates(t, rateRequestBody(rig.variantID, 5))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	var parsed struct {
		Data []map[string]any `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &parsed))
	require.Len(t, parsed.Data, 1, "only the service present in EVERY origin group may be offered")
	require.Equal(t, "Standard", parsed.Data[0]["service"])
}

// TestGetRates_EstimatedDaysIsMaxAcrossOrigins pins the days rule
// separately from the price rule.
func TestGetRates_EstimatedDaysIsMaxAcrossOrigins(t *testing.T) {
	rig := setupSplitOriginRig(t, 3, 4, "")
	rig.carrier.ratesFor = map[string][]shipping.Rate{
		rig.whA.Line1: {{Carrier: "delhivery", Service: "Standard", Price: decimal.NewFromInt(1), CurrencyCode: "USD", EstimatedDays: 2}},
		rig.whB.Line1: {{Carrier: "delhivery", Service: "Standard", Price: decimal.NewFromInt(1), CurrencyCode: "USD", EstimatedDays: 5}},
	}

	rec := rig.postRates(t, rateRequestBody(rig.variantID, 5))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	var parsed struct {
		Data []map[string]any `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &parsed))
	require.Len(t, parsed.Data, 1)
	require.Equal(t, float64(5), parsed.Data[0]["estimated_days"], "estimated_days must be the slowest origin's, not the fastest's")
}

// TestGetRates_FallsBackWhenVariantIDMissing covers #541's first fallback
// condition: any item without a variant_id must use the single-origin
// path entirely, even though the store has two warehouses.
func TestGetRates_FallsBackWhenVariantIDMissing(t *testing.T) {
	rig := setupSplitOriginRig(t, 3, 4, "")
	rig.carrier.ratesFor = map[string][]shipping.Rate{
		rig.whA.Line1: {{Carrier: "delhivery", Service: "Standard", Price: decimal.NewFromInt(5), CurrencyCode: "USD", EstimatedDays: 1}},
	}

	body := map[string]any{
		"items": []map[string]any{{
			"product_id":   "prod-1",
			"quantity":     1,
			"weight_grams": 100,
			// variant_id deliberately omitted
		}},
		"ship_to": map[string]any{
			"line1": "1 Buyer St", "city": "Buyer City", "country_code": "SO",
		},
	}
	rec := rig.postRates(t, body)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	require.Equal(t, 1, rig.carrier.callCount(), "a missing variant_id must fall back to a single carrier call")
	require.Equal(t, []string{rig.whA.Line1}, rig.carrier.fromAddresses(), "the fallback call must use the carrier config's linked warehouse")

	var parsed struct {
		Data []map[string]any `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &parsed))
	require.Len(t, parsed.Data, 1)
	_, hasShipmentCount := parsed.Data[0]["shipment_count"]
	require.False(t, hasShipmentCount)
}

// TestGetRates_FallsBackWhenPlanCannotFill covers the CannotFillError
// fallback condition: a cart asking for more units than every warehouse
// holds combined must still return a quote, from the single-origin path.
func TestGetRates_FallsBackWhenPlanCannotFill(t *testing.T) {
	rig := setupSplitOriginRig(t, 1, 2, "") // only 3 units exist across both warehouses
	rig.carrier.ratesFor = map[string][]shipping.Rate{
		rig.whA.Line1: {{Carrier: "delhivery", Service: "Standard", Price: decimal.NewFromInt(5), CurrencyCode: "USD", EstimatedDays: 1}},
	}

	rec := rig.postRates(t, rateRequestBody(rig.variantID, 10)) // more than the 3 available
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	require.Equal(t, 1, rig.carrier.callCount(), "a cart allocation.Plan cannot fill must fall back to a single carrier call")
	require.Equal(t, []string{rig.whA.Line1}, rig.carrier.fromAddresses())
}

// TestCalculateShipping_MatchesGetRatesQuoteForSameSplitCart is the point
// of #541: the checkout charge and the rates quote must never disagree
// about a split cart's price. Before this fix, checkout_ext.go priced
// from ONE fixed origin while (once fixed) GetRates priced from the
// warehouses that actually fulfil the cart — so a merchant could quote
// one number and charge another for the identical cart.
func TestCalculateShipping_MatchesGetRatesQuoteForSameSplitCart(t *testing.T) {
	rig := setupSplitOriginRig(t, 3, 4, "")
	rig.carrier.ratesFor = map[string][]shipping.Rate{
		rig.whA.Line1: {{Carrier: "delhivery", Service: "Standard", Price: decimal.NewFromInt(5), CurrencyCode: "USD", EstimatedDays: 1}},
		rig.whB.Line1: {{Carrier: "delhivery", Service: "Standard", Price: decimal.NewFromInt(7), CurrencyCode: "USD", EstimatedDays: 2}},
	}

	// What the rates endpoint quotes for this cart.
	rec := rig.postRates(t, rateRequestBody(rig.variantID, 5))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var parsed struct {
		Data []map[string]any `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &parsed))
	require.Len(t, parsed.Data, 1)
	quotedPrice := parsed.Data[0]["price"].(string)
	require.Equal(t, "12.00", quotedPrice, "sanity: this is the split-origin sum, not a single origin's rate")

	// What checkout charges for the SAME cart, same service.
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	ceh := NewCheckoutExtHandler(rig.db, nil, nil, nil, crypto.NewNoopEncryptor(), logger).
		WithCarrierFactory(func(string) (shipping.Carrier, bool) { return rig.carrier, true })

	variantID := rig.variantID
	req := CheckoutExtRequest{
		Items: []CheckoutItemRequest{{
			VariantID:     &variantID,
			TitleSnapshot: "Split Origin Widget",
			SKUSnapshot:   "SKU-1",
			UnitPrice:     decimal.NewFromInt(10),
			Quantity:      5,
			LineTotal:     decimal.NewFromInt(50),
			CurrencyCode:  "USD",
		}},
		ShippingAddress: CheckoutAddressRequest{
			Name: "Buyer", Line1: "1 Buyer St", City: "Buyer City", CountryCode: "SO",
		},
		ShippingService: "Standard",
		Subtotal:        decimal.NewFromInt(50),
	}
	sc := &country.SupportedCountry{ShippingCarriers: pq.StringArray{"delhivery"}}

	price, provider, err := ceh.calculateShipping(context.Background(), rig.store, sc, req)
	require.NoError(t, err)
	require.Equal(t, "delhivery", provider)
	require.Equal(t, quotedPrice, price.StringFixed(2),
		"checkout must charge exactly what GetRates quoted for the same split cart")
}
