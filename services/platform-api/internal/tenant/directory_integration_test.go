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

	apperrors "github.com/mark8ly/platform-api/pkg/errors"
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
			uuid.NewString(), tenantID, slug, name+" Store", "GB", "GBP", "Europe/London", "active",
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
		uuid.NewString(), id, "multi-two", "Second", "GB", "GBP", "Europe/London", "active",
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
		uuid.NewString(), id, "rollup-two", "Second", "GB", "GBP", "Europe/London", "suspended",
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

// TestIntegration_GetByOwnerEmail_ExactMatch exercises the happy path for
// the #279 admin-conversions lookup: an exact owner_email match returns
// the tenant that owns it.
func TestIntegration_GetByOwnerEmail_ExactMatch(t *testing.T) {
	tx := testdb.NewTx(t)
	repo := NewRepository(tx)
	ctx := context.Background()

	seed := newTenant("Acme Co", "uid-owner-1", "founder@acme.example")
	if err := repo.CreateInTx(ctx, tx, seed); err != nil {
		t.Fatalf("CreateInTx: %v", err)
	}

	got, err := repo.GetByOwnerEmail(ctx, "founder@acme.example")
	if err != nil {
		t.Fatalf("GetByOwnerEmail: %v", err)
	}
	if got.ID != seed.ID {
		t.Errorf("ID = %q, want %q", got.ID, seed.ID)
	}
	if got.Name != "Acme Co" {
		t.Errorf("Name = %q, want Acme Co", got.Name)
	}
}

// TestIntegration_GetByOwnerEmail_CaseInsensitive mirrors the unique index
// on lower(owner_email) (migration 0014): a differently-cased query must
// still find the tenant.
func TestIntegration_GetByOwnerEmail_CaseInsensitive(t *testing.T) {
	tx := testdb.NewTx(t)
	repo := NewRepository(tx)
	ctx := context.Background()

	seed := newTenant("Acme Co", "uid-owner-2", "founder@acme.example")
	if err := repo.CreateInTx(ctx, tx, seed); err != nil {
		t.Fatalf("CreateInTx: %v", err)
	}

	got, err := repo.GetByOwnerEmail(ctx, "Founder@ACME.example")
	if err != nil {
		t.Fatalf("GetByOwnerEmail: %v", err)
	}
	if got.ID != seed.ID {
		t.Errorf("ID = %q, want %q", got.ID, seed.ID)
	}
}

// TestIntegration_GetByOwnerEmail_WhitespaceTrimmed asserts surrounding
// whitespace on the query does not defeat the match.
func TestIntegration_GetByOwnerEmail_WhitespaceTrimmed(t *testing.T) {
	tx := testdb.NewTx(t)
	repo := NewRepository(tx)
	ctx := context.Background()

	seed := newTenant("Acme Co", "uid-owner-3", "founder@acme.example")
	if err := repo.CreateInTx(ctx, tx, seed); err != nil {
		t.Fatalf("CreateInTx: %v", err)
	}

	got, err := repo.GetByOwnerEmail(ctx, "  founder@acme.example  ")
	if err != nil {
		t.Fatalf("GetByOwnerEmail: %v", err)
	}
	if got.ID != seed.ID {
		t.Errorf("ID = %q, want %q", got.ID, seed.ID)
	}
}

// TestIntegration_GetByOwnerEmail_SubstringIsNotAMatch is the regression
// guard against anyone later "simplifying" this into the directory's
// ILIKE '%q%' search: a substring of a real owner_email must NOT match.
func TestIntegration_GetByOwnerEmail_SubstringIsNotAMatch(t *testing.T) {
	tx := testdb.NewTx(t)
	repo := NewRepository(tx)
	ctx := context.Background()

	seed := newTenant("Bob's Shop", "uid-owner-4", "bob@acme.example")
	if err := repo.CreateInTx(ctx, tx, seed); err != nil {
		t.Fatalf("CreateInTx: %v", err)
	}

	_, err := repo.GetByOwnerEmail(ctx, "ob@acme.example")
	ae, ok := apperrors.As(err)
	if !ok || ae.Code != "tenant_not_found" {
		t.Errorf("expected tenant_not_found for substring query, got %v", err)
	}
}

// TestIntegration_GetByOwnerEmail_UnseededEmailNotFound asserts an email
// nothing owns returns a typed not-found rather than a zero-value Tenant
// with a nil error.
func TestIntegration_GetByOwnerEmail_UnseededEmailNotFound(t *testing.T) {
	tx := testdb.NewTx(t)
	repo := NewRepository(tx)
	ctx := context.Background()

	got, err := repo.GetByOwnerEmail(ctx, "nobody@nowhere.example")
	if got != nil {
		t.Errorf("expected nil tenant, got %+v", got)
	}
	ae, ok := apperrors.As(err)
	if !ok || ae.Code != "tenant_not_found" {
		t.Errorf("expected tenant_not_found, got %v", err)
	}
}

// TestIntegration_GetByOwnerEmail_EmptyAndWhitespaceOnly asserts both an
// empty string and a whitespace-only string short-circuit to not-found
// without touching the DB.
func TestIntegration_GetByOwnerEmail_EmptyAndWhitespaceOnly(t *testing.T) {
	tx := testdb.NewTx(t)
	repo := NewRepository(tx)
	ctx := context.Background()

	for _, probe := range []string{"", "   "} {
		got, err := repo.GetByOwnerEmail(ctx, probe)
		if got != nil {
			t.Errorf("GetByOwnerEmail(%q): expected nil tenant, got %+v", probe, got)
		}
		ae, ok := apperrors.As(err)
		if !ok || ae.Code != "tenant_not_found" {
			t.Errorf("GetByOwnerEmail(%q): expected tenant_not_found, got %v", probe, err)
		}
	}
}

// TestListDirectory_FiltersByIDs seeds three tenants and filters by two of
// their ids: exactly those two must come back, not the third.
func TestListDirectory_FiltersByIDs(t *testing.T) {
	db := testdb.NewTx(t)
	repo := NewRepository(db)

	a := seedTenantWithStore(t, db, "IDs Co A", "ids-a@example.com", StatusActive, "ids-a")
	b := seedTenantWithStore(t, db, "IDs Co B", "ids-b@example.com", StatusActive, "ids-b")
	_ = seedTenantWithStore(t, db, "IDs Co C", "ids-c@example.com", StatusActive, "ids-c")

	got, err := repo.ListDirectory(context.Background(), DirectoryFilter{IDs: []string{a, b}, Limit: 50})
	require.NoError(t, err)
	require.Equal(t, int64(2), got.Total)

	ids := map[string]bool{}
	for _, tn := range got.Tenants {
		ids[tn.ID] = true
	}
	require.True(t, ids[a])
	require.True(t, ids[b])
	require.Len(t, got.Tenants, 2)
}

// TestListDirectory_UnknownIDIsIgnored asserts an id that matches nothing
// is silently dropped, not an error, and the known ids still return.
func TestListDirectory_UnknownIDIsIgnored(t *testing.T) {
	db := testdb.NewTx(t)
	repo := NewRepository(db)

	a := seedTenantWithStore(t, db, "Known Co", "known@example.com", StatusActive, "known-slug")

	got, err := repo.ListDirectory(context.Background(), DirectoryFilter{
		IDs:   []string{a, uuid.NewString()},
		Limit: 50,
	})
	require.NoError(t, err)
	require.Equal(t, int64(1), got.Total)
	require.Len(t, got.Tenants, 1)
	require.Equal(t, a, got.Tenants[0].ID)
}

// TestListDirectory_EmptyIDsReturnsEverything is the guard against the
// silently-empty-result regression: len(f.IDs) == 0 must add no clause.
func TestListDirectory_EmptyIDsReturnsEverything(t *testing.T) {
	db := testdb.NewTx(t)
	repo := NewRepository(db)

	a := seedTenantWithStore(t, db, "Guard Co A", "guard-a@example.com", StatusActive, "guard-a")
	b := seedTenantWithStore(t, db, "Guard Co B", "guard-b@example.com", StatusActive, "guard-b")

	got, err := repo.ListDirectory(context.Background(), DirectoryFilter{Q: "Guard Co", IDs: []string{}, Limit: 50})
	require.NoError(t, err)
	require.Equal(t, int64(2), got.Total)

	ids := map[string]bool{}
	for _, tn := range got.Tenants {
		ids[tn.ID] = true
	}
	require.True(t, ids[a])
	require.True(t, ids[b])
}

// TestListDirectory_IDsCombinesWithStatus asserts ids narrows within the
// status filter rather than replacing it: an id whose tenant doesn't match
// the status must not come back.
func TestListDirectory_IDsCombinesWithStatus(t *testing.T) {
	db := testdb.NewTx(t)
	repo := NewRepository(db)

	active := seedTenantWithStore(t, db, "Combo Active Co", "combo-active@example.com", StatusActive, "combo-active")
	suspended := seedTenantWithStore(t, db, "Combo Suspended Co", "combo-susp@example.com", StatusSuspended, "combo-susp")

	got, err := repo.ListDirectory(context.Background(), DirectoryFilter{
		IDs:    []string{active, suspended},
		Status: StatusActive,
		Limit:  50,
	})
	require.NoError(t, err)
	require.Equal(t, int64(1), got.Total)
	require.Len(t, got.Tenants, 1)
	require.Equal(t, active, got.Tenants[0].ID)
}
