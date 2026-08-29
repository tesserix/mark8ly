//go:build integration

package testdb_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/order"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

func TestSeedStore_SatisfiesNotNullAndUniqueConstraints(t *testing.T) {
	db := testdb.NewTx(t)
	tenantID, storeID := uuid.New(), uuid.New()

	testdb.SeedStore(t, db, tenantID, storeID)

	var got struct {
		TenantID uuid.UUID
		Secret   string
	}
	err := db.Raw(
		`SELECT tenant_id, storefront_customer_portal_secret AS secret FROM stores WHERE id = ?`,
		storeID,
	).Scan(&got).Error
	require.NoError(t, err)
	require.Equal(t, tenantID, got.TenantID, "store must carry the caller's tenant")
	require.Len(t, got.Secret, 64, "portal secret must be 32 random bytes hex-encoded")
}

func TestSeedStore_TwoStoresInOneTestDoNotCollideOnSlug(t *testing.T) {
	db := testdb.NewTx(t)
	tenantID := uuid.New()

	testdb.SeedStore(t, db, tenantID, uuid.New())
	testdb.SeedStore(t, db, tenantID, uuid.New())

	var n int64
	require.NoError(t, db.Raw(`SELECT count(*) FROM stores WHERE tenant_id = ?`, tenantID).Scan(&n).Error)
	require.EqualValues(t, 2, n)
}

func TestSeedVendor_ReturnsUsableSelfVendorForTenant(t *testing.T) {
	db := testdb.NewTx(t)
	tenantID := uuid.New()

	vendorID := testdb.SeedVendor(t, db, tenantID)
	require.NotEqual(t, uuid.Nil, vendorID)

	var got struct {
		TenantID uuid.UUID
		IsSelf   bool
	}
	err := db.Raw(
		`SELECT tenant_id, is_self FROM vendors WHERE id = ?`, vendorID,
	).Scan(&got).Error
	require.NoError(t, err)
	require.Equal(t, tenantID, got.TenantID)
	require.True(t, got.IsSelf, "seeded vendor stands in for the tenant's self-vendor")
}

func TestSeedVendor_IsIdempotentPerTenant(t *testing.T) {
	db := testdb.NewTx(t)
	tenantID := uuid.New()

	first := testdb.SeedVendor(t, db, tenantID)
	second := testdb.SeedVendor(t, db, tenantID)

	require.Equal(t, first, second, "repeat calls must return the tenant's existing self-vendor")

	var n int64
	require.NoError(t, db.Raw(
		`SELECT count(*) FROM vendors WHERE tenant_id = ? AND is_self = true`, tenantID,
	).Scan(&n).Error)
	require.EqualValues(t, 1, n, "vendors_tenant_self_idx allows exactly one self-vendor per tenant")
}

func TestSeedVendor_DistinctTenantsGetDistinctVendors(t *testing.T) {
	db := testdb.NewTx(t)
	tenantA, tenantB := uuid.New(), uuid.New()

	vendorA := testdb.SeedVendor(t, db, tenantA)
	vendorB := testdb.SeedVendor(t, db, tenantB)

	require.NotEqual(t, vendorA, vendorB, "each tenant must get its own self-vendor")

	var n int64
	require.NoError(t, db.Raw(
		`SELECT count(*) FROM vendors WHERE id IN (?, ?) AND is_self = true`, vendorA, vendorB,
	).Scan(&n).Error)
	require.EqualValues(t, 2, n)
}

// A product insert is the actual thing 74 baseline failures could not do.
func TestSeededParents_AcceptAProductInsert(t *testing.T) {
	db := testdb.NewTx(t)
	tenantID, storeID := uuid.New(), uuid.New()

	testdb.SeedStore(t, db, tenantID, storeID)
	vendorID := testdb.SeedVendor(t, db, tenantID)

	err := db.Exec(
		`INSERT INTO products (id, tenant_id, store_id, vendor_id, handle, title, status)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		uuid.New(), tenantID, storeID, vendorID, "seed-probe", "Seed Probe", "draft",
	).Error
	require.NoError(t, err)
}

// SeedStore's cleanup must reclaim the two per-store sequences migration
// 000004's AFTER INSERT trigger creates, not just the stores row. This is
// the regression guard for #436: a sequence is a catalog relation, so
// neither the cleanup DELETE nor NewDB's TRUNCATE ... CASCADE reaches it,
// and without an explicit DROP every run of every NewDB-based integration
// test leaked two sequences permanently.
//
// NewDB (not NewTx) is essential here. Under NewTx the CREATE SEQUENCE the
// trigger issues is rolled back with the rest of the transaction, so the
// assertion would hold with or without the DROP and the test would prove
// nothing.
func TestSeedStore_CleanupDropsPerStoreSequences(t *testing.T) {
	db := testdb.NewDB(t, "stores")
	tenantID, storeID := uuid.New(), uuid.New()

	countSequences := func() int64 {
		t.Helper()
		var n int64
		require.NoError(t, db.Raw(
			`SELECT count(*) FROM pg_class WHERE relkind = 'S' AND relname IN (?, ?)`,
			order.SequenceName(storeID, "order"), order.SequenceName(storeID, "return"),
		).Scan(&n).Error)
		return n
	}

	// The nested subtest scopes SeedStore's t.Cleanup: it runs when the
	// subtest returns, so the outer test can observe the post-cleanup state.
	t.Run("fixture", func(st *testing.T) {
		testdb.SeedStore(st, db, tenantID, storeID)
		require.EqualValues(t, 2, countSequences(),
			"the stores AFTER INSERT trigger must have created both sequences")
	})

	require.EqualValues(t, 0, countSequences(),
		"SeedStore cleanup must DROP both per-store sequences; leaving them behind is the #436 leak")
}
