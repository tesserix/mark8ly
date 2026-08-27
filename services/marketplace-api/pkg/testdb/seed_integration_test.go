//go:build integration

package testdb_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

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
