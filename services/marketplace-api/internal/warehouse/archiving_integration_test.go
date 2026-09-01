//go:build integration

package warehouse_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/warehouse"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

// #177 PR 5a — archiving, the deletion refusals, and reordering.
//
// The rule these encode: a warehouse with ANY allocation history is
// ARCHIVED, never deleted. order_allocations.warehouse_id is ON DELETE
// RESTRICT because an allocation row is the record of which warehouse
// shipped a line; deleting the warehouse would corrupt that record.
// Delete therefore applies only to a warehouse with no history at all.

// seedAllocation writes one order_allocations row for a warehouse.
// shipped=false leaves shipment_id NULL — an unshipped parcel.
func seedAllocation(t *testing.T, db *gorm.DB, tenantID, storeID, warehouseID string, shipped bool) {
	t.Helper()
	orderID, itemID := uuid.NewString(), uuid.NewString()
	require.NoError(t, db.Exec(
		`INSERT INTO orders (id, tenant_id, store_id, order_number, idempotency_key,
		                     customer_email, status, payment_status, fulfillment_status,
		                     subtotal, shipping_total, tax_total, discount_total,
		                     grand_total, refunded_amount, currency_code, placed_at)
		 VALUES (?, ?, ?, ?, ?, 'wh-test@example.com', 'pending', 'pending', 'unfulfilled',
		         0, 0, 0, 0, 0, 0, 'INR', now())`,
		orderID, tenantID, storeID, "WH-"+uuid.NewString()[:8], uuid.NewString()).Error)
	require.NoError(t, db.Exec(
		`INSERT INTO order_items (id, order_id, title_snapshot, sku_snapshot,
		                          unit_price, quantity, line_total, currency_code)
		 VALUES (?, ?, 'WH Test Item', 'WH-SKU', 0, 1, 0, 'INR')`,
		itemID, orderID).Error)

	var shipmentID any
	if shipped {
		sid := uuid.NewString()
		require.NoError(t, db.Exec(
			`INSERT INTO shipments (id, tenant_id, store_id, order_id, carrier, status,
			                        ship_from, ship_to, handling_fee, currency_code)
			 VALUES (?, ?, ?, ?, 'shipengine', 'delivered', '{}'::jsonb, '{}'::jsonb, 0, 'INR')`,
			sid, tenantID, storeID, orderID).Error)
		shipmentID = sid
	}
	require.NoError(t, db.Exec(
		`INSERT INTO order_allocations (id, tenant_id, store_id, order_id, order_item_id,
		                                warehouse_id, quantity, shipment_id)
		 VALUES (?, ?, ?, ?, ?, ?, 1, ?)`,
		uuid.NewString(), tenantID, storeID, orderID, itemID, warehouseID, shipmentID).Error)
}

// seedVariantStock puts qty units of a fresh variant into a warehouse.
func seedVariantStock(t *testing.T, db *gorm.DB, tenantID, storeID, warehouseID string, qty int) string {
	t.Helper()
	productID := uuid.NewString()
	require.NoError(t, db.Exec(
		// status='active' requires published_at (products_published_requires_active).
		`INSERT INTO products (id, tenant_id, store_id, title, handle, status, vendor_id, published_at)
		 VALUES (?, ?, ?, 'WH Stock Product', ?, 'active', ?, now())`,
		productID, tenantID, storeID, "wh-"+uuid.NewString()[:8], uuid.NewString()).Error)

	variantID := uuid.NewString()
	require.NoError(t, db.Exec(
		`INSERT INTO product_variants (id, product_id, store_id, sku, price, currency_code)
		 VALUES (?, ?, ?, ?, 10.00, 'INR')`,
		variantID, productID, storeID, "SKU-"+uuid.NewString()[:8]).Error)

	require.NoError(t, db.Exec(
		`INSERT INTO variant_stock (variant_id, location_id, quantity, updated_at)
		 VALUES (?, ?, ?, now())`, variantID, warehouseID, qty).Error)
	return variantID
}

func TestDelete_RemovesAWarehouseWithNoHistory(t *testing.T) {
	db := testdb.NewDB(t, "warehouses")
	repo := warehouse.NewRepository()
	ctx := context.Background()
	tenantID, storeID := seedStore(t, db)

	wh, err := repo.Upsert(ctx, db, sample(tenantID, storeID, "Spare"))
	require.NoError(t, err)

	require.NoError(t, repo.Delete(ctx, db, wh.ID))

	_, err = repo.ByID(ctx, db, wh.ID)
	require.ErrorIs(t, err, warehouse.ErrNotFound)
}

// Deleting a warehouse that still holds units would drop them from the
// store's availability with no record they existed.
func TestDelete_RefusesWhileStockRemains(t *testing.T) {
	db := testdb.NewDB(t, "warehouses")
	repo := warehouse.NewRepository()
	ctx := context.Background()
	tenantID, storeID := seedStore(t, db)

	wh, err := repo.Upsert(ctx, db, sample(tenantID, storeID, "Stocked"))
	require.NoError(t, err)
	seedVariantStock(t, db, tenantID, storeID, wh.ID, 5)

	require.ErrorIs(t, repo.Delete(ctx, db, wh.ID), warehouse.ErrHasStock)
}

// An unshipped allocation means the warehouse owes a parcel; removing its
// origin leaves an order that can never be fulfilled.
func TestDelete_RefusesWithAnUnshippedParcel(t *testing.T) {
	db := testdb.NewDB(t, "warehouses")
	repo := warehouse.NewRepository()
	ctx := context.Background()
	tenantID, storeID := seedStore(t, db)

	wh, err := repo.Upsert(ctx, db, sample(tenantID, storeID, "Owing"))
	require.NoError(t, err)
	seedAllocation(t, db, tenantID, storeID, wh.ID, false)

	require.ErrorIs(t, repo.Delete(ctx, db, wh.ID), warehouse.ErrHasUnshippedParcel)
}

// Fully shipped is still history. The caller must archive instead — and
// this is the case the FK's RESTRICT would otherwise reject with a raw
// database error.
func TestDelete_RefusesShippedHistoryAndDirectsToArchive(t *testing.T) {
	db := testdb.NewDB(t, "warehouses")
	repo := warehouse.NewRepository()
	ctx := context.Background()
	tenantID, storeID := seedStore(t, db)

	wh, err := repo.Upsert(ctx, db, sample(tenantID, storeID, "Retired"))
	require.NoError(t, err)
	seedAllocation(t, db, tenantID, storeID, wh.ID, true)

	require.ErrorIs(t, repo.Delete(ctx, db, wh.ID), warehouse.ErrHasHistory)

	// ...and archiving it works, which is the whole point of the refusal.
	require.NoError(t, repo.Archive(ctx, db, wh.ID))
}

// The trap the partial index exists to prevent: archiving must not burn the
// name, or a merchant who archives "Main Warehouse" can never create another.
func TestArchive_FreesTheNameForReuse(t *testing.T) {
	db := testdb.NewDB(t, "warehouses")
	repo := warehouse.NewRepository()
	ctx := context.Background()
	tenantID, storeID := seedStore(t, db)

	first, err := repo.Upsert(ctx, db, sample(tenantID, storeID, "Main Warehouse"))
	require.NoError(t, err)
	require.NoError(t, repo.Archive(ctx, db, first.ID))

	second, err := repo.Upsert(ctx, db, sample(tenantID, storeID, "Main Warehouse"))
	require.NoError(t, err)
	require.NotEqual(t, first.ID, second.ID, "reusing an archived name must create a NEW warehouse, not resurrect the archived one")
}

// An archived warehouse must disappear from listings and stop being the
// default — otherwise DefaultForStore keeps handing out a warehouse nothing
// is allowed to allocate to.
func TestArchive_ExcludesFromListAndClearsDefault(t *testing.T) {
	db := testdb.NewDB(t, "warehouses")
	repo := warehouse.NewRepository()
	ctx := context.Background()
	tenantID, storeID := seedStore(t, db)

	keep, err := repo.Upsert(ctx, db, sample(tenantID, storeID, "Keep"))
	require.NoError(t, err)
	gone, err := repo.Upsert(ctx, db, sample(tenantID, storeID, "Gone"))
	require.NoError(t, err)
	require.NoError(t, db.Exec(`UPDATE warehouses SET is_default = true WHERE id = ?`, gone.ID).Error)

	require.NoError(t, repo.Archive(ctx, db, gone.ID))

	live, err := repo.List(ctx, db, storeID, false)
	require.NoError(t, err)
	require.Len(t, live, 1)
	require.Equal(t, keep.ID, live[0].ID)

	all, err := repo.List(ctx, db, storeID, true)
	require.NoError(t, err)
	require.Len(t, all, 2, "includeArchived must still show it")

	var isDefault bool
	require.NoError(t, db.Raw(`SELECT is_default FROM warehouses WHERE id = ?`, gone.ID).Scan(&isDefault).Error)
	require.False(t, isDefault, "an archived warehouse must not remain the store's default")
}

// A double-click must not rewrite the archive timestamp.
func TestArchive_IsIdempotent(t *testing.T) {
	db := testdb.NewDB(t, "warehouses")
	repo := warehouse.NewRepository()
	ctx := context.Background()
	tenantID, storeID := seedStore(t, db)

	wh, err := repo.Upsert(ctx, db, sample(tenantID, storeID, "Twice"))
	require.NoError(t, err)
	require.NoError(t, repo.Archive(ctx, db, wh.ID))

	var first string
	require.NoError(t, db.Raw(`SELECT archived_at::text FROM warehouses WHERE id = ?`, wh.ID).Scan(&first).Error)

	require.NoError(t, repo.Archive(ctx, db, wh.ID))

	var second string
	require.NoError(t, db.Raw(`SELECT archived_at::text FROM warehouses WHERE id = ?`, wh.ID).Scan(&second).Error)
	require.Equal(t, first, second, "re-archiving must not move the timestamp")
}

func TestArchive_UnknownIDIsNotFound(t *testing.T) {
	db := testdb.NewDB(t, "warehouses")
	repo := warehouse.NewRepository()
	require.ErrorIs(t, repo.Archive(context.Background(), db, uuid.NewString()), warehouse.ErrNotFound)
}

// Ordering must be stable: two warehouses at the same priority would
// otherwise swap places between calls and make the reorder UI jump.
func TestList_OrdersByPriorityThenName(t *testing.T) {
	db := testdb.NewDB(t, "warehouses")
	repo := warehouse.NewRepository()
	ctx := context.Background()
	tenantID, storeID := seedStore(t, db)

	for _, n := range []string{"Bravo", "Alpha", "Charlie"} {
		_, err := repo.Upsert(ctx, db, sample(tenantID, storeID, n))
		require.NoError(t, err)
	}
	// All at priority 0 → name breaks the tie.
	got, err := repo.List(ctx, db, storeID, false)
	require.NoError(t, err)
	require.Equal(t, []string{"Alpha", "Bravo", "Charlie"}, namesOf(got))
}

func TestSetPriorities_ReordersInOneTransaction(t *testing.T) {
	db := testdb.NewDB(t, "warehouses")
	repo := warehouse.NewRepository()
	ctx := context.Background()
	tenantID, storeID := seedStore(t, db)

	a, err := repo.Upsert(ctx, db, sample(tenantID, storeID, "Alpha"))
	require.NoError(t, err)
	b, err := repo.Upsert(ctx, db, sample(tenantID, storeID, "Bravo"))
	require.NoError(t, err)

	require.NoError(t, repo.SetPriorities(ctx, db, storeID, []warehouse.PriorityUpdate{
		{ID: b.ID, Priority: 0},
		{ID: a.ID, Priority: 1},
	}))

	got, err := repo.List(ctx, db, storeID, false)
	require.NoError(t, err)
	require.Equal(t, []string{"Bravo", "Alpha"}, namesOf(got))
}

// A partial reorder is a silently different allocation order. One bad id
// must roll the whole thing back.
func TestSetPriorities_RollsBackEntirelyOnABadID(t *testing.T) {
	db := testdb.NewDB(t, "warehouses")
	repo := warehouse.NewRepository()
	ctx := context.Background()
	tenantID, storeID := seedStore(t, db)

	a, err := repo.Upsert(ctx, db, sample(tenantID, storeID, "Alpha"))
	require.NoError(t, err)
	b, err := repo.Upsert(ctx, db, sample(tenantID, storeID, "Bravo"))
	require.NoError(t, err)

	err = repo.SetPriorities(ctx, db, storeID, []warehouse.PriorityUpdate{
		{ID: b.ID, Priority: 0},
		{ID: uuid.NewString(), Priority: 1}, // not this store's
	})
	require.Error(t, err)

	// Bravo's priority must NOT have been written.
	got, err := repo.List(ctx, db, storeID, false)
	require.NoError(t, err)
	require.Equal(t, []string{"Alpha", "Bravo"}, namesOf(got), "a failed reorder must leave the original order intact")
	_ = a
}

// Another store's warehouse must not be reprioritisable through a crafted id.
func TestSetPriorities_IgnoresAnotherStoresWarehouse(t *testing.T) {
	db := testdb.NewDB(t, "warehouses")
	repo := warehouse.NewRepository()
	ctx := context.Background()
	tenantA, storeA := seedStore(t, db)
	_, storeB := seedStore(t, db)

	mine, err := repo.Upsert(ctx, db, sample(tenantA, storeA, "Mine"))
	require.NoError(t, err)

	err = repo.SetPriorities(ctx, db, storeB, []warehouse.PriorityUpdate{{ID: mine.ID, Priority: 9}})
	require.Error(t, err, "an id belonging to another store must be refused")

	var priority int
	require.NoError(t, db.Raw(`SELECT priority FROM warehouses WHERE id = ?`, mine.ID).Scan(&priority).Error)
	require.Equal(t, 0, priority)
}

func namesOf(ws []warehouse.Warehouse) []string {
	out := make([]string, 0, len(ws))
	for _, w := range ws {
		out = append(out, w.Name)
	}
	return out
}

// Archive() clears is_default, but DefaultForStore's ordering — is_default
// DESC, created_at ASC — would still hand back an ARCHIVED row whenever it
// is the oldest, or whenever every warehouse has been archived. Callers use
// this to fill a carrier config's pickup address, so an archived answer
// binds a live carrier to a warehouse nothing is allowed to allocate to.
func TestDefaultForStore_SkipsArchived(t *testing.T) {
	db := testdb.NewDB(t, "warehouses")
	repo := warehouse.NewRepository()
	ctx := context.Background()
	tenantID, storeID := seedStore(t, db)

	// Oldest first, so it wins the created_at ASC tie-break once both have
	// is_default = false.
	old, err := repo.Upsert(ctx, db, sample(tenantID, storeID, "Old"))
	require.NoError(t, err)
	current, err := repo.Upsert(ctx, db, sample(tenantID, storeID, "Current"))
	require.NoError(t, err)

	require.NoError(t, repo.Archive(ctx, db, old.ID))

	got, err := repo.DefaultForStore(ctx, db, storeID)
	require.NoError(t, err)
	require.Equal(t, current.ID, got.ID,
		"an archived warehouse must never be offered as the store default")
}

// Every warehouse archived is not "the oldest one, then" — it is the same
// answer as a store with no warehouses at all.
func TestDefaultForStore_AllArchivedIsNotFound(t *testing.T) {
	db := testdb.NewDB(t, "warehouses")
	repo := warehouse.NewRepository()
	ctx := context.Background()
	tenantID, storeID := seedStore(t, db)

	only, err := repo.Upsert(ctx, db, sample(tenantID, storeID, "Only"))
	require.NoError(t, err)
	require.NoError(t, repo.Archive(ctx, db, only.ID))

	_, err = repo.DefaultForStore(ctx, db, storeID)
	require.ErrorIs(t, err, warehouse.ErrNotFound)
}
