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
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/product"
	"github.com/mark8ly/marketplace-api/internal/stockhold"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

// seedOrderWithItems creates an order and its order_items in ONE multi-row
// INSERT, mirroring internal/order/repository.go's batch tx.Create(&items):
// production inserts all of an order's items in a single statement, so they
// share one created_at and the only tie-break Postgres has is the random
// gen_random_uuid() id. A test that inserted items with separate statements
// (and therefore strictly increasing created_at) would stay green against a
// production-broken assumption that order_items come back in line order.
// Returns (orderID, []orderItemID) in the SAME order as lines.
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
	placeholders := make([]string, 0, len(lines))
	args := make([]interface{}, 0, len(lines)*9)
	for _, l := range lines {
		itemID := uuid.NewString()
		ids = append(ids, itemID)
		placeholders = append(placeholders, "(?, ?, ?, 'Item', 'SKU', 10.00, ?, 10.00, 'INR')")
		args = append(args, itemID, orderID, l.VariantID, l.Quantity)
	}
	require.NoError(t, db.Exec(
		`INSERT INTO order_items (id, order_id, variant_id, title_snapshot, sku_snapshot,
		                          unit_price, quantity, line_total, currency_code)
		 VALUES `+strings.Join(placeholders, ", "), args...).Error)
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

// seedAvailVariant creates one more product+variant for an existing store,
// for tests that need two distinct variants.
func seedAvailVariant(t *testing.T, db *gorm.DB, storeID string) string {
	t.Helper()
	var tenantID string
	require.NoError(t, db.Raw(`SELECT tenant_id FROM stores WHERE id = ?`, storeID).Row().Scan(&tenantID))

	productID := uuid.NewString()
	require.NoError(t, db.Exec(
		`INSERT INTO products (id, tenant_id, store_id, title, handle, status, vendor_id, published_at)
		 VALUES (?, ?, ?, 'Avail Product 2', ?, 'active', ?, now())`,
		productID, tenantID, storeID, "avail-"+uuid.NewString()[:8], uuid.NewString()).Error)

	variantID := uuid.NewString()
	require.NoError(t, db.Exec(
		`INSERT INTO product_variants (id, product_id, store_id, sku, price, currency_code)
		 VALUES (?, ?, ?, ?, 10.00, 'INR')`,
		variantID, productID, storeID, "SKU-"+uuid.NewString()[:8]).Error)
	return variantID
}

// order_items are inserted in ONE batch statement (seedOrderWithItems
// mirrors production's tx.Create(&items)), so all rows share one created_at
// and Postgres's tie-break — a random UUID — is the only thing left to sort
// by. recordAllocations must not rely on that order: it must find each
// line's item by (variant_id, quantity), and its per-warehouse attribution
// must not confuse two DIFFERENT variants split across the SAME two
// warehouses.
func TestCommitStock_RecordsAllocationsForTwoDifferentVariantsAcrossWarehouses(t *testing.T) {
	db := testdb.NewDB(t, "order_allocations", "stock_holds", "order_items", "orders",
		"variant_stock", "product_variants", "products", "stores")
	storeID, variantX := seedAvailStore(t, db)
	variantY := seedAvailVariant(t, db, storeID)
	whA := seedWarehouseRow(t, db, storeID, "A")
	whB := seedWarehouseRow(t, db, storeID, "B")
	require.NoError(t, db.Exec(`UPDATE warehouses SET priority = 0 WHERE id = ?`, whA).Error)
	require.NoError(t, db.Exec(`UPDATE warehouses SET priority = 1 WHERE id = ?`, whB).Error)
	require.NoError(t, db.Exec(
		`INSERT INTO variant_stock (variant_id, location_id, quantity, updated_at) VALUES
		   (?, ?, 2, now()), (?, ?, 1, now()),
		   (?, ?, 1, now()), (?, ?, 2, now())`,
		variantX, whA, variantX, whB,
		variantY, whA, variantY, whB).Error)

	// X: 2 at A, 1 at B — needs both. Y: 1 at A, 2 at B — needs both too,
	// so both variants' assignments interleave across the same warehouses.
	lines := []stockLine{{VariantID: variantX, Quantity: 3}, {VariantID: variantY, Quantity: 3}}
	orderID, itemIDs := seedOrderWithItems(t, db, storeID, lines)

	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		return commitStock(context.Background(), tx, stockhold.NewRepository(), uuid.NewString(),
			orderID, storeID, lines)
	}))

	assertAllocs := func(itemID string, want map[string]int) {
		t.Helper()
		var got []struct {
			WarehouseID string
			Quantity    int
		}
		require.NoError(t, db.Raw(
			`SELECT warehouse_id, quantity FROM order_allocations WHERE order_item_id = ?`, itemID).
			Scan(&got).Error)
		have := map[string]int{}
		for _, g := range got {
			have[g.WarehouseID] = g.Quantity
		}
		require.Equal(t, want, have)
	}

	// itemIDs[0] is line0 (variantX): 2 from A, 1 from B.
	assertAllocs(itemIDs[0], map[string]int{whA: 2, whB: 1})
	// itemIDs[1] is line1 (variantY): 1 from A, 2 from B.
	assertAllocs(itemIDs[1], map[string]int{whA: 1, whB: 2})

	require.Equal(t, 0, stockUnitsAt(t, db, variantX, whA))
	require.Equal(t, 0, stockUnitsAt(t, db, variantX, whB))
	require.Equal(t, 0, stockUnitsAt(t, db, variantY, whA))
	require.Equal(t, 0, stockUnitsAt(t, db, variantY, whB))
}

// Two lines of the SAME variant but DIFFERENT quantities. Their order_items
// rows are indistinguishable by variant alone, but the (variant, quantity)
// pair is unique to each line here, so this proves recordAllocations pairs
// by that key rather than by whatever order order_items happens to load in.
func TestCommitStock_TwoLinesOfSameVariantDifferentQuantitiesAttributeCorrectly(t *testing.T) {
	db := testdb.NewDB(t, "order_allocations", "stock_holds", "order_items", "orders",
		"variant_stock", "product_variants", "products", "stores")
	storeID, variantID := seedAvailStore(t, db)
	whA := seedWarehouseRow(t, db, storeID, "A")
	require.NoError(t, db.Exec(
		`INSERT INTO variant_stock (variant_id, location_id, quantity, updated_at)
		 VALUES (?, ?, 10, now())`, variantID, whA).Error)

	lines := []stockLine{{VariantID: variantID, Quantity: 2}, {VariantID: variantID, Quantity: 5}}
	orderID, itemIDs := seedOrderWithItems(t, db, storeID, lines)

	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		return commitStock(context.Background(), tx, stockhold.NewRepository(), uuid.NewString(),
			orderID, storeID, lines)
	}))

	for i, itemID := range itemIDs {
		var q int
		require.NoError(t, db.Raw(
			`SELECT quantity FROM order_allocations WHERE order_item_id = ?`, itemID).Row().Scan(&q))
		require.Equal(t, lines[i].Quantity, q,
			"each item's allocation must match ITS OWN line's quantity, not the other line's")
	}
}

// A continue-policy (sell-past-zero) variant with NO variant_stock row at
// all — the common case, since it never needed one to sell — must succeed
// on the allocation path exactly as the legacy sentinel path does: its
// UPDATE matches zero rows and that is fine.
func TestCommitStock_ContinuePolicyWithNoStorageSucceedsOnAllocationPath(t *testing.T) {
	db := testdb.NewDB(t, "order_allocations", "stock_holds", "order_items", "orders",
		"variant_stock", "product_variants", "products", "stores")
	storeID, variantID := seedAvailStore(t, db)
	whA := seedWarehouseRow(t, db, storeID, "A")
	require.NoError(t, db.Exec(
		`UPDATE product_variants SET inventory_policy = 'continue' WHERE id = ?`, variantID).Error)
	// Deliberately no variant_stock row for variantID anywhere.

	lines := []stockLine{{VariantID: variantID, Quantity: 3}}
	orderID, itemIDs := seedOrderWithItems(t, db, storeID, lines)

	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		return commitStock(context.Background(), tx, stockhold.NewRepository(), uuid.NewString(),
			orderID, storeID, lines)
	}))

	require.Equal(t, -1, stockUnitsAt(t, db, variantID, whA),
		"no variant_stock row existed and none should have been created")

	var got struct {
		WarehouseID string
		Quantity    int
	}
	require.NoError(t, db.Raw(
		`SELECT warehouse_id, quantity FROM order_allocations WHERE order_item_id = ?`, itemIDs[0]).
		Row().Scan(&got.WarehouseID, &got.Quantity))
	require.Equal(t, whA, got.WarehouseID)
	require.Equal(t, 3, got.Quantity)
}

// A continue-policy variant whose warehouse's units span TWO storage
// locations (the sentinel row plus a real one). Both must be decremented,
// not just the first one the breakdown happens to list.
func TestCommitStock_ContinuePolicyDecrementsAcrossMultipleStorageLocations(t *testing.T) {
	db := testdb.NewDB(t, "order_allocations", "stock_holds", "order_items", "orders",
		"variant_stock", "product_variants", "products", "stores")
	storeID, variantID := seedAvailStore(t, db)
	whA := seedWarehouseRow(t, db, storeID, "A")
	require.NoError(t, db.Exec(
		`UPDATE product_variants SET inventory_policy = 'continue' WHERE id = ?`, variantID).Error)
	require.NoError(t, db.Exec(
		`INSERT INTO variant_stock (variant_id, location_id, quantity, updated_at)
		 VALUES (?, ?, 1, now()), (?, ?, 1, now())`,
		variantID, product.DefaultLocationID, variantID, whA).Error)

	// 2 units requested against two locations holding 1 each: whichever
	// sorts first can only cover 1, so the second MUST be touched too, or
	// this fails regardless of storage order.
	lines := []stockLine{{VariantID: variantID, Quantity: 2}}
	orderID, _ := seedOrderWithItems(t, db, storeID, lines)

	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		return commitStock(context.Background(), tx, stockhold.NewRepository(), uuid.NewString(),
			orderID, storeID, lines)
	}))

	require.Equal(t, 0, stockUnitsAt(t, db, variantID, product.DefaultLocationID),
		"the sentinel location must be decremented, not left untouched")
	require.Equal(t, 0, stockUnitsAt(t, db, variantID, whA),
		"the real warehouse location must be decremented too")
}

// FIX 1 (final review): the storage draw must track units already consumed
// by EARLIER assignments in this call, not restart from the full breakdown
// snapshot for every assignment. Two lines of the same variant, both filled
// at one warehouse whose units span the sentinel row and a real row —
// breakdown [sentinel:3, A:4], lines of 2 and 3 — must total 3 at the
// sentinel and 2 at A, not 5 at a location that only holds 3.
func TestCommitStock_TwoLinesFilledAtSameWarehouseDoNotDoubleCountTheBreakdown(t *testing.T) {
	db := testdb.NewDB(t, "order_allocations", "stock_holds", "order_items", "orders",
		"variant_stock", "product_variants", "products", "stores")
	storeID, variantID := seedAvailStore(t, db)
	whA := seedWarehouseRow(t, db, storeID, "A")
	require.NoError(t, db.Exec(
		`INSERT INTO variant_stock (variant_id, location_id, quantity, updated_at)
		 VALUES (?, ?, 3, now()), (?, ?, 4, now())`,
		variantID, product.DefaultLocationID, variantID, whA).Error)

	lines := []stockLine{{VariantID: variantID, Quantity: 2}, {VariantID: variantID, Quantity: 3}}
	orderID, itemIDs := seedOrderWithItems(t, db, storeID, lines)

	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		return commitStock(context.Background(), tx, stockhold.NewRepository(), uuid.NewString(),
			orderID, storeID, lines)
	}), "a cart the store can fully fill must not see out_of_stock")

	require.Equal(t, 0, stockUnitsAt(t, db, variantID, product.DefaultLocationID),
		"3 units at the sentinel, fully drawn across both lines")
	require.Equal(t, 2, stockUnitsAt(t, db, variantID, whA),
		"4 units at A, only the 2 units the sentinel could not cover")

	var total int64
	require.NoError(t, db.Raw(
		`SELECT COALESCE(SUM(quantity), 0) FROM order_allocations WHERE order_id = ?`, orderID).
		Scan(&total).Error)
	require.Equal(t, int64(5), total, "5 units requested, 5 units allocated")
	_ = itemIDs
}

// FIX 2 (final review): a cart-time hold that no longer matches the final
// placement plan must be released before Commit runs, or Commit decrements
// it a second time alongside the plan's own hold — or, when the stale
// location no longer has the units, the variant_stock non-negative CHECK
// fires and the checkout 500s instead of succeeding.
//
// The stale hold is manufactured the way it happens in production: a
// cart-add hold succeeds against the sentinel while it still has stock, and
// by checkout time an admin has corrected that count to zero — the hold
// still exists, but the sentinel no longer has anything to give, so the
// warehouse allocation plan must fill entirely from the real row instead.
func TestCommitStock_StaleCartHoldAtAnUnusedLocationIsReleasedNotDoubleCommitted(t *testing.T) {
	db := testdb.NewDB(t, "order_allocations", "stock_holds", "order_items", "orders",
		"variant_stock", "product_variants", "products", "stores")
	storeID, variantID := seedAvailStore(t, db)
	whA := seedWarehouseRow(t, db, storeID, "A")
	require.NoError(t, db.Exec(
		`INSERT INTO variant_stock (variant_id, location_id, quantity, updated_at)
		 VALUES (?, ?, 2, now()), (?, ?, 5, now())`,
		variantID, product.DefaultLocationID, variantID, whA).Error)

	cart := uuid.NewString()
	holds := stockhold.NewRepository()

	// The cart-add hold, placed while the sentinel still had stock.
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		return holds.Hold(context.Background(), tx, cart, variantID, product.DefaultLocationID, 2, time.Hour)
	}))

	// An admin corrects the count between cart-add and checkout. The hold
	// row is untouched — it still says 2 units held at the sentinel.
	require.NoError(t, db.Exec(
		`UPDATE variant_stock SET quantity = 0 WHERE variant_id = ? AND location_id = ?`,
		variantID, product.DefaultLocationID).Error)

	lines := []stockLine{{VariantID: variantID, Quantity: 2}}
	orderID, itemIDs := seedOrderWithItems(t, db, storeID, lines)

	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		return commitStock(context.Background(), tx, holds, cart, orderID, storeID, lines)
	}), "the stale sentinel hold must be released, not decremented a second time")

	require.Equal(t, 0, stockUnitsAt(t, db, variantID, product.DefaultLocationID),
		"the sentinel had nothing left and must stay at zero, not go negative")
	require.Equal(t, 3, stockUnitsAt(t, db, variantID, whA),
		"the whole order must be filled from A, the only location with real stock")

	var state string
	require.NoError(t, db.Raw(
		`SELECT state FROM stock_holds WHERE cart_token = ? AND location_id = ?`,
		cart, product.DefaultLocationID).Row().Scan(&state))
	require.Equal(t, "released", state, "the stale hold must be released, not left held or committed")

	var allocWarehouse string
	require.NoError(t, db.Raw(
		`SELECT warehouse_id FROM order_allocations WHERE order_item_id = ?`, itemIDs[0]).
		Row().Scan(&allocWarehouse))
	require.Equal(t, whA, allocWarehouse)
}

// FIX 3 (final review): a checkout item with a NULL variant_id (a custom or
// unstocked line) must not make recordAllocations 500 the whole checkout.
// Only order_items that correspond to a stock line are considered; the rest
// are ignored, matching the legacy path's silent skip.
func TestCommitStock_ItemWithNilVariantIDIsIgnoredNotAFailure(t *testing.T) {
	db := testdb.NewDB(t, "order_allocations", "stock_holds", "order_items", "orders",
		"variant_stock", "product_variants", "products", "stores")
	storeID, variantID := seedAvailStore(t, db)
	whA := seedWarehouseRow(t, db, storeID, "A")
	require.NoError(t, db.Exec(
		`INSERT INTO variant_stock (variant_id, location_id, quantity, updated_at)
		 VALUES (?, ?, 5, now())`, variantID, whA).Error)

	var tenantID string
	require.NoError(t, db.Raw(`SELECT tenant_id FROM stores WHERE id = ?`, storeID).Row().Scan(&tenantID))
	orderID := uuid.NewString()
	require.NoError(t, db.Exec(
		`INSERT INTO orders (id, tenant_id, store_id, order_number, idempotency_key,
		                     customer_email, currency_code, subtotal, grand_total)
		 VALUES (?, ?, ?, ?, ?, 'buyer@example.com', 'INR', 10.00, 10.00)`,
		orderID, tenantID, storeID, "AL-"+uuid.NewString()[:8], uuid.NewString()).Error)

	stockedItemID := uuid.NewString()
	unstockedItemID := uuid.NewString()
	require.NoError(t, db.Exec(
		`INSERT INTO order_items (id, order_id, variant_id, title_snapshot, sku_snapshot,
		                          unit_price, quantity, line_total, currency_code)
		 VALUES (?, ?, ?, 'Stocked', 'SKU-A', 10.00, 2, 20.00, 'INR'),
		        (?, ?, NULL, 'Custom', 'SKU-B', 5.00, 1, 5.00, 'INR')`,
		stockedItemID, orderID, variantID, unstockedItemID, orderID).Error)

	// Mirrors stockLinesFromItems: the nil-variant item produced no line.
	lines := []stockLine{{VariantID: variantID, Quantity: 2}}

	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		return commitStock(context.Background(), tx, stockhold.NewRepository(), uuid.NewString(),
			orderID, storeID, lines)
	}), "a variantless item must not turn a paid checkout into a 500")

	require.Equal(t, 3, stockUnitsAt(t, db, variantID, whA))

	var allocCount int64
	require.NoError(t, db.Raw(
		`SELECT count(*) FROM order_allocations WHERE order_id = ?`, orderID).Scan(&allocCount).Error)
	require.Equal(t, int64(1), allocCount,
		"exactly one allocation row for the stocked line — the variantless item is ignored, not attributed")

	var allocItemID string
	require.NoError(t, db.Raw(
		`SELECT order_item_id FROM order_allocations WHERE order_id = ?`, orderID).Row().Scan(&allocItemID))
	require.Equal(t, stockedItemID, allocItemID)
}

// FIX 5 (final review): the legacy path passed the client's variant_id string
// straight into SQL, where Postgres casts to uuid case-insensitively. The
// allocation path does Go map lookups against ids read back from the
// database in canonical lowercase, so an uppercase client-supplied id must
// be normalised at the boundary or it reads as zero availability.
func TestCommitStock_UppercaseVariantIDStillMatchesAvailability(t *testing.T) {
	db := testdb.NewDB(t, "order_allocations", "stock_holds", "order_items", "orders",
		"variant_stock", "product_variants", "products", "stores")
	storeID, variantID := seedAvailStore(t, db)
	whA := seedWarehouseRow(t, db, storeID, "A")
	require.NoError(t, db.Exec(
		`INSERT INTO variant_stock (variant_id, location_id, quantity, updated_at)
		 VALUES (?, ?, 5, now())`, variantID, whA).Error)

	upper := strings.ToUpper(variantID)
	qty := 2
	lines := stockLinesFromItems([]CheckoutItemRequest{{VariantID: &upper, Quantity: qty}})
	require.Len(t, lines, 1)
	require.Equal(t, strings.ToLower(upper), lines[0].VariantID,
		"lines must be normalised to lowercase so map lookups against DB-canonical ids succeed")

	orderID, _ := seedOrderWithItems(t, db, storeID, lines)

	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		return commitStock(context.Background(), tx, stockhold.NewRepository(), uuid.NewString(),
			orderID, storeID, lines)
	}), "an uppercase client-supplied variant id must not produce a false out_of_stock")

	require.Equal(t, 3, stockUnitsAt(t, db, variantID, whA))
}

// #177 PR 5a added archiving; the allocator's candidate query did not learn
// about it. An archived warehouse still holding stock would keep receiving
// allocations — the merchant removes a warehouse from the settings page and
// orders keep being routed to it, with nobody there to pick them.
//
// The order below needs 5 units. Only the LIVE warehouse's 3 count, so it
// must fail rather than quietly draw the other 2 from an archived one.
func TestCommitStock_ArchivedWarehouseIsNotAllocatedTo(t *testing.T) {
	db := testdb.NewDB(t, "order_allocations", "stock_holds", "order_items", "orders",
		"variant_stock", "product_variants", "products", "stores")
	storeID, variantID := seedAvailStore(t, db)
	live := seedWarehouseRow(t, db, storeID, "Live")
	archived := seedWarehouseRow(t, db, storeID, "Archived")
	require.NoError(t, db.Exec(`UPDATE warehouses SET priority = 0 WHERE id = ?`, live).Error)
	require.NoError(t, db.Exec(`UPDATE warehouses SET priority = 1 WHERE id = ?`, archived).Error)
	require.NoError(t, db.Exec(`UPDATE warehouses SET archived_at = now() WHERE id = ?`, archived).Error)
	require.NoError(t, db.Exec(
		`INSERT INTO variant_stock (variant_id, location_id, quantity, updated_at)
		 VALUES (?, ?, 3, now()), (?, ?, 4, now())`,
		variantID, live, variantID, archived).Error)

	lines := []stockLine{{VariantID: variantID, Quantity: 5}}
	orderID, _ := seedOrderWithItems(t, db, storeID, lines)

	err := db.Transaction(func(tx *gorm.DB) error {
		return commitStock(context.Background(), tx, stockhold.NewRepository(), uuid.NewString(),
			orderID, storeID, lines)
	})
	require.Error(t, err,
		"5 units against 3 live + 4 archived must not fill — archived stock is not sellable")

	require.Equal(t, 3, stockUnitsAt(t, db, variantID, live), "a failed checkout must move no stock")
	require.Equal(t, 4, stockUnitsAt(t, db, variantID, archived))
}
