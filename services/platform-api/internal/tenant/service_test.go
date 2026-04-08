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
	mu   sync.Mutex
	byID map[string]*Tenant
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{
		byID: map[string]*Tenant{},
	}
}

func (f *fakeRepo) seed(t *Tenant) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if t.ID == "" {
		t.ID = "id-" + t.Name
	}
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

func (f *fakeRepo) GetByOwnerUserID(ctx context.Context, uid string) (*Tenant, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, t := range f.byID {
		if t.OwnerUserID == uid {
			return t, nil
		}
	}
	return nil, apperrors.NotFound("tenant_not_found", "no")
}

func (f *fakeRepo) ListByIDs(ctx context.Context, ids []string) ([]Tenant, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]Tenant, 0, len(ids))
	for _, id := range ids {
		if t, ok := f.byID[id]; ok {
			out = append(out, *t)
		}
	}
	return out, nil
}

func (f *fakeRepo) UpdateEditable(ctx context.Context, id string, patch map[string]any) (*Tenant, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	t, ok := f.byID[id]
	if !ok {
		return nil, apperrors.NotFound("tenant_not_found", "no")
	}
	if v, ok := patch["name"].(string); ok {
		t.Name = v
	}
	return t, nil
}

// Phase Q: slug-related tests (IsSlugAvailable, SlugExists) moved
// to the store package along with the slug itself. See
// internal/store for equivalent coverage.

// ─────────────────────────────────────────────────────────────────────────

func TestService_GetByID_RejectsEmpty(t *testing.T) {
	svc := NewService(newFakeRepo(), nil)
	_, err := svc.GetByID(context.Background(), "")
	ae, ok := apperrors.As(err)
	if !ok || ae.Code != "invalid_tenant_id" {
		t.Errorf("expected invalid_tenant_id, got %v", err)
	}
}

func TestService_GetByID_PropagatesNotFound(t *testing.T) {
	svc := NewService(newFakeRepo(), nil)
	_, err := svc.GetByID(context.Background(), "nonexistent")
	ae, ok := apperrors.As(err)
	if !ok || ae.Code != "tenant_not_found" {
		t.Errorf("expected tenant_not_found, got %v", err)
	}
}

func TestService_GetByOwnerUserID_RejectsEmpty(t *testing.T) {
	svc := NewService(newFakeRepo(), nil)
	_, err := svc.GetByOwnerUserID(context.Background(), "  ")
	ae, ok := apperrors.As(err)
	if !ok || ae.Code != "invalid_uid" {
		t.Errorf("expected invalid_uid, got %v", err)
	}
}

func TestService_GetByOwnerUserID_FindsSeededTenant(t *testing.T) {
	repo := newFakeRepo()
	repo.seed(&Tenant{ID: "t-acme", Name: "Acme", OwnerUserID: "uid-42"})
	svc := NewService(repo, nil)

	got, err := svc.GetByOwnerUserID(context.Background(), "uid-42")
	if err != nil {
		t.Fatalf("error = %v", err)
	}
	if got.ID != "t-acme" {
		t.Errorf("ID = %q, want t-acme", got.ID)
	}
}

func ptr[T any](v T) *T { return &v }

func TestService_Update_RenamesTenant(t *testing.T) {
	repo := newFakeRepo()
	repo.seed(&Tenant{ID: "t1", Name: "Acme"})
	svc := NewService(repo, nil)

	got, err := svc.Update(context.Background(), "t1", UpdateInput{Name: ptr("Acme Corp")})
	if err != nil {
		t.Fatalf("error = %v", err)
	}
	if got.Name != "Acme Corp" {
		t.Errorf("Name = %q, want %q", got.Name, "Acme Corp")
	}
}

func TestService_Update_TrimsName(t *testing.T) {
	repo := newFakeRepo()
	repo.seed(&Tenant{ID: "t1", Name: "Acme"})
	svc := NewService(repo, nil)

	got, err := svc.Update(context.Background(), "t1", UpdateInput{Name: ptr("  Trimmed  ")})
	if err != nil {
		t.Fatalf("error = %v", err)
	}
	if got.Name != "Trimmed" {
		t.Errorf("Name = %q, want %q", got.Name, "Trimmed")
	}
}

func TestService_Update_RejectsEmptyName(t *testing.T) {
	repo := newFakeRepo()
	repo.seed(&Tenant{ID: "t1", Name: "Acme"})
	svc := NewService(repo, nil)

	_, err := svc.Update(context.Background(), "t1", UpdateInput{Name: ptr("   ")})
	ae, ok := apperrors.As(err)
	if !ok || ae.Code != "invalid_name" {
		t.Errorf("expected invalid_name, got %v", err)
	}
}

func TestService_Update_RejectsOversizedName(t *testing.T) {
	repo := newFakeRepo()
	repo.seed(&Tenant{ID: "t1", Name: "Acme"})
	svc := NewService(repo, nil)

	long := make([]byte, 201)
	for i := range long {
		long[i] = 'a'
	}
	_, err := svc.Update(context.Background(), "t1", UpdateInput{Name: ptr(string(long))})
	ae, ok := apperrors.As(err)
	if !ok || ae.Code != "invalid_name" {
		t.Errorf("expected invalid_name, got %v", err)
	}
}

func TestService_Update_RejectsEmptyPatch(t *testing.T) {
	repo := newFakeRepo()
	repo.seed(&Tenant{ID: "t1", Name: "Acme"})
	svc := NewService(repo, nil)

	_, err := svc.Update(context.Background(), "t1", UpdateInput{})
	ae, ok := apperrors.As(err)
	if !ok || ae.Code != "empty_update" {
		t.Errorf("expected empty_update, got %v", err)
	}
}

func TestService_Update_RejectsEmptyID(t *testing.T) {
	svc := NewService(newFakeRepo(), nil)
	_, err := svc.Update(context.Background(), "  ", UpdateInput{Name: ptr("Acme")})
	ae, ok := apperrors.As(err)
	if !ok || ae.Code != "invalid_tenant_id" {
		t.Errorf("expected invalid_tenant_id, got %v", err)
	}
}

func TestService_GetByOwnerUserID_NotFound(t *testing.T) {
	svc := NewService(newFakeRepo(), nil)
	_, err := svc.GetByOwnerUserID(context.Background(), "nobody")
	ae, ok := apperrors.As(err)
	if !ok || ae.Code != "tenant_not_found" {
		t.Errorf("expected tenant_not_found, got %v", err)
	}
}
