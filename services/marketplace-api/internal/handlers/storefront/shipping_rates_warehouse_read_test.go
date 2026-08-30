//go:build integration

// Package storefront — read-path coverage for #177 (the cheap half) at
// the shipping-rates quote site.
//
// (*ShippingRatesHandler).GetRates loads a carrierConfigRow and used to
// build its rate-quote origin address exclusively from the legacy
// warehouse_* columns. It must now prefer the store-level warehouses row
// via warehouse_id, falling back to the legacy columns otherwise — see
// resolveWarehouseAddress. Testing that function directly, rather than
// driving GetRates end-to-end, avoids needing a live carrier for a rates
// call; resolveWarehouseAddress is the single seam that decides which
// address source wins.
package storefront

import (
	"context"
	"log/slog"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/warehouse"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

var shippingRatesWarehouseReadTables = []string{
	"shipping_carrier_configs",
	"warehouses",
	"stores",
}

func newShippingRatesWarehouseReadHandler(db *gorm.DB) *ShippingRatesHandler {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	return &ShippingRatesHandler{db: db, warehouseRepo: warehouse.NewRepository(), logger: logger}
}

func seedShippingRatesWarehouseReadStore(t *testing.T, db *gorm.DB) (storeID, tenantID string) {
	t.Helper()
	tenantID = uuid.NewString()
	storeID = uuid.NewString()
	require.NoError(t, db.Exec(
		`INSERT INTO stores (id, tenant_id, name, slug, status, country_code, currency_code, timezone,
		                     storefront_customer_portal_secret)
		 VALUES (?, ?, 'WH Rates Test', ?, 'active', 'IN', 'INR', 'Asia/Kolkata', ?)`,
		storeID, tenantID, "wh-rates-"+storeID[:8], uuid.NewString()).Error)
	return storeID, tenantID
}

// seedShippingRatesCarrierConfig inserts a shipping_carrier_configs row via
// raw SQL rather than the carrierConfigRow GORM model — that model
// deliberately omits store_id (it's only ever used for reads keyed by the
// gin-context store), so it can't Create() a row that satisfies the
// table's NOT NULL store_id.
func seedShippingRatesCarrierConfig(t *testing.T, db *gorm.DB, storeID, tenantID string, warehouseID *string) {
	t.Helper()
	require.NoError(t, db.Exec(
		`INSERT INTO shipping_carrier_configs
		    (id, tenant_id, store_id, provider, api_key_encrypted, mode, is_active,
		     warehouse_name, warehouse_line1, warehouse_city, warehouse_region,
		     warehouse_postal, warehouse_country, warehouse_phone, warehouse_id)
		 VALUES (?, ?, ?, 'delhivery', 'legacy-key', 'test', true,
		         'Legacy Warehouse', '1 Legacy Lane', 'Legacy City', 'LG',
		         '111111', 'IN', '+911111111111', ?)`,
		uuid.NewString(), tenantID, storeID, warehouseID).Error)
}

func loadShippingRatesCarrierConfigRow(t *testing.T, db *gorm.DB, storeID string) carrierConfigRow {
	t.Helper()
	var cfg carrierConfigRow
	require.NoError(t, db.Where("store_id = ?", storeID).First(&cfg).Error)
	return cfg
}

// TestResolveWarehouseAddress_PrefersTheWarehousesRowWhenLinked proves the
// rates-quote origin comes from the warehouses row when warehouse_id is
// set — with an address that deliberately differs from the legacy
// columns, so the test can't pass by accident regardless of which source
// actually won.
func TestResolveWarehouseAddress_PrefersTheWarehousesRowWhenLinked(t *testing.T) {
	db := testdb.NewDB(t, shippingRatesWarehouseReadTables...)
	storeID, tenantID := seedShippingRatesWarehouseReadStore(t, db)
	whRepo := warehouse.NewRepository()

	wh, err := whRepo.Upsert(context.Background(), db, warehouse.Warehouse{
		TenantID: tenantID, StoreID: storeID, Name: "Main",
		Line1: "99 Warehouses-Table Road", City: "Mumbai", Region: "MH",
		PostalCode: "400001", CountryCode: "IN", Phone: "+912200000000",
	})
	require.NoError(t, err)
	seedShippingRatesCarrierConfig(t, db, storeID, tenantID, &wh.ID)
	cfg := loadShippingRatesCarrierConfigRow(t, db, storeID)

	h := newShippingRatesWarehouseReadHandler(db)
	got := h.resolveWarehouseAddress(context.Background(), cfg)

	require.Equal(t, "99 Warehouses-Table Road", got.Line1, "must read the warehouses row, not the legacy column")
	require.Equal(t, "Mumbai", got.City)
}

// TestResolveWarehouseAddress_FallsBackToLegacyColumnsWhenWarehouseIDIsNil
// covers a config saved before #177's write path, or by a writer that
// hasn't been updated — warehouse_id is NULL.
func TestResolveWarehouseAddress_FallsBackToLegacyColumnsWhenWarehouseIDIsNil(t *testing.T) {
	db := testdb.NewDB(t, shippingRatesWarehouseReadTables...)
	storeID, tenantID := seedShippingRatesWarehouseReadStore(t, db)
	seedShippingRatesCarrierConfig(t, db, storeID, tenantID, nil)
	cfg := loadShippingRatesCarrierConfigRow(t, db, storeID)

	h := newShippingRatesWarehouseReadHandler(db)
	got := h.resolveWarehouseAddress(context.Background(), cfg)

	require.Equal(t, "1 Legacy Lane", got.Line1)
	require.Equal(t, "Legacy City", got.City)
}

// TestResolveWarehouseAddress_FallsBackToLegacyColumnsWhenWarehouseIDIsDangling
// covers a warehouse_id that points at a row that no longer exists — the
// FK is ON DELETE SET NULL, so this should be rare, but resolving to
// "no quote" rather than the legacy address would be a worse failure mode
// for what is a best-effort estimate anyway.
//
// The dangling id is built directly on an in-memory carrierConfigRow
// rather than persisted: the real FK (plus ON DELETE SET NULL) means
// Postgres won't let an actual row go dangling, but
// resolveWarehouseAddress only reads cfg's fields to decide what to look
// up, so this still exercises the fallback a hypothetical race would hit.
func TestResolveWarehouseAddress_FallsBackToLegacyColumnsWhenWarehouseIDIsDangling(t *testing.T) {
	db := testdb.NewDB(t, shippingRatesWarehouseReadTables...)
	storeID, tenantID := seedShippingRatesWarehouseReadStore(t, db)
	seedShippingRatesCarrierConfig(t, db, storeID, tenantID, nil)
	cfg := loadShippingRatesCarrierConfigRow(t, db, storeID)
	dangling := uuid.NewString()
	cfg.WarehouseID = &dangling

	h := newShippingRatesWarehouseReadHandler(db)
	got := h.resolveWarehouseAddress(context.Background(), cfg)

	require.Equal(t, "1 Legacy Lane", got.Line1, "a dangling warehouse_id must fall back, not return a blank origin")
	require.Equal(t, "Legacy City", got.City)
}
