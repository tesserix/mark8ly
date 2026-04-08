package store

import (
	"context"
	"encoding/json"
	"testing"

	apperrors "github.com/mark8ly/platform-api/pkg/errors"
	"gorm.io/gorm"
)

type fakeStoreRepo struct {
	updatePatch map[string]any
	updateResp  *Store
}

func (f *fakeStoreRepo) CreateInTx(ctx context.Context, tx *gorm.DB, s *Store) error {
	return nil
}

func (f *fakeStoreRepo) Create(ctx context.Context, s *Store) error { return nil }

func (f *fakeStoreRepo) GetByID(ctx context.Context, id string) (*Store, error) {
	return &Store{ID: id}, nil
}

func (f *fakeStoreRepo) GetBySlug(ctx context.Context, slug string) (*Store, error) {
	return &Store{Slug: slug}, nil
}

func (f *fakeStoreRepo) ListByTenant(ctx context.Context, tenantID string) ([]Store, error) {
	return nil, nil
}

func (f *fakeStoreRepo) SlugExists(ctx context.Context, slug string) (bool, error) {
	return false, nil
}

func (f *fakeStoreRepo) UpdateEditable(ctx context.Context, id string, patch map[string]any) (*Store, error) {
	f.updatePatch = patch
	if f.updateResp != nil {
		return f.updateResp, nil
	}
	return &Store{ID: id}, nil
}

func TestServiceUpdateRejectsInvalidStorefrontThemeJSON(t *testing.T) {
	repo := &fakeStoreRepo{}
	svc := NewService(repo, nil)

	_, err := svc.Update(context.Background(), "store-1", UpdateInput{
		StorefrontTheme: json.RawMessage(`{not-json}`),
	})
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	ae, ok := apperrors.As(err)
	if !ok {
		t.Fatalf("expected app error, got %T", err)
	}
	if ae.Code != "invalid_storefront_theme" {
		t.Fatalf("expected invalid_storefront_theme, got %s", ae.Code)
	}
}

func TestServiceUpdateAppliesStorefrontThemePatch(t *testing.T) {
	repo := &fakeStoreRepo{}
	svc := NewService(repo, nil)
	theme := json.RawMessage(`{"layout":"bold-promo","preset":"warm"}`)

	_, err := svc.Update(context.Background(), "store-2", UpdateInput{
		StorefrontTheme: theme,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	raw, ok := repo.updatePatch["storefront_theme"]
	if !ok {
		t.Fatalf("expected storefront_theme patch key")
	}
	got, ok := raw.(json.RawMessage)
	if !ok {
		t.Fatalf("expected json.RawMessage, got %T", raw)
	}
	if string(got) != string(theme) {
		t.Fatalf("unexpected theme payload: got %s want %s", string(got), string(theme))
	}
}
