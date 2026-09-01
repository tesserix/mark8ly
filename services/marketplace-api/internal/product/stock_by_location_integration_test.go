//go:build integration

// Package product_test — #177 PR 5e: per-warehouse stock.
//
// The trap this file exists for: until PR 6's backfill runs, a variant's
// units live on the SENTINEL location, and checkout_availability.go
// attributes sentinel rows to the store's FIRST warehouse while SUMMING
// them with any real row for that same warehouse. So writing a real row
// without clearing the sentinel silently doubles the merchant's stock —
// they type the number already on screen and end up with twice it.
//
// SetVariantStockByLocation therefore CONSERVES: it writes the real rows
// and drops the variant's sentinel row in the same transaction.
package product_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/product"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

// stockAtLocation reads one variant_stock quantity, or -1 when no row.
func stockAtLocation(t *testing.T, tx *gorm.DB, variantID, locationID string) int {
	t.Helper()
	var q int
	require.NoError(t, tx.Raw(
		`SELECT COALESCE((SELECT quantity FROM variant_stock
		                   WHERE variant_id = ? AND location_id = ?), -1)`,
		variantID, locationID).Scan(&q).Error)
	return q
}

// seedWarehouseRowForStock inserts a live warehouse for the store.
func seedWarehouseRowForStock(t *testing.T, tx *gorm.DB, tenantID, storeID, name string) string {
	t.Helper()
	id := uuid.NewString()
	require.NoError(t, tx.Exec(
		`INSERT INTO warehouses (id, tenant_id, store_id, name, line1, city, region,
		                         postal_code, country_code, phone)
		 VALUES (?, ?, ?, ?, '1 Dock Rd', 'Mumbai', 'MH', '400001', 'IN', '+912200000000')`,
		id, tenantID, storeID, name).Error)
	return id
}

func TestSetVariantStockByLocation_WritesRealRowsAndClearsTheSentinel(t *testing.T) {
	tx := testdb.NewTx(t)
	repo := product.NewRepository(tx)
	ctx := context.Background()

	storeID, tenantID, vendorID := seedStore(t, tx)
	agg := minimalAggregate(storeID, tenantID, vendorID, "linen-shirt", "LINEN-1")
	require.NoError(t, repo.CreateAggregateInTx(ctx, tx, agg))
	variantID := agg.Variants[0].ID

	// The pre-5e world: everything on the sentinel.
	require.NoError(t, repo.UpdateVariantStockInTx(ctx, tx, variantID, product.DefaultLocationID, 10))

	whA := seedWarehouseRowForStock(t, tx, tenantID, storeID, "Alpha")
	whB := seedWarehouseRowForStock(t, tx, tenantID, storeID, "Bravo")

	require.NoError(t, repo.SetVariantStockByLocationInTx(ctx, tx, variantID,
		map[string]int{whA: 10, whB: 5}))

	var sentinel int64
	require.NoError(t, tx.Raw(
		`SELECT count(*) FROM variant_stock WHERE variant_id = ? AND location_id = ?`,
		variantID, product.DefaultLocationID).Scan(&sentinel).Error)
	require.Zero(t, sentinel,
		"the sentinel row must be gone — left behind it is summed with the real row for the same warehouse and the merchant's stock doubles")

	require.Equal(t, 10, stockAtLocation(t, tx, variantID, whA))
	require.Equal(t, 5, stockAtLocation(t, tx, variantID, whB))
}

// The denormalised column is trigger-maintained and must end up as the
// TOTAL across locations, not the last one written.
func TestSetVariantStockByLocation_InventoryQuantityIsTheTotal(t *testing.T) {
	tx := testdb.NewTx(t)
	repo := product.NewRepository(tx)
	ctx := context.Background()

	storeID, tenantID, vendorID := seedStore(t, tx)
	agg := minimalAggregate(storeID, tenantID, vendorID, "linen-shirt", "LINEN-1")
	require.NoError(t, repo.CreateAggregateInTx(ctx, tx, agg))
	variantID := agg.Variants[0].ID
	require.NoError(t, repo.UpdateVariantStockInTx(ctx, tx, variantID, product.DefaultLocationID, 10))

	whA := seedWarehouseRowForStock(t, tx, tenantID, storeID, "Alpha")
	whB := seedWarehouseRowForStock(t, tx, tenantID, storeID, "Bravo")
	require.NoError(t, repo.SetVariantStockByLocationInTx(ctx, tx, variantID,
		map[string]int{whA: 10, whB: 5}))

	got, err := repo.GetByIDForStore(ctx, agg.Product.ID, storeID, tenantID)
	require.NoError(t, err)
	require.Equal(t, 15, got.Variants[0].InventoryQuantity,
		"inventory_quantity must be the sum across locations")
}

// Setting a location to zero must leave a zero row rather than deleting
// it: a missing row and a zero row read the same to availability, but the
// merchant explicitly said "none here", and a later edit needs the row to
// exist to show the location at all.
func TestSetVariantStockByLocation_ZeroIsRecordedNotDropped(t *testing.T) {
	tx := testdb.NewTx(t)
	repo := product.NewRepository(tx)
	ctx := context.Background()

	storeID, tenantID, vendorID := seedStore(t, tx)
	agg := minimalAggregate(storeID, tenantID, vendorID, "linen-shirt", "LINEN-1")
	require.NoError(t, repo.CreateAggregateInTx(ctx, tx, agg))
	variantID := agg.Variants[0].ID

	whA := seedWarehouseRowForStock(t, tx, tenantID, storeID, "Alpha")
	whB := seedWarehouseRowForStock(t, tx, tenantID, storeID, "Bravo")
	require.NoError(t, repo.SetVariantStockByLocationInTx(ctx, tx, variantID,
		map[string]int{whA: 7, whB: 0}))

	require.Equal(t, 7, stockAtLocation(t, tx, variantID, whA))
	require.Equal(t, 0, stockAtLocation(t, tx, variantID, whB))

	var rows int64
	require.NoError(t, tx.Raw(
		`SELECT count(*) FROM variant_stock WHERE variant_id = ? AND location_id = ?`,
		variantID, whB).Scan(&rows).Error)
	require.EqualValues(t, 1, rows, "an explicit zero must still have a row")
}

// Calling it twice with the same numbers must not change the totals — the
// merchant pressing Save twice is not a stock movement.
func TestSetVariantStockByLocation_IsIdempotent(t *testing.T) {
	tx := testdb.NewTx(t)
	repo := product.NewRepository(tx)
	ctx := context.Background()

	storeID, tenantID, vendorID := seedStore(t, tx)
	agg := minimalAggregate(storeID, tenantID, vendorID, "linen-shirt", "LINEN-1")
	require.NoError(t, repo.CreateAggregateInTx(ctx, tx, agg))
	variantID := agg.Variants[0].ID
	require.NoError(t, repo.UpdateVariantStockInTx(ctx, tx, variantID, product.DefaultLocationID, 10))

	whA := seedWarehouseRowForStock(t, tx, tenantID, storeID, "Alpha")
	for range 2 {
		require.NoError(t, repo.SetVariantStockByLocationInTx(ctx, tx, variantID,
			map[string]int{whA: 10}))
	}

	require.Equal(t, 10, stockAtLocation(t, tx, variantID, whA))
	got, err := repo.GetByIDForStore(ctx, agg.Product.ID, storeID, tenantID)
	require.NoError(t, err)
	require.Equal(t, 10, got.Variants[0].InventoryQuantity)
}

// The sentinel is not a place a merchant can choose, and accepting it
// would write back the very row this method exists to delete — the write
// and the delete would race within one call and the outcome would depend
// on statement order. Refuse it outright.
//
// Found by mutation: removing the guard broke no test until this one.
func TestSetVariantStockByLocation_RefusesTheSentinelAsALocation(t *testing.T) {
	tx := testdb.NewTx(t)
	repo := product.NewRepository(tx)
	ctx := context.Background()

	storeID, tenantID, vendorID := seedStore(t, tx)
	agg := minimalAggregate(storeID, tenantID, vendorID, "linen-shirt", "LINEN-1")
	require.NoError(t, repo.CreateAggregateInTx(ctx, tx, agg))
	variantID := agg.Variants[0].ID
	require.NoError(t, repo.UpdateVariantStockInTx(ctx, tx, variantID, product.DefaultLocationID, 10))

	err := repo.SetVariantStockByLocationInTx(ctx, tx, variantID,
		map[string]int{product.DefaultLocationID: 99})
	require.Error(t, err)

	require.Equal(t, 10, stockAtLocation(t, tx, variantID, product.DefaultLocationID),
		"a refused call must not have touched the stock")
}

// An empty map is "nothing asked for", not "clear this variant". Treating
// it as the latter would destroy stock on a no-op save.
func TestSetVariantStockByLocation_EmptyMapChangesNothing(t *testing.T) {
	tx := testdb.NewTx(t)
	repo := product.NewRepository(tx)
	ctx := context.Background()

	storeID, tenantID, vendorID := seedStore(t, tx)
	agg := minimalAggregate(storeID, tenantID, vendorID, "linen-shirt", "LINEN-1")
	require.NoError(t, repo.CreateAggregateInTx(ctx, tx, agg))
	variantID := agg.Variants[0].ID
	require.NoError(t, repo.UpdateVariantStockInTx(ctx, tx, variantID, product.DefaultLocationID, 10))

	require.NoError(t, repo.SetVariantStockByLocationInTx(ctx, tx, variantID, map[string]int{}))

	require.Equal(t, 10, stockAtLocation(t, tx, variantID, product.DefaultLocationID),
		"an empty request must not have cleared the sentinel")
}
