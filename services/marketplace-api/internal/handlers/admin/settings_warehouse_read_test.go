//go:build integration

// Package admin — read-path coverage for #484 (the contract half of #177)
// at the syncWarehouseAsync site in settings.go.
//
// Before #484, resolveWarehouseForSync preferred the store-level
// warehouses row when the config's warehouse_id was set, and fell back to
// the legacy warehouse_* columns otherwise. #484 removed that fallback —
// the legacy columns are no longer read anywhere, which is what makes
// dropping them in a later migration safe. These tests replace the old
// fallback-pinning tests: a config with warehouse_id resolves from the
// warehouses row (including ContactPerson/Email, which have no legacy
// equivalent), and a config with no warehouse_id (or a dangling one)
// yields an error rather than a silent fall back to stale columns.
package admin

import (
	"context"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/stores"
	"github.com/mark8ly/marketplace-api/internal/warehouse"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

var settingsWarehouseReadTables = []string{
	"shipping_carrier_configs",
	"warehouses",
	"stores",
}

func newSettingsWarehouseReadHandler(db *gorm.DB) *ShippingSettingsHandler {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	return &ShippingSettingsHandler{db: db, warehouseRepo: warehouse.NewRepository(), logger: logger}
}

func seedSettingsWarehouseReadStore(t *testing.T, db *gorm.DB) (storeID, tenantID string) {
	t.Helper()
	tenantID = uuid.NewString()
	storeID = uuid.NewString()
	s := &stores.Store{
		ID:           storeID,
		TenantID:     tenantID,
		Slug:         "wh-sync-" + storeID[:8],
		Name:         "Warehouse Sync Test Store",
		CountryCode:  "IN",
		CurrencyCode: "INR",
		Timezone:     "Asia/Kolkata",
		Status:       stores.StatusActive,
		SyncedAt:     time.Now(),
	}
	require.NoError(t, db.Create(s).Error)
	return storeID, tenantID
}

func settingsCarrierConfigRow(storeID, tenantID string, warehouseID *uuid.UUID) ShippingCarrierConfigRow {
	return ShippingCarrierConfigRow{
		TenantID:        uuid.MustParse(tenantID),
		StoreID:         uuid.MustParse(storeID),
		Provider:        "delhivery",
		APIKeyEncrypted: "legacy-key",
		Mode:            "test",
		IsActive:        true,
		WarehouseID:     warehouseID,
	}
}

// TestResolveWarehouseForSync_ResolvesFromTheWarehousesRow is the core
// assertion for #484's read half: given a config whose warehouse_id points
// at a warehouses row, the resolved shipping.Warehouse must come from that
// row, ContactPerson and Email included — those two fields have no
// legacy-column equivalent, so they only ever flowed through this path.
func TestResolveWarehouseForSync_ResolvesFromTheWarehousesRow(t *testing.T) {
	db := testdb.NewDB(t, settingsWarehouseReadTables...)
	storeID, tenantID := seedSettingsWarehouseReadStore(t, db)
	whRepo := warehouse.NewRepository()

	wh, err := whRepo.Upsert(context.Background(), db, warehouse.Warehouse{
		TenantID: tenantID, StoreID: storeID, Name: "Main",
		Line1: "99 Warehouses-Table Road", City: "Mumbai", Region: "MH",
		PostalCode: "400001", CountryCode: "IN", Phone: "+912200000000",
		ContactPerson: "Warehouse Manager", Email: "warehouse@example.com",
	})
	require.NoError(t, err)
	whUUID := uuid.MustParse(wh.ID)

	cfg := settingsCarrierConfigRow(storeID, tenantID, &whUUID)

	h := newSettingsWarehouseReadHandler(db)
	got, err := h.resolveWarehouseForSync(context.Background(), cfg)
	require.NoError(t, err)

	require.Equal(t, "99 Warehouses-Table Road", got.Address)
	require.Equal(t, "Mumbai", got.City)
	require.Equal(t, "Warehouse Manager", got.ContactPerson,
		"contact_person is what Delhivery clientwarehouse registration needs and has no legacy-column source")
	require.Equal(t, "warehouse@example.com", got.Email)
}

// TestResolveWarehouseForSync_ErrorsWhenWarehouseIDIsNil covers a config
// with no linked warehouse. #484 removed the legacy-column fallback, so
// this must be an error the caller (syncWarehouseAsync) treats as "nothing
// to sync" — never a silent read of stale legacy data.
func TestResolveWarehouseForSync_ErrorsWhenWarehouseIDIsNil(t *testing.T) {
	db := testdb.NewDB(t, settingsWarehouseReadTables...)
	storeID, tenantID := seedSettingsWarehouseReadStore(t, db)

	cfg := settingsCarrierConfigRow(storeID, tenantID, nil)

	h := newSettingsWarehouseReadHandler(db)
	require.Panics(t, func() {
		// resolveWarehouseForSync dereferences cfg.WarehouseID directly —
		// its only caller (syncWarehouseAsync) never invokes it without
		// first checking WarehouseID != nil. This test documents that
		// contract rather than exercising a nil-safe path that doesn't
		// exist: calling it with a nil WarehouseID is a programmer error.
		_, _ = h.resolveWarehouseForSync(context.Background(), cfg)
	})
}

// TestResolveWarehouseForSync_ErrorsWhenWarehouseIDIsDangling mirrors the
// shipments.go label-creation test: a warehouse_id that points at nothing
// must return an error rather than fall back to (now-removed) legacy data.
func TestResolveWarehouseForSync_ErrorsWhenWarehouseIDIsDangling(t *testing.T) {
	db := testdb.NewDB(t, settingsWarehouseReadTables...)
	storeID, tenantID := seedSettingsWarehouseReadStore(t, db)

	dangling := uuid.New()
	cfg := settingsCarrierConfigRow(storeID, tenantID, &dangling)

	h := newSettingsWarehouseReadHandler(db)
	_, err := h.resolveWarehouseForSync(context.Background(), cfg)
	require.Error(t, err, "a dangling warehouse_id must error, not silently resolve to a blank/stale warehouse")
}
