package tenant

import (
	"context"
	"sync"
	"testing"

	"gorm.io/gorm"

	apperrors "github.com/mark8ly/platform-api/pkg/errors"
)

// fakeRepo is an in-memory Repository for unit tests.
type fakeRepo struct {
	mu     sync.Mutex
	bySlug map[string]*Tenant
	byID   map[string]*Tenant
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{
		bySlug: map[string]*Tenant{},
		byID:   map[string]*Tenant{},
	}
}

func (f *fakeRepo) seed(t *Tenant) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if t.ID == "" {
		t.ID = "id-" + t.Slug
	}
	f.bySlug[t.Slug] = t
	f.byID[t.ID] = t
}

func (f *fakeRepo) CreateInTx(ctx context.Context, tx *gorm.DB, t *Tenant) error { return nil }

func (f *fakeRepo) GetByID(ctx context.Context, id string) (*Tenant, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	t, ok := f.byID[id]
	if !ok {
		return nil, apperrors.NotFound("tenant_not_found", "no")
	}
	return t, nil
}

func (f *fakeRepo) GetBySlug(ctx context.Context, slug string) (*Tenant, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	t, ok := f.bySlug[slug]
	if !ok {
		return nil, apperrors.NotFound("tenant_not_found", "no")
	}
	return t, nil
}

func (f *fakeRepo) SlugExists(ctx context.Context, slug string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	_, ok := f.bySlug[slug]
	return ok, nil
}

// ─────────────────────────────────────────────────────────────────────────

func TestService_IsSlugAvailable_ReturnsTrueForUnusedSlug(t *testing.T) {
	svc := NewService(newFakeRepo())
	ok, err := svc.IsSlugAvailable(context.Background(), "acme")
	if err != nil {
		t.Fatalf("error = %v", err)
	}
	if !ok {
		t.Error("expected available, got taken")
	}
}

func TestService_IsSlugAvailable_ReturnsFalseForTakenSlug(t *testing.T) {
	repo := newFakeRepo()
	repo.seed(&Tenant{Slug: "acme"})
	svc := NewService(repo)

	ok, err := svc.IsSlugAvailable(context.Background(), "acme")
	if err != nil {
		t.Fatalf("error = %v", err)
	}
	if ok {
		t.Error("expected taken, got available")
	}
}

func TestService_IsSlugAvailable_RejectsMalformedSlugs(t *testing.T) {
	svc := NewService(newFakeRepo())
	cases := []string{
		"",        // empty
		"a",       // too short
		"-leading", // leading hyphen
		"trailing-", // trailing hyphen
		"UPPER",   // uppercase
		"has space",
		"has.dot",
	}
	for _, slug := range cases {
		t.Run(slug, func(t *testing.T) {
			_, err := svc.IsSlugAvailable(context.Background(), slug)
			ae, ok := apperrors.As(err)
			if !ok || ae.Code != "invalid_slug" {
				t.Errorf("expected invalid_slug, got %v", err)
			}
		})
	}
}

func TestService_IsSlugAvailable_TrimsWhitespaceButNotCase(t *testing.T) {
	svc := NewService(newFakeRepo())

	// Whitespace IS trimmed.
	if _, err := svc.IsSlugAvailable(context.Background(), "  acme  "); err != nil {
		t.Errorf("whitespace should be trimmed, got %v", err)
	}
	// Uppercase is NOT silently lowercased — it's rejected so the user
	// commits to a single canonical casing.
	if _, err := svc.IsSlugAvailable(context.Background(), "Acme"); err == nil {
		t.Error("uppercase should be rejected, not normalized")
	}
}

func TestService_GetByID_RejectsEmpty(t *testing.T) {
	svc := NewService(newFakeRepo())
	_, err := svc.GetByID(context.Background(), "")
	ae, ok := apperrors.As(err)
	if !ok || ae.Code != "invalid_tenant_id" {
		t.Errorf("expected invalid_tenant_id, got %v", err)
	}
}

func TestService_GetByID_PropagatesNotFound(t *testing.T) {
	svc := NewService(newFakeRepo())
	_, err := svc.GetByID(context.Background(), "nonexistent")
	ae, ok := apperrors.As(err)
	if !ok || ae.Code != "tenant_not_found" {
		t.Errorf("expected tenant_not_found, got %v", err)
	}
}
