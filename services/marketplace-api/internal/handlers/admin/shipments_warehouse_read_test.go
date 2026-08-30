//go:build integration

// Package admin — read-path coverage for #177 (the cheap half) at the
// shipments.go label-creation site.
//
// cf71fe67 taught the admin settings save to upsert a store-level
// warehouses row and point shipping_carrier_configs.warehouse_id at it,
// while STILL writing the legacy warehouse_* columns (the expand half).
// This file pins the corresponding read: (*ShipmentsHandler).
// resolvePickupAddress must prefer the warehouses row over the legacy
// columns when warehouse_id is set, and must fall back to the legacy
// columns — never error — when it is NULL or dangling. Driving this
// through the full label-creation HTTP handler would require a live
// carrier; resolvePickupAddress is the single seam that decides which
// source wins, so it is tested directly here, in-package.
package admin

import (
	"context"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/shipping"
	"github.com/mark8ly/marketplace-api/internal/stores"
	"github.com/mark8ly/marketplace-api/internal/warehouse"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

var shipmentsWarehouseReadTables = []string{
	"shipping_carrier_configs",
	"warehouses",
	"stores",
}

func newShipmentsWarehouseReadHandler(db *gorm.DB) *ShipmentsHandler {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	return &ShipmentsHandler{db: db, warehouseRepo: warehouse.NewRepository(), logger: logger}
}

func seedShipmentsWarehouseReadStore(t *testing.T, db *gorm.DB) (storeID, tenantID string) {
	t.Helper()
	tenantID = uuid.NewString()
	storeID = uuid.NewString()
	s := &stores.Store{
		ID:           storeID,
		TenantID:     tenantID,
		Slug:         "wh-read-" + storeID[:8],
		Name:         "Warehouse Read Test Store",
		CountryCode:  "IN",
		CurrencyCode: "INR",
		Timezone:     "Asia/Kolkata",
		Status:       stores.StatusActive,
		SyncedAt:     time.Now(),
	}
	require.NoError(t, db.Create(s).Error)
	return storeID, tenantID
}

// legacyCarrierConfig builds a shipping_carrier_configs row with the
// legacy warehouse_* columns populated. warehouseID, when non-nil, links
// it to a warehouses row.
func legacyCarrierConfig(storeID, tenantID string, warehouseID *uuid.UUID) *shipping.CarrierConfig {
	storeUUID := uuid.MustParse(storeID)
	tenantUUID := uuid.MustParse(tenantID)
	return &shipping.CarrierConfig{
		TenantID:         tenantUUID,
		StoreID:          storeUUID,
		Provider:         "delhivery",
		APIKey:           "legacy-key",
		Mode:             "test",
		Enabled:          true,
		WarehouseName:    "Legacy Warehouse",
		WarehouseLine1:   "1 Legacy Lane",
		WarehouseCity:    "Legacy City",
		WarehouseRegion:  "LG",
		WarehousePostal:  "111111",
		WarehouseCountry: "IN",
		WarehousePhone:   "+911111111111",
		HandlingFee:      decimal.Zero,
		WarehouseID:      warehouseID,
	}
}

// TestResolvePickupAddress_PrefersTheWarehousesRowWhenLinked is the core
// assertion for #177's read half: given a config whose warehouse_id points
// at a warehouses row with a DIFFERENT address than the legacy columns,
// the resolved address must come from the warehouses row. Making the two
// addresses differ is deliberate — a test where they agree would pass
// whichever source the code actually reads from.
func TestResolvePickupAddress_PrefersTheWarehousesRowWhenLinked(t *testing.T) {
	db := testdb.NewDB(t, shipmentsWarehouseReadTables...)
	storeID, tenantID := seedShipmentsWarehouseReadStore(t, db)
	whRepo := warehouse.NewRepository()

	wh, err := whRepo.Upsert(context.Background(), db, warehouse.Warehouse{
		TenantID: tenantID, StoreID: storeID, Name: "Main",
		Line1: "99 Warehouses-Table Road", City: "Mumbai", Region: "MH",
		PostalCode: "400001", CountryCode: "IN", Phone: "+912200000000",
		ContactPerson: "Warehouse Manager",
	})
	require.NoError(t, err)
	whUUID := uuid.MustParse(wh.ID)

	cfg := legacyCarrierConfig(storeID, tenantID, &whUUID)
	require.NoError(t, db.Create(cfg).Error)

	h := newShipmentsWarehouseReadHandler(db)
	got := h.resolvePickupAddress(context.Background(), cfg)

	require.Equal(t, "99 Warehouses-Table Road", got.Line1, "must read the warehouses row, not the legacy column")
	require.Equal(t, "Mumbai", got.City)
	require.Equal(t, "Warehouse Manager", got.ContactPerson,
		"contact_person has no legacy equivalent — it only flows through when the warehouses row wins")
}

// TestResolvePickupAddress_FallsBackToLegacyColumnsWhenWarehouseIDIsNil
// covers every config written before #177's write path landed (or by a
// writer that still hasn't been updated) — warehouse_id is NULL and the
// legacy columns are the only data there is.
func TestResolvePickupAddress_FallsBackToLegacyColumnsWhenWarehouseIDIsNil(t *testing.T) {
	db := testdb.NewDB(t, shipmentsWarehouseReadTables...)
	storeID, tenantID := seedShipmentsWarehouseReadStore(t, db)

	cfg := legacyCarrierConfig(storeID, tenantID, nil)
	require.NoError(t, db.Create(cfg).Error)

	h := newShipmentsWarehouseReadHandler(db)
	got := h.resolvePickupAddress(context.Background(), cfg)

	require.Equal(t, "1 Legacy Lane", got.Line1)
	require.Equal(t, "Legacy City", got.City)
	require.Empty(t, got.ContactPerson, "the legacy columns have no contact_person equivalent")
}

// TestResolvePickupAddress_FallsBackToLegacyColumnsWhenWarehouseIDIsDangling
// simulates the rare case the FK's ON DELETE SET NULL is meant to prevent
// but cannot fully rule out under concurrent writes: warehouse_id is set
// but points at nothing. Shipping a label from the last-known-good
// (legacy) address matters more than failing the request.
//
// The dangling id is never persisted on the config row: warehouse_id has
// a real FK to warehouses, so Postgres itself refuses to let a genuine
// row point at a nonexistent one (and ON DELETE SET NULL means it can't
// go dangling by deleting the parent, either). resolvePickupAddress only
// reads cfg's in-memory fields to decide what to look up, so building the
// dangling reference in Go — without persisting the config row — still
// exercises exactly the code path a hypothetical race would hit.
func TestResolvePickupAddress_FallsBackToLegacyColumnsWhenWarehouseIDIsDangling(t *testing.T) {
	db := testdb.NewDB(t, shipmentsWarehouseReadTables...)
	storeID, tenantID := seedShipmentsWarehouseReadStore(t, db)

	dangling := uuid.New()
	cfg := legacyCarrierConfig(storeID, tenantID, &dangling)

	h := newShipmentsWarehouseReadHandler(db)
	got := h.resolvePickupAddress(context.Background(), cfg)

	require.Equal(t, "1 Legacy Lane", got.Line1,
		"a dangling warehouse_id must fall back, not error the request")
	require.Equal(t, "Legacy City", got.City)
}
