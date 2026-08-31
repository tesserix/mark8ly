//go:build integration

// Package storefront — coverage for the multi-warehouse availability
// snapshot (#177 PR 3).
//
// The snapshot is what allocation reasons over. Two things about it are
// easy to get wrong and expensive to get wrong:
//
//   - It must EXCLUDE the calling cart's own live holds, matching
//     stockhold.Hold and stockhold.Available. A cart asking what it can have
//     must not be told its own reservation is competition.
//   - Until PR 6's backfill, units are stored at the sentinel location, not
//     at any warehouse id. The snapshot therefore reports availability
//     against a WAREHOUSE while remembering the STORAGE location the units
//     actually sit at, because that is where a hold has to be placed.
package storefront

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/allocation"
	"github.com/mark8ly/marketplace-api/internal/product"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

// seedAvailStore creates a store, a product and one variant, and returns
// (storeID, variantID).
func seedAvailStore(t *testing.T, db *gorm.DB) (string, string) {
	t.Helper()
	tenantID, storeID := uuid.NewString(), uuid.NewString()
	require.NoError(t, db.Exec(
		`INSERT INTO stores (id, tenant_id, name, slug, status, country_code, currency_code, timezone,
		                     storefront_customer_portal_secret)
		 VALUES (?, ?, 'Avail Test', ?, 'active', 'IN', 'INR', 'Asia/Kolkata', ?)`,
		storeID, tenantID, "avail-"+uuid.NewString()[:8], uuid.NewString()).Error)

	productID := uuid.NewString()
	require.NoError(t, db.Exec(
		`INSERT INTO products (id, tenant_id, store_id, title, handle, status, vendor_id, published_at)
		 VALUES (?, ?, ?, 'Avail Product', ?, 'active', ?, now())`,
		productID, tenantID, storeID, "avail-"+uuid.NewString()[:8], uuid.NewString()).Error)

	variantID := uuid.NewString()
	require.NoError(t, db.Exec(
		`INSERT INTO product_variants (id, product_id, store_id, sku, price, currency_code)
		 VALUES (?, ?, ?, ?, 10.00, 'INR')`,
		variantID, productID, storeID, "SKU-"+uuid.NewString()[:8]).Error)
	return storeID, variantID
}

func seedWarehouseRow(t *testing.T, db *gorm.DB, storeID, name string) string {
	t.Helper()
	var tenantID string
	require.NoError(t, db.Raw(`SELECT tenant_id FROM stores WHERE id = ?`, storeID).Row().Scan(&tenantID))
	id := uuid.NewString()
	require.NoError(t, db.Exec(
		`INSERT INTO warehouses (id, tenant_id, store_id, name, line1, city, region, postal_code, country_code, phone)
		 VALUES (?, ?, ?, ?, '1 Dock Rd', 'Mumbai', 'MH', '400001', 'IN', '+912200000000')`,
		id, tenantID, storeID, name).Error)
	return id
}

func TestLoadAvailability_SentinelStockIsReportedAgainstTheFirstWarehouse(t *testing.T) {
	db := testdb.NewTx(t)
	storeID, variantID := seedAvailStore(t, db)
	whID := seedWarehouseRow(t, db, storeID, "Main")

	// Stock still lives at the sentinel — PR 6 has not backfilled yet.
	require.NoError(t, db.Exec(
		`INSERT INTO variant_stock (variant_id, location_id, quantity, updated_at)
		 VALUES (?, ?, 7, now())`, variantID, product.DefaultLocationID).Error)

	warehouses := []allocation.Warehouse{{ID: whID}}
	avail, storage, err := loadAvailability(context.Background(), db, uuid.NewString(), warehouses, []string{variantID})
	require.NoError(t, err)

	require.Equal(t, 7, avail.At(variantID, whID),
		"sentinel-stored units must be visible against the store's warehouse before the backfill")
	require.Equal(t, product.DefaultLocationID, storage[variantID][whID],
		"a hold must target the location the units are actually stored at")
}

func TestLoadAvailability_RealWarehouseStockIsReportedAgainstItself(t *testing.T) {
	db := testdb.NewTx(t)
	storeID, variantID := seedAvailStore(t, db)
	whA := seedWarehouseRow(t, db, storeID, "A")
	whB := seedWarehouseRow(t, db, storeID, "B")

	require.NoError(t, db.Exec(
		`INSERT INTO variant_stock (variant_id, location_id, quantity, updated_at)
		 VALUES (?, ?, 3, now()), (?, ?, 4, now())`,
		variantID, whA, variantID, whB).Error)

	warehouses := []allocation.Warehouse{{ID: whA}, {ID: whB}}
	avail, storage, err := loadAvailability(context.Background(), db, uuid.NewString(), warehouses, []string{variantID})
	require.NoError(t, err)

	require.Equal(t, 3, avail.At(variantID, whA))
	require.Equal(t, 4, avail.At(variantID, whB))
	require.Equal(t, whA, storage[variantID][whA], "post-backfill the storage location IS the warehouse")
	require.Equal(t, whB, storage[variantID][whB])
}

// Matching stockhold.Hold and stockhold.Available: a cart must not see its
// own reservation as competition, or it could never re-hold what it already
// has and checkout would refuse a cart it had itself reserved.
func TestLoadAvailability_ExcludesTheCallingCartsOwnHolds(t *testing.T) {
	db := testdb.NewTx(t)
	storeID, variantID := seedAvailStore(t, db)
	whID := seedWarehouseRow(t, db, storeID, "Main")
	require.NoError(t, db.Exec(
		`INSERT INTO variant_stock (variant_id, location_id, quantity, updated_at)
		 VALUES (?, ?, 10, now())`, variantID, product.DefaultLocationID).Error)

	mine, theirs := uuid.NewString(), uuid.NewString()
	require.NoError(t, db.Exec(
		`INSERT INTO stock_holds (variant_id, location_id, cart_token, qty, expires_at, state)
		 VALUES (?, ?, ?, 4, ?, 'held'), (?, ?, ?, 3, ?, 'held')`,
		variantID, product.DefaultLocationID, mine, time.Now().Add(time.Hour),
		variantID, product.DefaultLocationID, theirs, time.Now().Add(time.Hour)).Error)

	warehouses := []allocation.Warehouse{{ID: whID}}
	avail, _, err := loadAvailability(context.Background(), db, mine, warehouses, []string{variantID})
	require.NoError(t, err)

	require.Equal(t, 7, avail.At(variantID, whID),
		"10 units less the OTHER cart's 3; this cart's own 4 must not count against it")
}

func TestLoadAvailability_ExpiredHoldsDoNotReduceAvailability(t *testing.T) {
	db := testdb.NewTx(t)
	storeID, variantID := seedAvailStore(t, db)
	whID := seedWarehouseRow(t, db, storeID, "Main")
	require.NoError(t, db.Exec(
		`INSERT INTO variant_stock (variant_id, location_id, quantity, updated_at)
		 VALUES (?, ?, 5, now())`, variantID, product.DefaultLocationID).Error)
	require.NoError(t, db.Exec(
		`INSERT INTO stock_holds (variant_id, location_id, cart_token, qty, expires_at, state)
		 VALUES (?, ?, ?, 5, ?, 'held')`,
		variantID, product.DefaultLocationID, uuid.NewString(), time.Now().Add(-time.Minute)).Error)

	avail, _, err := loadAvailability(context.Background(), db, uuid.NewString(),
		[]allocation.Warehouse{{ID: whID}}, []string{variantID})
	require.NoError(t, err)

	require.Equal(t, 5, avail.At(variantID, whID),
		"a hold expires by the clock, not by a sweeper running")
}

// A variant with no stock row anywhere must simply be absent, not zero-filled
// — allocation treats a missing entry as zero, and inventing rows would hide
// the difference between "none here" and "no such pairing".
func TestLoadAvailability_VariantWithNoStockRowsIsAbsent(t *testing.T) {
	db := testdb.NewTx(t)
	storeID, variantID := seedAvailStore(t, db)
	whID := seedWarehouseRow(t, db, storeID, "Main")

	avail, storage, err := loadAvailability(context.Background(), db, uuid.NewString(),
		[]allocation.Warehouse{{ID: whID}}, []string{variantID})
	require.NoError(t, err)

	require.Equal(t, 0, avail.At(variantID, whID))
	require.Empty(t, storage[variantID])
}
