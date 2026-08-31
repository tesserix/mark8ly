//go:build integration

// Package product_test — coverage for migration 000119.
//
// product_variants.inventory_quantity is what browse, PDP and cart read.
// Until 000119 the sync trigger assigned the LAST WRITTEN location's
// quantity, which is the total only while a variant has one stock row. The
// second warehouse would have made the storefront's stock number mean
// "whichever warehouse was touched most recently" — wrong, and silently so.
package product_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

// seedVariantForSum creates the store/product/variant a stock row needs and
// returns the variant id. No variant_stock row is created: each test decides
// how many locations it wants.
func seedVariantForSum(t *testing.T, db *gorm.DB) string {
	t.Helper()
	tenantID, storeID := uuid.NewString(), uuid.NewString()
	require.NoError(t, db.Exec(
		`INSERT INTO stores (id, tenant_id, name, slug, status, country_code, currency_code, timezone,
		                     storefront_customer_portal_secret)
		 VALUES (?, ?, 'Sum Test', ?, 'active', 'IN', 'INR', 'Asia/Kolkata', ?)`,
		storeID, tenantID, "sum-"+uuid.NewString()[:8], uuid.NewString()).Error)

	productID := uuid.NewString()
	require.NoError(t, db.Exec(
		// status='active' requires published_at (products_published_requires_active).
		`INSERT INTO products (id, tenant_id, store_id, title, handle, status, vendor_id, published_at)
		 VALUES (?, ?, ?, 'Sum Test Product', ?, 'active', ?, now())`,
		productID, tenantID, storeID, "sum-"+uuid.NewString()[:8], uuid.NewString()).Error)

	variantID := uuid.NewString()
	require.NoError(t, db.Exec(
		`INSERT INTO product_variants (id, product_id, store_id, sku, price, currency_code)
		 VALUES (?, ?, ?, ?, 10.00, 'INR')`,
		variantID, productID, storeID, "SKU-"+uuid.NewString()[:8]).Error)
	return variantID
}

func addStock(t *testing.T, db *gorm.DB, variantID, locationID string, qty int) {
	t.Helper()
	require.NoError(t, db.Exec(
		`INSERT INTO variant_stock (variant_id, location_id, quantity, updated_at)
		 VALUES (?, ?, ?, now())`, variantID, locationID, qty).Error)
}

func inventoryQuantity(t *testing.T, db *gorm.DB, variantID string) int {
	t.Helper()
	var qty int
	require.NoError(t, db.Raw(
		`SELECT inventory_quantity FROM product_variants WHERE id = ?`, variantID,
	).Row().Scan(&qty))
	return qty
}

func TestInventorySync_SumsAcrossLocations(t *testing.T) {
	db := testdb.NewTx(t)
	variantID := seedVariantForSum(t, db)

	addStock(t, db, variantID, uuid.NewString(), 3)
	addStock(t, db, variantID, uuid.NewString(), 2)

	require.Equal(t, 5, inventoryQuantity(t, db, variantID),
		"the storefront's stock number must be the total across warehouses")
}

// The pre-000119 trigger assigned NEW.quantity, so writing the SMALLER
// location second would have reported 2 for a variant holding 5. This is the
// case that pins the fix rather than merely exercising it.
func TestInventorySync_UpdatingOneLocationKeepsTheTotal(t *testing.T) {
	db := testdb.NewTx(t)
	variantID := seedVariantForSum(t, db)
	locA, locB := uuid.NewString(), uuid.NewString()

	addStock(t, db, variantID, locA, 4)
	addStock(t, db, variantID, locB, 1)
	require.Equal(t, 5, inventoryQuantity(t, db, variantID))

	require.NoError(t, db.Exec(
		`UPDATE variant_stock SET quantity = 2 WHERE variant_id = ? AND location_id = ?`,
		variantID, locB).Error)

	require.Equal(t, 6, inventoryQuantity(t, db, variantID),
		"updating one location must re-sum, not overwrite the total with that location")
}

// Without an AFTER DELETE arm the trigger never fires on removal and
// inventory_quantity keeps counting stock in a warehouse that no longer has
// a row — an oversell that no test would otherwise catch.
func TestInventorySync_DeletingALocationLowersTheTotal(t *testing.T) {
	db := testdb.NewTx(t)
	variantID := seedVariantForSum(t, db)
	locA, locB := uuid.NewString(), uuid.NewString()

	addStock(t, db, variantID, locA, 4)
	addStock(t, db, variantID, locB, 3)
	require.Equal(t, 7, inventoryQuantity(t, db, variantID))

	require.NoError(t, db.Exec(
		`DELETE FROM variant_stock WHERE variant_id = ? AND location_id = ?`,
		variantID, locB).Error)

	require.Equal(t, 4, inventoryQuantity(t, db, variantID),
		"removing a location's stock must lower the total")
}

func TestInventorySync_LastLocationRemovedLeavesZeroNotNull(t *testing.T) {
	db := testdb.NewTx(t)
	variantID := seedVariantForSum(t, db)
	loc := uuid.NewString()

	addStock(t, db, variantID, loc, 6)
	require.NoError(t, db.Exec(
		`DELETE FROM variant_stock WHERE variant_id = ?`, variantID).Error)

	require.Equal(t, 0, inventoryQuantity(t, db, variantID),
		"SUM over no rows is NULL; the trigger must coalesce it to zero")
}

// waitForBlockedLock polls pg_locks until it sees ANY ungranted lock request,
// or the timeout elapses. Used instead of a bare sleep so the interleaving
// below is deterministic rather than hoping a fixed delay was long enough.
//
// Deliberately not scoped to a specific relation: a second transaction
// waiting on a first transaction's uncommitted UPDATE of a conflicting row
// shows up as a `transactionid` wait (PostgreSQL's XactLockTableWait), not a
// `relation`-typed tuple lock joinable to pg_class — so filtering by relname
// would miss exactly the wait this test relies on.
func waitForBlockedLock(t *testing.T, db *gorm.DB, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		var n int64
		require.NoError(t, db.Raw(
			`SELECT count(*) FROM pg_locks WHERE NOT granted`,
		).Row().Scan(&n))
		if n > 0 {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("timed out waiting for a blocked lock")
}

// TestInventorySync_ConcurrentWritesToDifferentLocationsDoNotLoseUpdates
// reproduces the race the reviewer found on Postgres 15: a variant with
// locations A=5, B=4 (inventory_quantity=9). Tx1 sets A=3 and sleeps before
// commit; Tx2 sets B=2 and commits.
//
// Without the row lock this test would still (correctly) see Tx2 block on
// Tx1's held lock on the product_variants row — but the trigger's `SELECT
// SUM(...)` sub-select is evaluated against Tx2's PRE-BLOCK snapshot (where A
// is still 5), and PostgreSQL's EvalPlanQual re-checks only the UPDATE's
// qual on unblocking, not that sub-select. The result is inventory_quantity
// = 7 (5 + 2) even though the true total, once both commit, is 5 (3 + 2).
//
// This needs two real connections, not testdb.NewTx (one transaction can't
// interleave with itself). testdb.NewDB's pool has room for both: the setup
// connection plus one each for tx1 and tx2.
func TestInventorySync_ConcurrentWritesToDifferentLocationsDoNotLoseUpdates(t *testing.T) {
	db := testdb.NewDB(t, "variant_stock", "product_variants", "products", "stores")
	variantID := seedVariantForSum(t, db)
	locA, locB := uuid.NewString(), uuid.NewString()

	addStock(t, db, variantID, locA, 5)
	addStock(t, db, variantID, locB, 4)
	require.Equal(t, 9, inventoryQuantity(t, db, variantID))

	tx1 := db.Begin()
	require.NoError(t, tx1.Error)
	require.NoError(t, tx1.Exec(
		`UPDATE variant_stock SET quantity = 3 WHERE variant_id = ? AND location_id = ?`,
		variantID, locA).Error)

	tx2Done := make(chan error, 1)
	go func() {
		tx2 := db.Begin()
		if tx2.Error != nil {
			tx2Done <- tx2.Error
			return
		}
		if err := tx2.Exec(
			`UPDATE variant_stock SET quantity = 2 WHERE variant_id = ? AND location_id = ?`,
			variantID, locB).Error; err != nil {
			tx2.Rollback()
			tx2Done <- err
			return
		}
		tx2Done <- tx2.Commit().Error
	}()

	// Confirm tx2 is genuinely blocked on tx1's lock before committing tx1 —
	// otherwise the two writes might not interleave at all and the test
	// would pass without exercising the race.
	waitForBlockedLock(t, db, 3*time.Second)

	require.NoError(t, tx1.Commit().Error)
	require.NoError(t, <-tx2Done, "tx2 must succeed once tx1's lock is released")

	require.Equal(t, 5, inventoryQuantity(t, db, variantID),
		"concurrent writes to different locations of the same variant must not lose an update: "+
			"true total after both commits is 5 (3+2), not 7 (the pre-block snapshot of A plus B)")
}
