//go:build integration

// Package admin — read-path coverage for #177 (the cheap half) at the
// syncWarehouseAsync site in settings.go.
//
// syncWarehouseAsync builds the shipping.Warehouse pushed to the carrier
// via WarehouseSyncer.UpsertWarehouse. Before this change it read the
// legacy warehouse_* columns unconditionally; now
// (*ShippingSettingsHandler).resolveWarehouseForSync must prefer the
// store-level warehouses row when the config's warehouse_id is set, and
// fall back to the legacy columns otherwise. This is also the site where
// ContactPerson and Email — which have no legacy-column equivalent — flow
// into shipping.Warehouse for the first time (#177's stated bonus fix for
// the "ClientWarehouse matching query does not exist" Delhivery failure).
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

func legacySettingsCarrierConfigRow(storeID, tenantID string, warehouseID *uuid.UUID) ShippingCarrierConfigRow {
	return ShippingCarrierConfigRow{
		TenantID:         uuid.MustParse(tenantID),
		StoreID:          uuid.MustParse(storeID),
		Provider:         "delhivery",
		APIKeyEncrypted:  "legacy-key",
		Mode:             "test",
		IsActive:         true,
		WarehouseName:    "Legacy Warehouse",
		WarehouseLine1:   "1 Legacy Lane",
		WarehouseCity:    "Legacy City",
		WarehouseRegion:  "LG",
		WarehousePostal:  "111111",
		WarehouseCountry: "IN",
		WarehousePhone:   "+911111111111",
		WarehouseID:      warehouseID,
	}
}

// TestResolveWarehouseForSync_PrefersTheWarehousesRowAndCarriesContactAndEmail
// pins both halves of the read fix in one place: the warehouses row wins
// over a legacy address that deliberately differs, and ContactPerson +
// Email — which the legacy columns can't express at all — reach the
// shipping.Warehouse handed to the carrier.
func TestResolveWarehouseForSync_PrefersTheWarehousesRowAndCarriesContactAndEmail(t *testing.T) {
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

	cfg := legacySettingsCarrierConfigRow(storeID, tenantID, &whUUID)

	h := newSettingsWarehouseReadHandler(db)
	got := h.resolveWarehouseForSync(context.Background(), cfg)

	require.Equal(t, "99 Warehouses-Table Road", got.Address,
		"must build the address from the warehouses row, not the legacy columns")
	require.Equal(t, "Mumbai", got.City)
	require.Equal(t, "Warehouse Manager", got.ContactPerson,
		"contact_person is what Delhivery clientwarehouse registration needs and had no home before #177")
	require.Equal(t, "warehouse@example.com", got.Email)
}

// TestResolveWarehouseForSync_FallsBackToLegacyColumnsWhenWarehouseIDIsNil
// covers configs saved before #177's write path, or by a writer that
// hasn't been updated — warehouse_id is NULL and the legacy columns are
// all there is. ContactPerson/Email must stay empty, exactly as before
// this change, since the legacy columns never carried them.
func TestResolveWarehouseForSync_FallsBackToLegacyColumnsWhenWarehouseIDIsNil(t *testing.T) {
	db := testdb.NewDB(t, settingsWarehouseReadTables...)
	storeID, tenantID := seedSettingsWarehouseReadStore(t, db)

	cfg := legacySettingsCarrierConfigRow(storeID, tenantID, nil)

	h := newSettingsWarehouseReadHandler(db)
	got := h.resolveWarehouseForSync(context.Background(), cfg)

	require.Equal(t, "Legacy Warehouse", got.Name)
	require.Equal(t, "Legacy City", got.City)
	require.Empty(t, got.ContactPerson)
	require.Empty(t, got.Email)
}

// TestResolveWarehouseForSync_FallsBackToLegacyColumnsWhenWarehouseIDIsDangling
// mirrors the shipments.go label-creation test: a warehouse_id that
// points at nothing must fall back rather than break the background sync.
func TestResolveWarehouseForSync_FallsBackToLegacyColumnsWhenWarehouseIDIsDangling(t *testing.T) {
	db := testdb.NewDB(t, settingsWarehouseReadTables...)
	storeID, tenantID := seedSettingsWarehouseReadStore(t, db)

	dangling := uuid.New()
	cfg := legacySettingsCarrierConfigRow(storeID, tenantID, &dangling)

	h := newSettingsWarehouseReadHandler(db)
	got := h.resolveWarehouseForSync(context.Background(), cfg)

	require.Equal(t, "Legacy Warehouse", got.Name, "a dangling warehouse_id must fall back, not panic or leave the warehouse blank")
	require.Equal(t, "Legacy City", got.City)
}
