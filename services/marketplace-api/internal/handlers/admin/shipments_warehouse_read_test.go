//go:build integration

// Package admin — read-path coverage for #484 (the contract half of #177)
// at the shipments.go label-creation site.
//
// #177's write path (cf71fe67) taught the admin settings save to upsert a
// store-level warehouses row and point shipping_carrier_configs.warehouse_id
// at it, while still writing the legacy warehouse_* columns (the expand
// half). #480 made resolvePickupAddress prefer warehouse_id with a fallback
// to those columns. #484 removes that fallback: it is no longer read
// anywhere, which is what makes dropping the columns in a later migration
// safe. These tests replace the old fallback-pinning tests: a config with
// warehouse_id resolves the pickup address from the warehouses row, and a
// config with no warehouse_id (or a dangling one) yields the ZERO address —
// not a silent read of stale legacy data.
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

// carrierConfigForRead builds a shipping_carrier_configs row. warehouseID,
// when non-nil, links it to a warehouses row.
func carrierConfigForRead(storeID, tenantID string, warehouseID *uuid.UUID) *shipping.CarrierConfig {
	storeUUID := uuid.MustParse(storeID)
	tenantUUID := uuid.MustParse(tenantID)
	return &shipping.CarrierConfig{
		TenantID:    tenantUUID,
		StoreID:     storeUUID,
		Provider:    "delhivery",
		APIKey:      "legacy-key",
		Mode:        "test",
		Enabled:     true,
		HandlingFee: decimal.Zero,
		WarehouseID: warehouseID,
	}
}

// TestResolvePickupAddress_ResolvesFromTheWarehousesRowWhenLinked is the
// core assertion for #484's read half: a config whose warehouse_id points
// at a warehouses row resolves its pickup address, ContactPerson included,
// from that row.
func TestResolvePickupAddress_ResolvesFromTheWarehousesRowWhenLinked(t *testing.T) {
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

	cfg := carrierConfigForRead(storeID, tenantID, &whUUID)
	require.NoError(t, db.Create(cfg).Error)

	h := newShipmentsWarehouseReadHandler(db)
	got := h.resolvePickupAddress(context.Background(), cfg)

	require.Equal(t, "99 Warehouses-Table Road", got.Line1)
	require.Equal(t, "Mumbai", got.City)
	require.Equal(t, "Warehouse Manager", got.ContactPerson)
}

// TestResolvePickupAddress_ZeroValueWhenWarehouseIDIsNil is the case #484
// changes: a config with no linked warehouse (written before #177's write
// path landed, or never given one) must resolve to the ZERO pickupAddress,
// not fall back to any stored data. That zero value is exactly what makes
// the "warehouse address is not configured" validation in CreateShipment
// still fire correctly.
func TestResolvePickupAddress_ZeroValueWhenWarehouseIDIsNil(t *testing.T) {
	db := testdb.NewDB(t, shipmentsWarehouseReadTables...)
	storeID, tenantID := seedShipmentsWarehouseReadStore(t, db)

	cfg := carrierConfigForRead(storeID, tenantID, nil)
	require.NoError(t, db.Create(cfg).Error)

	h := newShipmentsWarehouseReadHandler(db)
	got := h.resolvePickupAddress(context.Background(), cfg)

	require.Equal(t, pickupAddress{}, got, "no warehouse_id must yield the zero address, not a silent legacy fallback")
}

// TestResolvePickupAddress_ZeroValueWhenWarehouseIDIsDangling covers the
// rare case the FK's ON DELETE SET NULL is meant to prevent but cannot
// fully rule out under concurrent writes: warehouse_id is set but points
// at nothing. #484 removed the legacy-column fallback this used to take,
// so it must resolve to the zero address (and log), not error the request
// outright — shipping still fails downstream on the "not configured" check,
// which is the correct outcome for a config with no real address on file.
//
// The dangling id is never persisted on the config row: warehouse_id has a
// real FK to warehouses, so Postgres refuses to let a genuine row point at
// a nonexistent one. resolvePickupAddress only reads cfg's in-memory
// fields to decide what to look up, so building the dangling reference in
// Go — without persisting the config row — still exercises exactly the
// code path a hypothetical race would hit.
func TestResolvePickupAddress_ZeroValueWhenWarehouseIDIsDangling(t *testing.T) {
	db := testdb.NewDB(t, shipmentsWarehouseReadTables...)
	storeID, tenantID := seedShipmentsWarehouseReadStore(t, db)

	dangling := uuid.New()
	cfg := carrierConfigForRead(storeID, tenantID, &dangling)

	h := newShipmentsWarehouseReadHandler(db)
	got := h.resolvePickupAddress(context.Background(), cfg)

	require.Equal(t, pickupAddress{}, got, "a dangling warehouse_id must yield the zero address, not error or fall back")
}
