//go:build integration

// Package warehouse_test — coverage for migration 000118, the schema half
// of #177's multi-warehouse work.
//
// These assert BEHAVIOUR (what the table refuses) rather than catalogue
// entries: a test that only checks a column exists passes against a table
// whose constraints were all dropped.
package warehouse_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

// seedAllocatableOrder creates the store, warehouse, order and order line an
// order_allocations row needs, and returns their ids.
func seedAllocatableOrder(t *testing.T, db *gorm.DB) (tenantID, storeID, warehouseID, orderID, orderItemID string) {
	t.Helper()
	tenantID, storeID = seedStore(t, db)

	warehouseID = uuid.NewString()
	require.NoError(t, db.Exec(
		`INSERT INTO warehouses (id, tenant_id, store_id, name, line1, city, region, postal_code, country_code, phone)
		 VALUES (?, ?, ?, 'Alloc WH', '1 Dock Rd', 'Mumbai', 'MH', '400001', 'IN', '+912200000000')`,
		warehouseID, tenantID, storeID).Error)

	orderID = uuid.NewString()
	require.NoError(t, db.Exec(
		// customer_email, not email. idempotency_key is NOT NULL with no
		// default and is unique per store, so it gets a fresh uuid.
		`INSERT INTO orders (id, tenant_id, store_id, order_number, idempotency_key,
		                     customer_email, currency_code, subtotal, grand_total)
		 VALUES (?, ?, ?, ?, ?, 'buyer@example.com', 'INR', 100.00, 100.00)`,
		orderID, tenantID, storeID, "AL-"+uuid.NewString()[:8], uuid.NewString()).Error)

	orderItemID = uuid.NewString()
	require.NoError(t, db.Exec(
		`INSERT INTO order_items (id, order_id, title_snapshot, sku_snapshot, unit_price,
		                          quantity, line_total, currency_code)
		 VALUES (?, ?, 'Ink Tee', 'SKU-1', 50.00, 2, 100.00, 'INR')`,
		orderItemID, orderID).Error)

	return tenantID, storeID, warehouseID, orderID, orderItemID
}

func insertAllocation(db *gorm.DB, tenantID, storeID, orderID, orderItemID, warehouseID string, qty int) error {
	return db.Exec(
		`INSERT INTO order_allocations (tenant_id, store_id, order_id, order_item_id, warehouse_id, quantity)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		tenantID, storeID, orderID, orderItemID, warehouseID, qty).Error
}

func TestOrderAllocations_AcceptsARowAndDefaultsShipmentToNull(t *testing.T) {
	db := testdb.NewTx(t)
	tenantID, storeID, warehouseID, orderID, orderItemID := seedAllocatableOrder(t, db)

	require.NoError(t, insertAllocation(db, tenantID, storeID, orderID, orderItemID, warehouseID, 2))

	var shipmentID *string
	require.NoError(t, db.Raw(
		`SELECT shipment_id FROM order_allocations WHERE order_item_id = ?`, orderItemID,
	).Row().Scan(&shipmentID))
	require.Nil(t, shipmentID, "an allocation is unshipped until a label is printed")
}

// A line allocated twice to the same warehouse is a bug, not a top-up: the
// allocator emits one row per (line, warehouse).
func TestOrderAllocations_RejectsASecondRowForTheSameLineAndWarehouse(t *testing.T) {
	db := testdb.NewTx(t)
	tenantID, storeID, warehouseID, orderID, orderItemID := seedAllocatableOrder(t, db)

	require.NoError(t, insertAllocation(db, tenantID, storeID, orderID, orderItemID, warehouseID, 1))
	err := insertAllocation(db, tenantID, storeID, orderID, orderItemID, warehouseID, 1)
	require.Error(t, err, "the (order_item_id, warehouse_id) unique key must refuse the duplicate")
}

func TestOrderAllocations_RejectsZeroQuantity(t *testing.T) {
	db := testdb.NewTx(t)
	tenantID, storeID, warehouseID, orderID, orderItemID := seedAllocatableOrder(t, db)

	err := insertAllocation(db, tenantID, storeID, orderID, orderItemID, warehouseID, 0)
	require.Error(t, err, "an allocation of nothing is meaningless and must be refused")
}

// The FK is the backstop behind the repository's deletion rules (PR 5): a
// path that forgets them must fail loudly rather than orphan a parcel.
func TestOrderAllocations_WarehouseCannotBeDeletedWhileAllocated(t *testing.T) {
	db := testdb.NewTx(t)
	tenantID, storeID, warehouseID, orderID, orderItemID := seedAllocatableOrder(t, db)
	require.NoError(t, insertAllocation(db, tenantID, storeID, orderID, orderItemID, warehouseID, 2))

	err := db.Exec(`DELETE FROM warehouses WHERE id = ?`, warehouseID).Error
	require.Error(t, err, "ON DELETE RESTRICT must refuse while an allocation references the warehouse")
}

func TestWarehouses_PriorityDefaultsToZero(t *testing.T) {
	db := testdb.NewTx(t)
	tenantID, storeID := seedStore(t, db)

	warehouseID := uuid.NewString()
	require.NoError(t, db.Exec(
		`INSERT INTO warehouses (id, tenant_id, store_id, name) VALUES (?, ?, ?, 'Prio WH')`,
		warehouseID, tenantID, storeID).Error)

	var priority int
	require.NoError(t, db.Raw(`SELECT priority FROM warehouses WHERE id = ?`, warehouseID).
		Row().Scan(&priority))
	require.Equal(t, 0, priority, "an unranked warehouse sorts with the rest, not ahead of them")
}
