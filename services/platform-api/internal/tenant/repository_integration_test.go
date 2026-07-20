//go:build integration

package tenant

import (
	"context"
	"testing"

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
