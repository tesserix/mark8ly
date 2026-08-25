//go:build integration

package tenant

import (
	"context"
	"sort"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	apperrors "github.com/mark8ly/platform-api/pkg/errors"
	"github.com/mark8ly/platform-api/pkg/testdb"
)

// This file was rewritten for Phase Q: slug, country, currency and
// timezone moved from Tenant onto Store, and GetBySlug/SlugExists moved
// to the store package. The previous slug-based tests here referenced
// fields and methods that no longer exist and had stopped compiling.

func newTenant(name, uid, email string) *Tenant {
	return &Tenant{
		Name:        name,
		OwnerUserID: uid,
		OwnerEmail:  email,
		Status:      StatusActive,
	}
}

// TestIntegration_CreateInTx_HappyPath writes a tenant through the
// transaction path and reads it back through GetByID. Uses tx-rollback so
// nothing leaks between tests.
func TestIntegration_CreateInTx_HappyPath(t *testing.T) {
	tx := testdb.NewTx(t)
	repo := NewRepository(tx)
	ctx := context.Background()

	seed := newTenant("Acme Test Co", "uid-test-1", "founder@test.local")
	if err := repo.CreateInTx(ctx, tx, seed); err != nil {
		t.Fatalf("CreateInTx: %v", err)
	}
	if seed.ID == "" {
		t.Error("ID should be populated by gen_random_uuid()")
	}

	got, err := repo.GetByID(ctx, seed.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.Name != "Acme Test Co" {
		t.Errorf("Name = %q, want Acme Test Co", got.Name)
	}
	if got.OwnerEmail != "founder@test.local" {
		t.Errorf("OwnerEmail = %q, want founder@test.local", got.OwnerEmail)
	}
}

// TestIntegration_CreateInTx_TranslatesUniqueViolation is the
// conflict-mapping regression test. Since Phase Q the only unique
// constraint on tenants is tenants_owner_email_unique (migration 0014) —
// a second tenant claiming an owner_email must surface as a 409 conflict,
// NOT a generic 500.
func TestIntegration_CreateInTx_TranslatesUniqueViolation(t *testing.T) {
	tx := testdb.NewTx(t)
	repo := NewRepository(tx)
	ctx := context.Background()

	first := newTenant("First", "uid-1", "duplicate@test.local")
	if err := repo.CreateInTx(ctx, tx, first); err != nil {
		t.Fatalf("first insert: %v", err)
	}

	second := newTenant("Second", "uid-2", "duplicate@test.local")
	err := repo.CreateInTx(ctx, tx, second)

	ae, ok := apperrors.As(err)
	if !ok {
		t.Fatalf("expected AppError on duplicate owner_email, got %v", err)
	}
	if ae.Status != 409 {
		t.Errorf("Status = %d, want 409", ae.Status)
	}
}

// The unique index is on lower(owner_email), so a case variant must
// collide too — otherwise the DB backstop would not actually match what
// onboarding.Service checks in application code.
func TestIntegration_CreateInTx_OwnerEmailUniqueIsCaseInsensitive(t *testing.T) {
	tx := testdb.NewTx(t)
	repo := NewRepository(tx)
	ctx := context.Background()

	if err := repo.CreateInTx(ctx, tx, newTenant("First", "uid-1", "founder@case.local")); err != nil {
		t.Fatalf("first insert: %v", err)
	}

	err := repo.CreateInTx(ctx, tx, newTenant("Second", "uid-2", "Founder@Case.local"))
	if err == nil {
		t.Fatal("a case variant of an existing owner_email was accepted; want unique violation")
	}
	if ae, ok := apperrors.As(err); ok && ae.Status != 409 {
		t.Errorf("Status = %d, want 409", ae.Status)
	}
}

// TestIntegration_OwnerEmailExists_TrueAndFalse exercises the existence
// check that onboarding uses to reject a duplicate admin email at step 1.
func TestIntegration_OwnerEmailExists_TrueAndFalse(t *testing.T) {
	tx := testdb.NewTx(t)
	repo := NewRepository(tx)
	ctx := context.Background()

	seed := newTenant("X", "uid-4", "taken@test.local")
	if err := repo.CreateInTx(ctx, tx, seed); err != nil {
		t.Fatal(err)
	}

	for _, probe := range []string{
		"taken@test.local",
		"Taken@Test.local",
		"  taken@test.local  ",
	} {
		exists, err := repo.OwnerEmailExists(ctx, probe)
		if err != nil {
			t.Fatalf("OwnerEmailExists(%q): %v", probe, err)
		}
		if !exists {
			t.Errorf("OwnerEmailExists(%q) = false, want true", probe)
		}
	}

	exists, err := repo.OwnerEmailExists(ctx, "definitely-not-here@test.local")
	if err != nil {
		t.Fatal(err)
	}
	if exists {
		t.Error("OwnerEmailExists for unused email = true, want false")
	}
}

// TestIntegration_GetByOwnerUserID_FoundAndNotFound exercises the
// returning-user sign-in read path.
func TestIntegration_GetByOwnerUserID_FoundAndNotFound(t *testing.T) {
	tx := testdb.NewTx(t)
	repo := NewRepository(tx)
	ctx := context.Background()

	seed := newTenant("Lookup", "uid-3", "lookup@test.local")
	if err := repo.CreateInTx(ctx, tx, seed); err != nil {
		t.Fatal(err)
	}

	got, err := repo.GetByOwnerUserID(ctx, "uid-3")
	if err != nil {
		t.Fatalf("GetByOwnerUserID: %v", err)
	}
	if got.ID != seed.ID {
		t.Errorf("ID mismatch: got %q, want %q", got.ID, seed.ID)
	}

	_, err = repo.GetByOwnerUserID(ctx, "uid-does-not-exist")
	ae, ok := apperrors.As(err)
	if !ok || ae.Code != "tenant_not_found" {
		t.Errorf("expected tenant_not_found, got %v", err)
	}
}

// TestIntegration_DeleteInTx_ReconcilesOnboardingThenDeletes is the
// account-deletion regression test. onboarding_sessions.tenant_id is ON
// DELETE SET NULL, but the onboarding_sessions_completed_consistency CHECK
// requires tenant_id to stay NOT NULL while status='completed'. Without
// reconciling the session row first, deleting the tenant would null that
// column and the tenant DELETE itself would fail the CHECK. This seeds a
// tenant with a completed onboarding_session referencing it (raw SQL,
// mirroring the shape onboarding.Session writes — importing the onboarding
// package here would create an import cycle since it imports tenant) and
// asserts DeleteInTx removes both rows.
func TestIntegration_DeleteInTx_ReconcilesOnboardingThenDeletes(t *testing.T) {
	tx := testdb.NewTx(t)
	repo := NewRepository(tx)
	ctx := context.Background()

	seed := newTenant("ToDelete Co", "uid-del-1", "todelete@test.local")
	if err := repo.CreateInTx(ctx, tx, seed); err != nil {
		t.Fatalf("CreateInTx: %v", err)
	}

	if err := tx.Exec(`
		INSERT INTO onboarding_sessions (email, status, email_verified_at, tenant_id, completed_at)
		VALUES (?, 'completed', NOW(), ?, NOW())
	`, "todelete@test.local", seed.ID).Error; err != nil {
		t.Fatalf("seed onboarding_sessions: %v", err)
	}

	if err := repo.DeleteInTx(ctx, tx, seed.ID); err != nil {
		t.Fatalf("DeleteInTx: %v", err)
	}

	var tenantCount int64
	if err := tx.Raw(`SELECT count(*) FROM tenants WHERE id = ?`, seed.ID).Scan(&tenantCount).Error; err != nil {
		t.Fatalf("count tenants: %v", err)
	}
	if tenantCount != 0 {
		t.Errorf("tenant still present after DeleteInTx")
	}

	var sessionCount int64
	if err := tx.Raw(`SELECT count(*) FROM onboarding_sessions WHERE tenant_id = ?`, seed.ID).Scan(&sessionCount).Error; err != nil {
		t.Fatalf("count onboarding_sessions: %v", err)
	}
	if sessionCount != 0 {
		t.Errorf("onboarding_sessions row still present after DeleteInTx")
	}
}

// TestIntegration_DeleteInTx_NotFound asserts the NotFound mapping for a
// tenant id that doesn't exist, so callers (Task 4's teardown service) get
// a typed apperrors.NotFound rather than a silent no-op.
func TestIntegration_DeleteInTx_NotFound(t *testing.T) {
	tx := testdb.NewTx(t)
	repo := NewRepository(tx)
	ctx := context.Background()

	err := repo.DeleteInTx(ctx, tx, "00000000-0000-0000-0000-000000000000")
	ae, ok := apperrors.As(err)
	if !ok || ae.Code != "tenant_not_found" {
		t.Errorf("expected tenant_not_found, got %v", err)
	}
}

// seedTenantNamed inserts a tenant row with the given name and owner uid, returning the tenant ID.
// Used by snapshot tests that need specific tenant names and UIDs.
func seedTenantNamed(t *testing.T, db *gorm.DB, name, ownerUID string) string {
	t.Helper()
	var tenantID string
	require.NoError(t, db.Raw(
		`INSERT INTO tenants (name, owner_user_id, owner_email, status)
		 VALUES (?, ?, ?, ?)
		 RETURNING id`,
		name, ownerUID, ownerUID+"@example.com", StatusActive,
	).Scan(&tenantID).Error)
	return tenantID
}

// seedStoreWithSlug inserts a store row under tenantID with the given slug, returning the store ID.
// Uses GB/GBP/Europe/London — the reference rows actually present in platform-api's seed.
// Used by snapshot tests that need specific store slugs.
func seedStoreWithSlug(t *testing.T, db *gorm.DB, tenantID, slug string) string {
	t.Helper()
	var storeID string
	require.NoError(t, db.Raw(
		`INSERT INTO stores (tenant_id, slug, name, country_code, currency_code, timezone, status)
		 VALUES (?, ?, ?, ?, ?, ?, ?)
		 RETURNING id`,
		tenantID, slug, "Test Store", "GB", "GBP", "Europe/London", StatusActive,
	).Scan(&storeID).Error)
	return storeID
}

func TestSnapshotForTeardown_ReturnsNameOwnerAndStoreSlugs(t *testing.T) {
	db := testdb.NewTx(t)
	repo := NewRepository(db)

	tenantID := seedTenantNamed(t, db, "The Bondi Store", "owner-uid-1")
	seedStoreWithSlug(t, db, tenantID, "the-bondi-store")
	seedStoreWithSlug(t, db, tenantID, "bondi-outlet")

	// A SECOND tenant with its own store. A snapshot that ignores its
	// tenant_id filter would pick this up, and a one-tenant fixture
	// could never tell the difference.
	otherID := seedTenantNamed(t, db, "The Facade Factory", "owner-uid-2")
	seedStoreWithSlug(t, db, otherID, "the-facade-factory")

	var snap *TeardownSnapshot
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		var err error
		snap, err = repo.SnapshotForTeardown(t.Context(), tx, tenantID)
		return err
	}))

	require.Equal(t, tenantID, snap.TenantID)
	require.Equal(t, "The Bondi Store", snap.Name)
	require.Equal(t, "owner-uid-1", snap.OwnerUserID)

	slugs := make([]string, 0, len(snap.Stores))
	for _, s := range snap.Stores {
		require.NotEmpty(t, s.ID, "store id must be populated, not just the slug")
		slugs = append(slugs, s.Slug)
	}
	sort.Strings(slugs)
	require.Equal(t, []string{"bondi-outlet", "the-bondi-store"}, slugs)
}

func TestSnapshotForTeardown_UnknownTenantIsNotFound(t *testing.T) {
	db := testdb.NewTx(t)
	repo := NewRepository(db)

	err := db.Transaction(func(tx *gorm.DB) error {
		_, err := repo.SnapshotForTeardown(t.Context(), tx, uuid.NewString())
		return err
	})

	ae, ok := apperrors.As(err)
	require.True(t, ok, "want an *apperrors.AppError, got %T", err)
	require.Equal(t, "tenant_not_found", ae.Code)
}
