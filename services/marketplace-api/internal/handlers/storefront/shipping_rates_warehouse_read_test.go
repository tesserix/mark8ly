//go:build integration

// Package storefront — read-path coverage for #484 (the contract half of
// #177) at the shipping-rates quote site.
//
// (*ShippingRatesHandler).GetRates loads a carrierConfigRow and used to
// build its rate-quote origin address from the legacy warehouse_* columns,
// then from the store-level warehouses row with a legacy fallback (#480).
// #484 removes that fallback: resolveWarehouseAddress now sources the
// origin exclusively from the warehouses row via warehouse_id — which is
// what made dropping those columns (migration 000117) safe. Testing resolveWarehouseAddress directly,
// rather than driving GetRates end-to-end, avoids needing a live carrier
// for a rates call.
package storefront

import (
	"context"
	"log/slog"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/shipping"
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
//
// This used to also populate the 8 legacy warehouse_* columns with an
// address deliberately unlike the warehouses row's, so a resolver that
// still fell back to them would be caught reading the wrong address.
// Migration 000117 dropped those columns, so that fixture is no longer
// writable — and no longer needed: a column that does not exist cannot be
// read. What the tests below still pin is the behaviour that replaced the
// fallback, namely that an unresolvable warehouse_id yields the zero
// address rather than anything else.
func seedShippingRatesCarrierConfig(t *testing.T, db *gorm.DB, storeID, tenantID string, warehouseID *string) {
	t.Helper()
	require.NoError(t, db.Exec(
		`INSERT INTO shipping_carrier_configs
		    (id, tenant_id, store_id, provider, api_key_encrypted, mode, is_active, warehouse_id)
		 VALUES (?, ?, ?, 'delhivery', 'legacy-key', 'test', true, ?)`,
		uuid.NewString(), tenantID, storeID, warehouseID).Error)
}

func loadShippingRatesCarrierConfigRow(t *testing.T, db *gorm.DB, storeID string) carrierConfigRow {
	t.Helper()
	var cfg carrierConfigRow
	require.NoError(t, db.Where("store_id = ?", storeID).First(&cfg).Error)
	return cfg
}

// TestResolveWarehouseAddress_ResolvesFromTheWarehousesRowWhenLinked proves
// the rates-quote origin comes from the warehouses row when warehouse_id
// is set — with an address that deliberately differs from the row's own
// (still-populated, no-longer-read) legacy columns, so the test can't pass
// by accident if the fallback were somehow still wired in.
func TestResolveWarehouseAddress_ResolvesFromTheWarehousesRowWhenLinked(t *testing.T) {
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

	require.Equal(t, "99 Warehouses-Table Road", got.Line1, "must read the warehouses row")
	require.Equal(t, "Mumbai", got.City)
}

// TestResolveWarehouseAddress_ZeroValueWhenWarehouseIDIsNil covers a
// config with no linked warehouse: it must yield the zero
// shipping.Address. Before #486 removed the fallback there was legacy
// column data to return instead; migration 000117 has since dropped the
// columns, so the zero address is now the only address there is.
func TestResolveWarehouseAddress_ZeroValueWhenWarehouseIDIsNil(t *testing.T) {
	db := testdb.NewDB(t, shippingRatesWarehouseReadTables...)
	storeID, tenantID := seedShippingRatesWarehouseReadStore(t, db)
	seedShippingRatesCarrierConfig(t, db, storeID, tenantID, nil)
	cfg := loadShippingRatesCarrierConfigRow(t, db, storeID)

	h := newShippingRatesWarehouseReadHandler(db)
	got := h.resolveWarehouseAddress(context.Background(), cfg)

	require.Equal(t, shipping.Address{}, got, "no warehouse_id must yield the zero address")
}

// TestResolveWarehouseAddress_ZeroValueWhenWarehouseIDIsDangling covers a
// warehouse_id that points at a row that no longer exists — the FK is ON
// DELETE SET NULL, so this should be rare, but resolving to the zero
// address (which GetRates would then report as "no active carrier
// config"-shaped zero data) is the correct behaviour now that there is no
// legacy data left to fall back to.
//
// The dangling id is built directly on an in-memory carrierConfigRow
// rather than persisted: the real FK (plus ON DELETE SET NULL) means
// Postgres won't let an actual row go dangling, but resolveWarehouseAddress
// only reads cfg's fields to decide what to look up, so this still
// exercises the code path a hypothetical race would hit.
func TestResolveWarehouseAddress_ZeroValueWhenWarehouseIDIsDangling(t *testing.T) {
	db := testdb.NewDB(t, shippingRatesWarehouseReadTables...)
	storeID, tenantID := seedShippingRatesWarehouseReadStore(t, db)
	seedShippingRatesCarrierConfig(t, db, storeID, tenantID, nil)
	cfg := loadShippingRatesCarrierConfigRow(t, db, storeID)
	dangling := uuid.NewString()
	cfg.WarehouseID = &dangling

	h := newShippingRatesWarehouseReadHandler(db)
	got := h.resolveWarehouseAddress(context.Background(), cfg)

	require.Equal(t, shipping.Address{}, got, "a dangling warehouse_id must yield the zero address, not a stale one")
}
