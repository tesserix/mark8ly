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

// The cheap half of #177.
//
// A "warehouse" used to be 8 warehouse_* columns hanging off
// shipping_carrier_configs — a row per (store, carrier). A merchant running
// Delhivery AND CouriersPlease typed the same physical address twice and
// kept the two copies in sync by hand. The address is a property of the
// STORE, not of the carrier account used to ship from it.
//
// Migration 000095 already did the expand half (table, backfill,
// warehouse_id). This is the repository that lets writers stop duplicating.
//
// Multi-warehouse ALLOCATION is deliberately not here — see #177, where the
// issue argues that choosing a warehouse per order is a product decision.
// One default warehouse per store is what this supports, which is exactly
// what every store has today.

func seedStore(t *testing.T, db *gorm.DB) (tenantID, storeID string) {
	t.Helper()
	tenantID, storeID = uuid.NewString(), uuid.NewString()
	require.NoError(t, db.Exec(
		`INSERT INTO stores (id, tenant_id, name, slug, status, country_code, currency_code, timezone,
		                     storefront_customer_portal_secret)
		 VALUES (?, ?, 'WH Test', ?, 'active', 'IN', 'INR', 'Asia/Kolkata', ?)`,
		storeID, tenantID, "wh-"+uuid.NewString()[:8], uuid.NewString()).Error)
	return tenantID, storeID
}

func sample(tenantID, storeID, name string) warehouse.Warehouse {
	return warehouse.Warehouse{
		TenantID: tenantID, StoreID: storeID, Name: name,
		Line1: "12 Industrial Estate", City: "Mumbai", Region: "MH",
		PostalCode: "400001", CountryCode: "IN", Phone: "+912200000000",
		ContactPerson: "Warehouse Manager",
	}
}

func TestUpsert_CreatesThenReusesTheSameRowForAStore(t *testing.T) {
	db := testdb.NewDB(t, "warehouses")
	repo := warehouse.NewRepository()
	ctx := context.Background()
	tenantID, storeID := seedStore(t, db)

	first, err := repo.Upsert(ctx, db, sample(tenantID, storeID, "Main"))
	require.NoError(t, err)
	require.NotEmpty(t, first.ID)

	// The SECOND carrier being configured for the same store must land on
	// the same warehouse — that is the duplication this removes.
	second, err := repo.Upsert(ctx, db, sample(tenantID, storeID, "Main"))
	require.NoError(t, err)
	require.Equal(t, first.ID, second.ID, "a second carrier must reuse the store's warehouse, not create another")

	var rows int64
	require.NoError(t, db.Raw(`SELECT count(*) FROM warehouses WHERE store_id = ?`, storeID).Scan(&rows).Error)
	require.Equal(t, int64(1), rows)
}

// An edited address must update in place, so both carriers see the change.
// Keeping two rows in sync by hand is precisely the failure being removed.
func TestUpsert_UpdatesTheExistingRowInPlace(t *testing.T) {
	db := testdb.NewDB(t, "warehouses")
	repo := warehouse.NewRepository()
	ctx := context.Background()
	tenantID, storeID := seedStore(t, db)

	created, err := repo.Upsert(ctx, db, sample(tenantID, storeID, "Main"))
	require.NoError(t, err)

	moved := sample(tenantID, storeID, "Main")
	moved.Line1 = "99 New Road"
	moved.PostalCode = "400002"
	updated, err := repo.Upsert(ctx, db, moved)
	require.NoError(t, err)

	require.Equal(t, created.ID, updated.ID)
	require.Equal(t, "99 New Road", updated.Line1)
	require.Equal(t, "400002", updated.PostalCode)
}

// Two stores must never share a warehouse row, even with the same name —
// they are different addresses belonging to different merchants.
func TestUpsert_IsScopedPerStore(t *testing.T) {
	db := testdb.NewDB(t, "warehouses")
	repo := warehouse.NewRepository()
	ctx := context.Background()
	t1, s1 := seedStore(t, db)
	t2, s2 := seedStore(t, db)

	a, err := repo.Upsert(ctx, db, sample(t1, s1, "Main"))
	require.NoError(t, err)
	b, err := repo.Upsert(ctx, db, sample(t2, s2, "Main"))
	require.NoError(t, err)

	require.NotEqual(t, a.ID, b.ID, "same name, different stores — must not collide")
}

func TestDefaultForStore_ReturnsNotFoundWhenNoneExists(t *testing.T) {
	db := testdb.NewDB(t, "warehouses")
	repo := warehouse.NewRepository()
	_, storeID := seedStore(t, db)

	_, err := repo.DefaultForStore(context.Background(), db, storeID)
	require.ErrorIs(t, err, warehouse.ErrNotFound,
		"absent must be distinguishable from a zero-value address — a caller "+
			"that filled a carrier config with blanks would ship nothing")
}

func TestDefaultForStore_ReturnsTheStoresWarehouse(t *testing.T) {
	db := testdb.NewDB(t, "warehouses")
	repo := warehouse.NewRepository()
	ctx := context.Background()
	tenantID, storeID := seedStore(t, db)

	created, err := repo.Upsert(ctx, db, sample(tenantID, storeID, "Main"))
	require.NoError(t, err)

	got, err := repo.DefaultForStore(ctx, db, storeID)
	require.NoError(t, err)
	require.Equal(t, created.ID, got.ID)
	require.Equal(t, "Mumbai", got.City)
	require.Equal(t, "Warehouse Manager", got.ContactPerson,
		"contact_person is the field Delhivery registration needs and the old "+
			"carrier columns never had")
}

// Migration 000095's backfill inserts no email/contact_person, so every
// pre-existing warehouse row has them NULL. Those rows are the common case
// in production, and a model that could not read them would break every
// store that existed before this table did.
func TestDefaultForStore_ReadsABackfilledRowWithNullContactColumns(t *testing.T) {
	db := testdb.NewDB(t, "warehouses")
	tenantID, storeID := seedStore(t, db)
	require.NoError(t, db.Exec(
		`INSERT INTO warehouses (id, tenant_id, store_id, name, line1, city, region,
		                         postal_code, country_code, phone, email, contact_person, is_default)
		 VALUES (?, ?, ?, 'Main', '12 Industrial Estate', 'Mumbai', 'MH',
		         '400001', 'IN', '+912200000000', NULL, NULL, true)`,
		uuid.NewString(), tenantID, storeID).Error)

	got, err := warehouse.NewRepository().DefaultForStore(context.Background(), db, storeID)
	require.NoError(t, err, "a backfilled row must be readable")
	require.Equal(t, "Mumbai", got.City)
	require.Empty(t, got.Email)
	require.Empty(t, got.ContactPerson)
}

// ByID is the read half of #177: every site that used to read the pickup
// address off shipping_carrier_configs.warehouse_* now looks it up by
// primary key via a config's warehouse_id. These two tests pin its
// contract directly, ahead of the handler-level tests that build on it.

func TestByID_ReturnsTheWarehouse(t *testing.T) {
	db := testdb.NewDB(t, "warehouses")
	repo := warehouse.NewRepository()
	ctx := context.Background()
	tenantID, storeID := seedStore(t, db)

	created, err := repo.Upsert(ctx, db, sample(tenantID, storeID, "Main"))
	require.NoError(t, err)

	got, err := repo.ByID(ctx, db, created.ID)
	require.NoError(t, err)
	require.Equal(t, created.ID, got.ID)
	require.Equal(t, "Mumbai", got.City)
	require.Equal(t, "Warehouse Manager", got.ContactPerson)
}

// A dangling id (the FK is ON DELETE SET NULL, so this should be rare, but
// not impossible under concurrent writes) must be distinguishable from a
// zero-value warehouse, exactly like DefaultForStore's ErrNotFound — every
// read-site caller falls back to the legacy columns on this error rather
// than failing the request outright.
func TestByID_ReturnsNotFoundForAMissingID(t *testing.T) {
	db := testdb.NewDB(t, "warehouses")
	repo := warehouse.NewRepository()

	_, err := repo.ByID(context.Background(), db, uuid.NewString())
	require.ErrorIs(t, err, warehouse.ErrNotFound)
}
