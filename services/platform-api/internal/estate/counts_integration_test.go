//go:build integration

package estate

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/mark8ly/platform-api/internal/store"
	"github.com/mark8ly/platform-api/internal/tenant"
	"github.com/mark8ly/platform-api/pkg/testdb"
)

// seedTenant inserts a tenant row with the given status and returns its id.
func seedTenant(t *testing.T, db *gorm.DB, status string) string {
	t.Helper()
	id := uuid.NewString()
	require.NoError(t, db.Exec(
		`INSERT INTO tenants (id, name, owner_user_id, owner_email, status)
		 VALUES (?, ?, ?, ?, ?)`,
		id, "Estate Co "+id[:8], "uid-"+id[:8], id[:8]+"@example.com", status,
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
		id, tenantID, "store-"+id[:8], "Estate Store", "GB", "GBP", "Europe/London", status,
	).Error)
	return id
}

func TestGet_CountsOnlyActiveTenants(t *testing.T) {
	db := testdb.NewTx(t)
	repo := NewRepository(db)

	seedTenant(t, db, tenant.StatusActive)
	seedTenant(t, db, tenant.StatusSuspended)

	got, err := repo.Get(context.Background())
	require.NoError(t, err)
	require.Equal(t, int64(1), got.TenantsActive)
}

func TestGet_CountsOnlyActiveStores(t *testing.T) {
	db := testdb.NewTx(t)
	repo := NewRepository(db)

	tenantID := seedTenant(t, db, tenant.StatusActive)
	seedStore(t, db, tenantID, store.StatusActive)
	seedStore(t, db, tenantID, "suspended")

	got, err := repo.Get(context.Background())
	require.NoError(t, err)
	require.Equal(t, int64(1), got.StoresActive)
}

// An empty estate is a real answer — zero, not an error. Zero counts and
// "we could not count" must stay distinguishable.
func TestGet_EmptyEstateReturnsZerosNoError(t *testing.T) {
	db := testdb.NewTx(t)
	repo := NewRepository(db)

	got, err := repo.Get(context.Background())
	require.NoError(t, err)
	require.Equal(t, int64(0), got.TenantsActive)
	require.Equal(t, int64(0), got.StoresActive)
}

// Tenants and stores are counted independently: one tenant with two active
// stores yields TenantsActive=1, StoresActive=2.
func TestGet_CountsTenantsAndStoresIndependently(t *testing.T) {
	db := testdb.NewTx(t)
	repo := NewRepository(db)

	tenantID := seedTenant(t, db, tenant.StatusActive)
	seedStore(t, db, tenantID, store.StatusActive)
	seedStore(t, db, tenantID, store.StatusActive)

	got, err := repo.Get(context.Background())
	require.NoError(t, err)
	require.Equal(t, int64(1), got.TenantsActive)
	require.Equal(t, int64(2), got.StoresActive)
}
