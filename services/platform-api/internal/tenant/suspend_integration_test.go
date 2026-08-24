//go:build integration

package tenant

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/mark8ly/platform-api/pkg/testdb"
)

// seedTenant inserts a tenant row with the given status and returns its id.
func seedTenant(t *testing.T, db *gorm.DB, status string) string {
	t.Helper()
	id := uuid.NewString()
	require.NoError(t, db.Exec(
		`INSERT INTO tenants (id, name, owner_user_id, owner_email, status)
		 VALUES (?, ?, ?, ?, ?)`,
		id, "Suspend Co "+id[:8], "uid-"+id[:8], id[:8]+"@example.com", status,
	).Error)
	return id
}

// seedStore inserts a store row under tenantID with the given status. Uses
// GB/GBP/Europe/London — the reference rows actually present in
// platform-api's seed (IE/Europe/Dublin are NOT seeded and would fail the FK).
func seedStore(t *testing.T, db *gorm.DB, tenantID, status string) string {
	t.Helper()
	id := uuid.NewString()
	require.NoError(t, db.Exec(
		`INSERT INTO stores (id, tenant_id, slug, name, country_code, currency_code, timezone, status)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		id, tenantID, "store-"+id[:8], "Suspend Store", "GB", "GBP", "Europe/London", status,
	).Error)
	return id
}

func storeStatus(t *testing.T, db *gorm.DB, storeID string) string {
	t.Helper()
	var status string
	require.NoError(t, db.Raw(`SELECT status FROM stores WHERE id = ?`, storeID).Scan(&status).Error)
	return status
}

func suspendedByTenant(t *testing.T, db *gorm.DB, storeID string) bool {
	t.Helper()
	var flag bool
	require.NoError(t, db.Raw(`SELECT suspended_by_tenant FROM stores WHERE id = ?`, storeID).Scan(&flag).Error)
	return flag
}

// TestSuspendThenUnsuspendPreservesIndividuallySuspendedStore is the whole
// reason suspended_by_tenant exists. A store suspended on its own, before
// the tenant was suspended, must still be suspended after the tenant is
// unsuspended. A cascade that just sets everything back to active fails
// exactly here and nowhere else.
func TestSuspendThenUnsuspendPreservesIndividuallySuspendedStore(t *testing.T) {
	db := testdb.NewTx(t)
	repo := NewRepository(db)
	ctx := context.Background()

	tenantID := seedTenant(t, db, StatusActive)
	activeStore := seedStore(t, db, tenantID, "active")
	alreadySuspended := seedStore(t, db, tenantID, "suspended")

	// Suspend the tenant.
	res, err := repo.Suspend(ctx, tenantID)
	require.NoError(t, err)
	require.True(t, res.Changed)
	require.Equal(t, StatusSuspended, res.Status)
	require.Equal(t, 1, res.StoresAffected, "only the ACTIVE store should be affected")

	// Unsuspend it.
	res, err = repo.Unsuspend(ctx, tenantID)
	require.NoError(t, err)
	require.True(t, res.Changed)
	require.Equal(t, 1, res.StoresAffected)

	require.Equal(t, "active", storeStatus(t, db, activeStore),
		"the store the cascade suspended must be restored")
	require.Equal(t, "suspended", storeStatus(t, db, alreadySuspended),
		"a store suspended individually BEFORE the tenant suspension must stay suspended")
	require.False(t, suspendedByTenant(t, db, activeStore), "flag must be cleared after unsuspend")
}

// A second suspend is a no-op: no extra stores affected, Changed false, and
// the flag on already-cascaded rows is untouched.
func TestSuspendIsIdempotent(t *testing.T) {
	db := testdb.NewTx(t)
	repo := NewRepository(db)
	ctx := context.Background()

	tenantID := seedTenant(t, db, StatusActive)
	s := seedStore(t, db, tenantID, "active")

	first, err := repo.Suspend(ctx, tenantID)
	require.NoError(t, err)
	require.True(t, first.Changed)
	require.Equal(t, 1, first.StoresAffected)

	second, err := repo.Suspend(ctx, tenantID)
	require.NoError(t, err)
	require.False(t, second.Changed, "already suspended: no-op")
	require.Equal(t, 0, second.StoresAffected)
	require.True(t, suspendedByTenant(t, db, s), "flag must survive a repeat suspend")
}
