//go:build integration

package tenant

import (
	"context"
	"testing"

	apperrors "github.com/mark8ly/platform-api/pkg/errors"
	"github.com/mark8ly/platform-api/pkg/testdb"
)

// TestIntegration_CreateInTx_HappyPath inserts a tenant via the real
// transaction path and reads it back through GetByID. Uses tx-rollback so
// nothing leaks between tests.
func TestIntegration_CreateInTx_HappyPath(t *testing.T) {
	tx := testdb.NewTx(t)
	repo := NewRepository(tx)

	tenant := &Tenant{
		Slug:         "acme-test",
		Name:         "Acme Test Co",
		OwnerUserID:  "uid-test-1",
		OwnerEmail:   "founder@test.local",
		CountryCode:  "US",
		CurrencyCode: "USD",
		Timezone:     "America/New_York",
		Status:       StatusActive,
	}
	if err := repo.CreateInTx(context.Background(), tx, tenant); err != nil {
		t.Fatalf("CreateInTx: %v", err)
	}
	if tenant.ID == "" {
		t.Error("ID should be populated by gen_random_uuid()")
	}

	got, err := repo.GetByID(context.Background(), tenant.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.Slug != "acme-test" {
		t.Errorf("Slug = %q, want acme-test", got.Slug)
	}
}

// TestIntegration_CreateInTx_TranslatesUniqueViolation is the regression
// test for the conflict-mapping path: a duplicate slug must surface as a
// 409 conflict (slug_taken), NOT a generic 500.
func TestIntegration_CreateInTx_TranslatesUniqueViolation(t *testing.T) {
	tx := testdb.NewTx(t)
	repo := NewRepository(tx)

	first := &Tenant{
		Slug: "duplicate-slug", Name: "First", OwnerUserID: "uid-1",
		OwnerEmail: "a@test.local", CountryCode: "US", CurrencyCode: "USD",
		Timezone: "America/New_York", Status: StatusActive,
	}
	if err := repo.CreateInTx(context.Background(), tx, first); err != nil {
		t.Fatalf("first insert: %v", err)
	}

	second := &Tenant{
		Slug: "duplicate-slug", Name: "Second", OwnerUserID: "uid-2",
		OwnerEmail: "b@test.local", CountryCode: "US", CurrencyCode: "USD",
		Timezone: "America/New_York", Status: StatusActive,
	}
	err := repo.CreateInTx(context.Background(), tx, second)

	ae, ok := apperrors.As(err)
	if !ok {
		t.Fatalf("expected AppError on duplicate slug, got %v", err)
	}
	if ae.Code != "slug_taken" {
		t.Errorf("Code = %q, want slug_taken", ae.Code)
	}
	if ae.Status != 409 {
		t.Errorf("Status = %d, want 409", ae.Status)
	}
}

// TestIntegration_GetBySlug_FoundAndNotFound exercises the read path.
func TestIntegration_GetBySlug_FoundAndNotFound(t *testing.T) {
	tx := testdb.NewTx(t)
	repo := NewRepository(tx)

	seed := &Tenant{
		Slug: "lookup-test", Name: "Lookup", OwnerUserID: "uid-3",
		OwnerEmail: "c@test.local", CountryCode: "US", CurrencyCode: "USD",
		Timezone: "America/New_York", Status: StatusActive,
	}
	if err := repo.CreateInTx(context.Background(), tx, seed); err != nil {
		t.Fatal(err)
	}

	got, err := repo.GetBySlug(context.Background(), "lookup-test")
	if err != nil {
		t.Fatalf("GetBySlug: %v", err)
	}
	if got.ID != seed.ID {
		t.Errorf("ID mismatch: got %q, want %q", got.ID, seed.ID)
	}

	_, err = repo.GetBySlug(context.Background(), "nope-not-here")
	ae, ok := apperrors.As(err)
	if !ok || ae.Code != "tenant_not_found" {
		t.Errorf("expected tenant_not_found, got %v", err)
	}
}

// TestIntegration_SlugExists_TrueAndFalse exercises the existence check
// used by IsSlugAvailable.
func TestIntegration_SlugExists_TrueAndFalse(t *testing.T) {
	tx := testdb.NewTx(t)
	repo := NewRepository(tx)

	seed := &Tenant{
		Slug: "exists-test", Name: "X", OwnerUserID: "uid-4",
		OwnerEmail: "d@test.local", CountryCode: "US", CurrencyCode: "USD",
		Timezone: "America/New_York", Status: StatusActive,
	}
	if err := repo.CreateInTx(context.Background(), tx, seed); err != nil {
		t.Fatal(err)
	}

	exists, err := repo.SlugExists(context.Background(), "exists-test")
	if err != nil {
		t.Fatal(err)
	}
	if !exists {
		t.Error("SlugExists(\"exists-test\") = false, want true")
	}

	notExists, err := repo.SlugExists(context.Background(), "definitely-not-here")
	if err != nil {
		t.Fatal(err)
	}
	if notExists {
		t.Error("SlugExists for unused slug = true, want false")
	}
}
