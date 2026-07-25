package notification

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/pkg/apperrors"
)

// fakeRepository is a minimal Repository implementation for unit-testing
// Service methods without a database. Only GetPreferences is exercised by
// these tests; every other method panics if called, since these tests never
// invoke them.
type fakeRepository struct {
	getPreferencesFn func(ctx context.Context, db *gorm.DB, storeID uuid.UUID) (*NotificationPreferences, error)
}

func (f *fakeRepository) ListByStore(ctx context.Context, db *gorm.DB, filter ListFilter) (ListResult, error) {
	panic("not implemented")
}

func (f *fakeRepository) GetUnreadCount(ctx context.Context, db *gorm.DB, storeID uuid.UUID) (int64, error) {
	panic("not implemented")
}

func (f *fakeRepository) MarkRead(ctx context.Context, db *gorm.DB, storeID, id uuid.UUID) error {
	panic("not implemented")
}

func (f *fakeRepository) MarkAllRead(ctx context.Context, db *gorm.DB, storeID uuid.UUID) error {
	panic("not implemented")
}

func (f *fakeRepository) ListByCustomer(ctx context.Context, db *gorm.DB, storeID uuid.UUID, recipientUserID string, page, perPage int) (ListResult, error) {
	panic("not implemented")
}

func (f *fakeRepository) GetUnreadCountByCustomer(ctx context.Context, db *gorm.DB, storeID uuid.UUID, recipientUserID string) (int64, error) {
	panic("not implemented")
}

func (f *fakeRepository) MarkReadByCustomer(ctx context.Context, db *gorm.DB, storeID uuid.UUID, recipientUserID string, id uuid.UUID) error {
	panic("not implemented")
}

func (f *fakeRepository) MarkAllReadByCustomer(ctx context.Context, db *gorm.DB, storeID uuid.UUID, recipientUserID string) error {
	panic("not implemented")
}

func (f *fakeRepository) Create(ctx context.Context, db *gorm.DB, n *Notification) error {
	panic("not implemented")
}

func (f *fakeRepository) GetPreferences(ctx context.Context, db *gorm.DB, storeID uuid.UUID) (*NotificationPreferences, error) {
	return f.getPreferencesFn(ctx, db, storeID)
}

func (f *fakeRepository) UpsertPreferences(ctx context.Context, db *gorm.DB, storeID, tenantID uuid.UUID, prefs json.RawMessage) (*NotificationPreferences, error) {
	panic("not implemented")
}

// TestService_GetPreferences_NotFoundFallsBackToDefaults reproduces the
// production bug: a store with no notification_preferences row must get a
// 200 with UI defaults, not a propagated 404. A 404 on this endpoint is
// misread by the mobile-admin API client as "tenant invalid" and bounces
// the user to the dashboard.
func TestService_GetPreferences_NotFoundFallsBackToDefaults(t *testing.T) {
	storeID := uuid.New()
	repo := &fakeRepository{
		getPreferencesFn: func(ctx context.Context, db *gorm.DB, sID uuid.UUID) (*NotificationPreferences, error) {
			return nil, apperrors.NotFound("notification preferences")
		},
	}
	svc := NewService(ServiceConfig{Repo: repo})

	prefs, err := svc.GetPreferences(context.Background(), storeID)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if prefs == nil {
		t.Fatal("expected non-nil default preferences, got nil")
	}
	if prefs.StoreID != storeID {
		t.Errorf("StoreID = %v, want %v", prefs.StoreID, storeID)
	}

	var parsed map[string]bool
	if err := json.Unmarshal(prefs.Preferences, &parsed); err != nil {
		t.Fatalf("failed to unmarshal preferences: %v", err)
	}

	want := map[string]bool{
		"new_order":        true,
		"low_stock":        true,
		"return_requested": true,
		"payment_received": false,
		"review_submitted": false,
	}
	for k, v := range want {
		if parsed[k] != v {
			t.Errorf("preferences[%q] = %v, want %v", k, parsed[k], v)
		}
	}
}

// TestService_GetPreferences_ExistingRowPassedThrough asserts a real
// preferences row from the repo is returned unchanged.
func TestService_GetPreferences_ExistingRowPassedThrough(t *testing.T) {
	storeID := uuid.New()
	existing := &NotificationPreferences{
		StoreID:     storeID,
		Preferences: json.RawMessage(`{"new_order":false}`),
	}
	repo := &fakeRepository{
		getPreferencesFn: func(ctx context.Context, db *gorm.DB, sID uuid.UUID) (*NotificationPreferences, error) {
			return existing, nil
		},
	}
	svc := NewService(ServiceConfig{Repo: repo})

	prefs, err := svc.GetPreferences(context.Background(), storeID)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if prefs != existing {
		t.Errorf("expected the exact repo-returned pointer to be passed through, got a different value: %+v", prefs)
	}
}

// TestService_GetPreferences_OtherErrorPropagated asserts non-not-found
// errors (e.g. a DB outage) are not swallowed by the default fallback.
func TestService_GetPreferences_OtherErrorPropagated(t *testing.T) {
	storeID := uuid.New()
	dbErr := errors.New("db down")
	repo := &fakeRepository{
		getPreferencesFn: func(ctx context.Context, db *gorm.DB, sID uuid.UUID) (*NotificationPreferences, error) {
			return nil, dbErr
		},
	}
	svc := NewService(ServiceConfig{Repo: repo})

	prefs, err := svc.GetPreferences(context.Background(), storeID)
	if prefs != nil {
		t.Errorf("expected nil preferences on error, got %+v", prefs)
	}
	if !errors.Is(err, dbErr) {
		t.Errorf("expected the original db error to propagate, got %v", err)
	}
}
