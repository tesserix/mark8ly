//go:build integration

package tenant

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/mark8ly/platform-api/pkg/testdb"
)

// seedTenantWithStore inserts a tenant and one store, returning the tenant id.
func seedTenantWithStore(t *testing.T, db *gorm.DB, name, ownerEmail, status, slug string) string {
	t.Helper()
	tenantID := uuid.NewString()
	require.NoError(t, db.Exec(
		`INSERT INTO tenants (id, name, owner_user_id, owner_email, status)
		 VALUES (?, ?, ?, ?, ?)`,
		tenantID, name, "uid-"+tenantID[:8], ownerEmail, status,
	).Error)
	if slug != "" {
		require.NoError(t, db.Exec(
			`INSERT INTO stores (id, tenant_id, slug, name, country_code, currency_code, timezone, status)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			uuid.NewString(), tenantID, slug, name+" Store", "IE", "EUR", "Europe/Dublin", "active",
		).Error)
	}
	return tenantID
}

// The case a literal reading of #277 would have missed: the tenant has no
// slug of its own, so it must be findable by its STORE's slug.
func TestListDirectory_MatchesStoreSlug(t *testing.T) {
	db := testdb.NewTx(t)
	repo := NewRepository(db)

	id := seedTenantWithStore(t, db, "Unrelated Name", "unrelated@example.com", StatusActive, "findme-slug")

	got, err := repo.ListDirectory(context.Background(), DirectoryFilter{Q: "findme", Limit: 50})
	require.NoError(t, err)
	require.Equal(t, int64(1), got.Total)
	require.Len(t, got.Tenants, 1)
	require.Equal(t, id, got.Tenants[0].ID)
}

func TestListDirectory_MatchesNameAndOwnerEmail(t *testing.T) {
	db := testdb.NewTx(t)
	repo := NewRepository(db)

	byName := seedTenantWithStore(t, db, "Acme Trading", "someone@example.com", StatusActive, "acme-store")
	byEmail := seedTenantWithStore(t, db, "Other Co", "founder@distinctive.test", StatusActive, "other-store")

	byNameRes, err := repo.ListDirectory(context.Background(), DirectoryFilter{Q: "acme", Limit: 50})
	require.NoError(t, err)
	require.Equal(t, int64(1), byNameRes.Total)
	require.Equal(t, byName, byNameRes.Tenants[0].ID)

	byEmailRes, err := repo.ListDirectory(context.Background(), DirectoryFilter{Q: "distinctive", Limit: 50})
	require.NoError(t, err)
	require.Equal(t, int64(1), byEmailRes.Total)
	require.Equal(t, byEmail, byEmailRes.Tenants[0].ID)
}

// A tenant with two stores must appear ONCE, not once per store. The join
// makes duplicate rows the default failure mode.
func TestListDirectory_DeduplicatesAcrossStores(t *testing.T) {
	db := testdb.NewTx(t)
	repo := NewRepository(db)

	id := seedTenantWithStore(t, db, "Multi Store Co", "multi@example.com", StatusActive, "multi-one")
	require.NoError(t, db.Exec(
		`INSERT INTO stores (id, tenant_id, slug, name, country_code, currency_code, timezone, status)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		uuid.NewString(), id, "multi-two", "Second", "IE", "EUR", "Europe/Dublin", "active",
	).Error)

	got, err := repo.ListDirectory(context.Background(), DirectoryFilter{Q: "multi", Limit: 50})
	require.NoError(t, err)
	require.Equal(t, int64(1), got.Total, "a tenant with two stores must count once")
	require.Len(t, got.Tenants, 1)
}

func TestListDirectory_FiltersByStatusAndCreatedRange(t *testing.T) {
	db := testdb.NewTx(t)
	repo := NewRepository(db)

	active := seedTenantWithStore(t, db, "Active Co", "a@example.com", StatusActive, "active-slug")
	seedTenantWithStore(t, db, "Suspended Co", "s@example.com", StatusSuspended, "susp-slug")

	res, err := repo.ListDirectory(context.Background(), DirectoryFilter{Status: StatusActive, Limit: 50})
	require.NoError(t, err)
	require.Equal(t, int64(1), res.Total)
	require.Equal(t, active, res.Tenants[0].ID)

	future, err := repo.ListDirectory(context.Background(), DirectoryFilter{
		CreatedFrom: time.Now().Add(24 * time.Hour), Limit: 50,
	})
	require.NoError(t, err)
	require.Equal(t, int64(0), future.Total)
	require.Empty(t, future.Tenants)
}

// total is the UNPAGINATED count. A total that equals the page size makes
// the console's page arithmetic silently wrong.
func TestListDirectory_TotalIsUnpaginated(t *testing.T) {
	db := testdb.NewTx(t)
	repo := NewRepository(db)

	// tenants.owner_email has a real unique index (lower(owner_email), see
	// migration 0014), so each seeded tenant needs a distinct email — the
	// brief's literal "p@example.com" for all three would violate it.
	for i := 0; i < 3; i++ {
		seedTenantWithStore(t, db, "Paged Co", fmt.Sprintf("p%d@example.com", i), StatusActive, "paged-"+uuid.NewString()[:8])
	}

	got, err := repo.ListDirectory(context.Background(), DirectoryFilter{Q: "Paged Co", Limit: 1})
	require.NoError(t, err)
	require.Equal(t, int64(3), got.Total)
	require.Len(t, got.Tenants, 1)
}

// The platform view returns tenants regardless of who is asking. There is no
// caller identity in DirectoryFilter at all, which is the point — but assert
// it, because a later "helpful" scoping change would break the console
// silently: it would just see fewer tenants, not an error.
func TestListDirectory_IsNotCallerScoped(t *testing.T) {
	db := testdb.NewTx(t)
	repo := NewRepository(db)

	a := seedTenantWithStore(t, db, "Owner A Co", "owner-a@example.com", StatusActive, "scope-a")
	b := seedTenantWithStore(t, db, "Owner B Co", "owner-b@example.com", StatusActive, "scope-b")

	got, err := repo.ListDirectory(context.Background(), DirectoryFilter{Q: "Owner", Limit: 50})
	require.NoError(t, err)
	require.Equal(t, int64(2), got.Total, "both tenants must appear regardless of ownership")

	ids := map[string]bool{}
	for _, tn := range got.Tenants {
		ids[tn.ID] = true
	}
	require.True(t, ids[a])
	require.True(t, ids[b])
}

func TestListDirectory_ClampsLimit(t *testing.T) {
	db := testdb.NewTx(t)
	repo := NewRepository(db)

	got, err := repo.ListDirectory(context.Background(), DirectoryFilter{Limit: 100000})
	require.NoError(t, err)
	require.LessOrEqual(t, len(got.Tenants), MaxDirectoryPageSize)
	require.NotNil(t, got.Tenants, "must be an allocated slice, never nil")
}

func TestGetWithStores_ReturnsRollup(t *testing.T) {
	db := testdb.NewTx(t)
	repo := NewRepository(db)

	id := seedTenantWithStore(t, db, "Rollup Co", "r@example.com", StatusActive, "rollup-one")
	require.NoError(t, db.Exec(
		`INSERT INTO stores (id, tenant_id, slug, name, country_code, currency_code, timezone, status)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		uuid.NewString(), id, "rollup-two", "Second", "IE", "EUR", "Europe/Dublin", "suspended",
	).Error)

	got, err := repo.GetWithStores(context.Background(), id)
	require.NoError(t, err)
	require.Equal(t, id, got.ID)
	require.Len(t, got.Stores, 2)

	statuses := map[string]string{}
	for _, s := range got.Stores {
		statuses[s.Slug] = s.Status
	}
	require.Equal(t, "active", statuses["rollup-one"])
	require.Equal(t, "suspended", statuses["rollup-two"])
}

func TestGetWithStores_TenantWithNoStores(t *testing.T) {
	db := testdb.NewTx(t)
	repo := NewRepository(db)

	id := seedTenantWithStore(t, db, "Storeless Co", "n@example.com", StatusActive, "")

	got, err := repo.GetWithStores(context.Background(), id)
	require.NoError(t, err)
	require.NotNil(t, got.Stores, "must be an allocated slice, never nil")
	require.Empty(t, got.Stores)
}

func TestGetWithStores_NotFound(t *testing.T) {
	db := testdb.NewTx(t)
	repo := NewRepository(db)

	_, err := repo.GetWithStores(context.Background(), uuid.NewString())
	require.Error(t, err)
}
