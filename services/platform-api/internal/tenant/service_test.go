package tenant

import (
	"context"
	"strings"
	"sync"
	"testing"

	"gorm.io/gorm"

	apperrors "github.com/mark8ly/platform-api/pkg/errors"
)

// fakeRepo is an in-memory Repository for unit tests.
type fakeRepo struct {
	mu       sync.Mutex
	byID     map[string]*Tenant
	storeIDs map[string][]string
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{
		byID:     map[string]*Tenant{},
		storeIDs: map[string][]string{},
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

func (f *fakeRepo) OwnerEmailExists(ctx context.Context, email string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	normalized := strings.ToLower(strings.TrimSpace(email))
	for _, t := range f.byID {
		if strings.ToLower(strings.TrimSpace(t.OwnerEmail)) == normalized {
			return true, nil
		}
	}
	return false, nil
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

// storeIDs and deleted support the account-deletion teardown path
// (ListStoreIDs / DeleteInTx). Kept minimal: no real GORM tx semantics,
// just enough in-memory state for Task 4's service-level tests.
func (f *fakeRepo) ListStoreIDs(ctx context.Context, tx *gorm.DB, tenantID string) ([]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.storeIDs[tenantID], nil
}

func (f *fakeRepo) DeleteInTx(ctx context.Context, tx *gorm.DB, tenantID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.byID[tenantID]; !ok {
		return apperrors.NotFound("tenant_not_found", "no")
	}
	delete(f.byID, tenantID)
	delete(f.storeIDs, tenantID)
	return nil
}

// ListDirectory and GetWithStores are not exercised by these service-level
// unit tests (covered by directory_integration_test.go against a real DB);
// these stubs exist only to satisfy the Repository interface.
func (f *fakeRepo) ListDirectory(ctx context.Context, filter DirectoryFilter) (DirectoryResult, error) {
	return DirectoryResult{Tenants: []Tenant{}}, nil
}

func (f *fakeRepo) GetWithStores(ctx context.Context, id string) (*TenantWithStores, error) {
	t, err := f.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	return &TenantWithStores{Tenant: *t, Stores: []StoreSummary{}}, nil
}

// GetByOwnerEmail is likewise not exercised by these service-level unit
// tests (covered by directory_integration_test.go against a real DB);
// this stub exists only to satisfy the Repository interface.
func (f *fakeRepo) GetByOwnerEmail(ctx context.Context, email string) (*Tenant, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, t := range f.byID {
		if strings.EqualFold(strings.TrimSpace(t.OwnerEmail), strings.TrimSpace(email)) {
			cp := *t
			return &cp, nil
		}
	}
	return nil, apperrors.NotFound("tenant_not_found", "no tenant owns that email")
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

// Suspend/Unsuspend are exercised against a real DB in
// suspend_integration_test.go (the cascade needs real transactions and the
// stores table); these stubs exist only to satisfy the Repository interface
// for the unit tests in this file.
func (f *fakeRepo) Suspend(ctx context.Context, tenantID string) (*SuspendResult, error) {
	return nil, apperrors.Internal("not_implemented", "fakeRepo.Suspend is not implemented")
}

func (f *fakeRepo) Unsuspend(ctx context.Context, tenantID string) (*SuspendResult, error) {
	return nil, apperrors.Internal("not_implemented", "fakeRepo.Unsuspend is not implemented")
}

// SnapshotForTeardown is not exercised by these service-level unit tests
// (covered by integration tests); this stub exists only to satisfy the
// Repository interface.
func (f *fakeRepo) SnapshotForTeardown(ctx context.Context, tx *gorm.DB, tenantID string) (*TeardownSnapshot, error) {
	return nil, apperrors.Internal("not_implemented", "fakeRepo.SnapshotForTeardown is not implemented")
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
