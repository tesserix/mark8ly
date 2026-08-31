//go:build integration

// Package storefront — coverage for allocation at order placement (#177 PR 3).
//
// The load-bearing case is the FIRST one: a store with no warehouses must
// behave exactly as it did before this PR. That is not a tidy fallback —
// production has zero warehouse rows, so it is the only path that currently
// runs, and a regression there breaks every checkout.
package storefront

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/product"
	"github.com/mark8ly/marketplace-api/internal/stockhold"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

// seedOrderWithItems creates an order and one order_item per line and
// returns (orderID, []orderItemID).
func seedOrderWithItems(t *testing.T, db *gorm.DB, storeID string, lines []stockLine) (string, []string) {
	t.Helper()
	var tenantID string
	require.NoError(t, db.Raw(`SELECT tenant_id FROM stores WHERE id = ?`, storeID).Row().Scan(&tenantID))

	orderID := uuid.NewString()
	require.NoError(t, db.Exec(
		`INSERT INTO orders (id, tenant_id, store_id, order_number, idempotency_key,
		                     customer_email, currency_code, subtotal, grand_total)
		 VALUES (?, ?, ?, ?, ?, 'buyer@example.com', 'INR', 10.00, 10.00)`,
		orderID, tenantID, storeID, "AL-"+uuid.NewString()[:8], uuid.NewString()).Error)

	ids := make([]string, 0, len(lines))
	for _, l := range lines {
		itemID := uuid.NewString()
		require.NoError(t, db.Exec(
			`INSERT INTO order_items (id, order_id, variant_id, title_snapshot, sku_snapshot,
			                          unit_price, quantity, line_total, currency_code)
			 VALUES (?, ?, ?, 'Item', 'SKU', 10.00, ?, 10.00, 'INR')`,
			itemID, orderID, l.VariantID, l.Quantity).Error)
		ids = append(ids, itemID)
	}
	return orderID, ids
}

// stockUnitsAt reads variant_stock.quantity for one (variant, location) pair.
//
// Named to avoid colliding with the stockAt struct type declared in
// checkout_availability.go — the brief's helper of the same name does not
// compile in this package, since a type and a function cannot share an
// identifier.
func stockUnitsAt(t *testing.T, db *gorm.DB, variantID, locationID string) int {
	t.Helper()
	var q int
	require.NoError(t, db.Raw(
		`SELECT COALESCE((SELECT quantity FROM variant_stock WHERE variant_id = ? AND location_id = ?), -1)`,
		variantID, locationID).Row().Scan(&q))
	return q
}

// THE load-bearing test. Production has zero warehouses; if this regresses,
// every checkout fails.
func TestCommitStock_StoreWithNoWarehousesBehavesExactlyAsBefore(t *testing.T) {
	db := testdb.NewDB(t, "order_allocations", "stock_holds", "order_items", "orders",
		"variant_stock", "product_variants", "products", "stores")
	storeID, variantID := seedAvailStore(t, db)
	require.NoError(t, db.Exec(
		`INSERT INTO variant_stock (variant_id, location_id, quantity, updated_at)
		 VALUES (?, ?, 5, now())`, variantID, product.DefaultLocationID).Error)

	lines := []stockLine{{VariantID: variantID, Quantity: 2}}
	orderID, _ := seedOrderWithItems(t, db, storeID, lines)
	cart := uuid.NewString()

	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		return commitStock(context.Background(), tx, stockhold.NewRepository(), cart, orderID, storeID, lines)
	}))

	require.Equal(t, 3, stockUnitsAt(t, db, variantID, product.DefaultLocationID),
		"the sentinel row must still be the one decremented")

	var allocations int64
	require.NoError(t, db.Raw(
		`SELECT count(*) FROM order_allocations WHERE order_id = ?`, orderID).Scan(&allocations).Error)
	require.Zero(t, allocations,
		"a store with no warehouses has nothing to allocate against — order_allocations.warehouse_id is NOT NULL")
}

func TestCommitStock_AllocatesAcrossWarehousesAndRecordsThem(t *testing.T) {
	db := testdb.NewDB(t, "order_allocations", "stock_holds", "order_items", "orders",
		"variant_stock", "product_variants", "products", "stores")
	storeID, variantID := seedAvailStore(t, db)
	whA := seedWarehouseRow(t, db, storeID, "A")
	whB := seedWarehouseRow(t, db, storeID, "B")
	require.NoError(t, db.Exec(`UPDATE warehouses SET priority = 0 WHERE id = ?`, whA).Error)
	require.NoError(t, db.Exec(`UPDATE warehouses SET priority = 1 WHERE id = ?`, whB).Error)
	require.NoError(t, db.Exec(
		`INSERT INTO variant_stock (variant_id, location_id, quantity, updated_at)
		 VALUES (?, ?, 3, now()), (?, ?, 4, now())`,
		variantID, whA, variantID, whB).Error)

	lines := []stockLine{{VariantID: variantID, Quantity: 5}}
	orderID, itemIDs := seedOrderWithItems(t, db, storeID, lines)

	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		return commitStock(context.Background(), tx, stockhold.NewRepository(), uuid.NewString(),
			orderID, storeID, lines)
	}))

	var got []struct {
		WarehouseID string
		Quantity    int
	}
	require.NoError(t, db.Raw(
		`SELECT warehouse_id, quantity FROM order_allocations
		  WHERE order_item_id = ? ORDER BY quantity DESC`, itemIDs[0]).Scan(&got).Error)
	require.Len(t, got, 2, "5 units from a 3+4 split must record two allocations")
	require.Equal(t, whA, got[0].WarehouseID)
	require.Equal(t, 3, got[0].Quantity)
	require.Equal(t, whB, got[1].WarehouseID)
	require.Equal(t, 2, got[1].Quantity)

	require.Equal(t, 0, stockUnitsAt(t, db, variantID, whA), "the higher-priority warehouse is drained first")
	require.Equal(t, 2, stockUnitsAt(t, db, variantID, whB))
}

// Two order lines carrying the SAME variant. stockLinesFromItems does not
// merge by variant, and stock_holds' ON CONFLICT REPLACES a quantity rather
// than adding to it — so holding per assignment without aggregating would
// reserve only the second line's units and oversell the first.
func TestCommitStock_TwoLinesOfOneVariantDoNotUnderHold(t *testing.T) {
	db := testdb.NewDB(t, "order_allocations", "stock_holds", "order_items", "orders",
		"variant_stock", "product_variants", "products", "stores")
	storeID, variantID := seedAvailStore(t, db)
	whA := seedWarehouseRow(t, db, storeID, "A")
	require.NoError(t, db.Exec(
		`INSERT INTO variant_stock (variant_id, location_id, quantity, updated_at)
		 VALUES (?, ?, 6, now())`, variantID, whA).Error)

	lines := []stockLine{{VariantID: variantID, Quantity: 2}, {VariantID: variantID, Quantity: 3}}
	orderID, itemIDs := seedOrderWithItems(t, db, storeID, lines)

	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		return commitStock(context.Background(), tx, stockhold.NewRepository(), uuid.NewString(),
			orderID, storeID, lines)
	}))

	require.Equal(t, 1, stockUnitsAt(t, db, variantID, whA),
		"6 units less 2 and 3 is 1 — under-holding would have left 4 or 3 here")

	for i, itemID := range itemIDs {
		var q int
		require.NoError(t, db.Raw(
			`SELECT quantity FROM order_allocations WHERE order_item_id = ?`, itemID).Row().Scan(&q))
		require.Equal(t, lines[i].Quantity, q, "each line records its own allocation")
	}
}

func TestCommitStock_UnfillableOrderFailsAndTakesNoStock(t *testing.T) {
	db := testdb.NewDB(t, "order_allocations", "stock_holds", "order_items", "orders",
		"variant_stock", "product_variants", "products", "stores")
	storeID, variantID := seedAvailStore(t, db)
	whA := seedWarehouseRow(t, db, storeID, "A")
	require.NoError(t, db.Exec(
		`INSERT INTO variant_stock (variant_id, location_id, quantity, updated_at)
		 VALUES (?, ?, 2, now())`, variantID, whA).Error)

	lines := []stockLine{{VariantID: variantID, Quantity: 9}}
	orderID, _ := seedOrderWithItems(t, db, storeID, lines)

	err := db.Transaction(func(tx *gorm.DB) error {
		return commitStock(context.Background(), tx, stockhold.NewRepository(), uuid.NewString(),
			orderID, storeID, lines)
	})
	require.Error(t, err, "an order no combination of warehouses can fill must fail the transaction")

	require.Equal(t, 2, stockUnitsAt(t, db, variantID, whA), "a failed checkout must move no stock")
}

// A warehouse whose units sit in two places — the sentinel row that predates
// the backfill, plus a real row written by per-location stock editing. One
// assignment against that warehouse must produce a hold in EACH location,
// adding up to the assignment.
func TestCommitStock_AssignmentSpanningTwoStorageLocationsHoldsInBoth(t *testing.T) {
	db := testdb.NewDB(t, "order_allocations", "stock_holds", "order_items", "orders",
		"variant_stock", "product_variants", "products", "stores")
	storeID, variantID := seedAvailStore(t, db)
	whA := seedWarehouseRow(t, db, storeID, "A")
	require.NoError(t, db.Exec(
		`INSERT INTO variant_stock (variant_id, location_id, quantity, updated_at)
		 VALUES (?, ?, 3, now()), (?, ?, 4, now())`,
		variantID, product.DefaultLocationID, variantID, whA).Error)

	lines := []stockLine{{VariantID: variantID, Quantity: 5}}
	orderID, _ := seedOrderWithItems(t, db, storeID, lines)

	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		return commitStock(context.Background(), tx, stockhold.NewRepository(), uuid.NewString(),
			orderID, storeID, lines)
	}))

	// 5 units drawn from a 3 + 4 breakdown: the sentinel is exhausted and the
	// remainder comes from the real row.
	require.Equal(t, 0, stockUnitsAt(t, db, variantID, product.DefaultLocationID))
	require.Equal(t, 2, stockUnitsAt(t, db, variantID, whA))
}
