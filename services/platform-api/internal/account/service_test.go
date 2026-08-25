package account

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"testing"

	"gorm.io/gorm"

	"github.com/mark8ly/platform-api/internal/authz"
	"github.com/mark8ly/platform-api/internal/tenant"
	apperrors "github.com/mark8ly/platform-api/pkg/errors"
)

// testLogger returns a slog.Logger that discards output so tests don't
// spam stdout while still exercising the WARN logging code paths.
func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// fakeTenantRepo is an in-memory double for TenantRepo. It records deletes
// so tests can assert whether the tenant row was torn down, without a
// database.
type fakeTenantRepo struct {
	stores      map[string][]string
	deleted     map[string]bool
	listErr     error
	teardownErr error
}

func (f *fakeTenantRepo) ListStoreIDs(ctx context.Context, tx *gorm.DB, tenantID string) ([]string, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	return f.stores[tenantID], nil
}

func (f *fakeTenantRepo) DeleteInTx(ctx context.Context, tx *gorm.DB, tenantID string) error {
	if f.teardownErr != nil {
		return f.teardownErr
	}
	if f.deleted == nil {
		f.deleted = map[string]bool{}
	}
	f.deleted[tenantID] = true
	return nil
}

// SnapshotForTeardown is not exercised by this file's tests (they cover
// DeleteAccount, not PurgeTenant); it exists only so fakeTenantRepo keeps
// satisfying TenantRepo. See purge_test.go for the tests that use it.
func (f *fakeTenantRepo) SnapshotForTeardown(ctx context.Context, tx *gorm.DB, tenantID string) (*tenant.TeardownSnapshot, error) {
	return nil, nil
}

// fakeGIP is an in-memory double for gipDeleter.
type fakeGIP struct {
	deleted map[string]bool
	err     error
}

func (f *fakeGIP) DeleteAccount(ctx context.Context, uid string) error {
	if f.err != nil {
		return f.err
	}
	if f.deleted == nil {
		f.deleted = map[string]bool{}
	}
	f.deleted[uid] = true
	return nil
}

// fakeOutbox is an in-memory double satisfying the outboxEnqueuer func
// type via its Enqueue method value (fakeOutbox{}.Enqueue matches the
// func(tx *gorm.DB, kind string, payload any) error shape).
type fakeOutbox struct {
	kinds []string
}

func (f *fakeOutbox) Enqueue(tx *gorm.DB, kind string, payload any) error {
	f.kinds = append(f.kinds, kind)
	return nil
}

func (f *fakeOutbox) has(kind string) bool {
	for _, k := range f.kinds {
		if k == kind {
			return true
		}
	}
	return false
}

func TestDeleteAccount_Owner_TearsDownTenant(t *testing.T) {
	ctx := context.Background()
	fga := authz.NewFake()
	if err := fga.WriteOwnership(ctx, "owner-1", "t1"); err != nil {
		t.Fatalf("seed ownership: %v", err)
	}
	if err := fga.WriteStoreParent(ctx, "s1", "t1"); err != nil {
		t.Fatalf("seed store parent: %v", err)
	}
	gip := &fakeGIP{}
	repo := &fakeTenantRepo{stores: map[string][]string{"t1": {"s1"}}}
	ob := &fakeOutbox{}

	svc := NewService(nil, repo, fga, gip, ob.Enqueue, testLogger())

	if err := svc.DeleteAccount(ctx, "t1", "owner-1"); err != nil {
		t.Fatalf("DeleteAccount() error = %v", err)
	}
	if !repo.deleted["t1"] {
		t.Error("tenant not deleted")
	}
	if !gip.deleted["owner-1"] {
		t.Error("gip user not deleted")
	}
	if !ob.has("tenant.deleted") {
		t.Error("tenant.deleted not enqueued")
	}
	if fga.HasOwnership("owner-1", "t1") {
		t.Error("owner tuple should be deleted")
	}
	if fga.HasStoreParent("s1", "t1") {
		t.Error("store parent tuple should be deleted")
	}
}

func TestDeleteAccount_Staff_RemovesTupleAndGIPOnly(t *testing.T) {
	ctx := context.Background()
	fga := authz.NewFake()
	if err := fga.WriteRole(ctx, "staff-1", authz.RoleStaff, "t1"); err != nil {
		t.Fatalf("seed role: %v", err)
	}
	gip := &fakeGIP{}
	repo := &fakeTenantRepo{stores: map[string][]string{"t1": {"s1"}}}
	ob := &fakeOutbox{}

	svc := NewService(nil, repo, fga, gip, ob.Enqueue, testLogger())

	if err := svc.DeleteAccount(ctx, "t1", "staff-1"); err != nil {
		t.Fatalf("DeleteAccount() error = %v", err)
	}
	if repo.deleted["t1"] {
		t.Error("tenant should survive staff account deletion")
	}
	if !gip.deleted["staff-1"] {
		t.Error("gip user not deleted")
	}
	if fga.HasRole("staff-1", authz.RoleStaff, "t1") {
		t.Error("staff role tuple should be deleted")
	}
	if ob.has("tenant.deleted") {
		t.Error("tenant.deleted should not be enqueued for the staff path")
	}
}

func TestDeleteAccount_UnknownRole_Forbidden(t *testing.T) {
	ctx := context.Background()
	fga := authz.NewFake()
	repo := &fakeTenantRepo{}
	gip := &fakeGIP{}
	ob := &fakeOutbox{}

	svc := NewService(nil, repo, fga, gip, ob.Enqueue, testLogger())

	err := svc.DeleteAccount(ctx, "t1", "ghost")
	if err == nil {
		t.Fatal("expected error for actor with no role on the tenant")
	}
	ae, ok := apperrors.As(err)
	if !ok {
		t.Fatalf("expected *apperrors.AppError, got %T: %v", err, err)
	}
	if ae.Status != http.StatusForbidden {
		t.Errorf("Status = %d, want %d", ae.Status, http.StatusForbidden)
	}
	if repo.deleted["t1"] {
		t.Error("tenant should not be touched for an unknown role")
	}
}

// TestDeleteAccount_Owner_PostCommitFailuresDoNotFailCall verifies
// CONTROLLER RESOLUTION #4: post-commit FGA/GIP cleanup is best-effort —
// a GIP hiccup must not unwind or fail the already-committed DB teardown,
// because the enqueued tenant.deleted outbox event is the durable retry
// channel and every primitive here is idempotent.
func TestDeleteAccount_Owner_PostCommitFailuresDoNotFailCall(t *testing.T) {
	ctx := context.Background()
	fga := authz.NewFake()
	if err := fga.WriteOwnership(ctx, "owner-1", "t1"); err != nil {
		t.Fatalf("seed ownership: %v", err)
	}
	gip := &fakeGIP{err: errors.New("gip unavailable")}
	repo := &fakeTenantRepo{stores: map[string][]string{"t1": {"s1"}}}
	ob := &fakeOutbox{}

	svc := NewService(nil, repo, fga, gip, ob.Enqueue, testLogger())

	if err := svc.DeleteAccount(ctx, "t1", "owner-1"); err != nil {
		t.Fatalf("DeleteAccount() error = %v, want nil (post-commit cleanup is best-effort)", err)
	}
	if !repo.deleted["t1"] {
		t.Error("tenant should still be deleted despite the GIP failure")
	}
	if !ob.has("tenant.deleted") {
		t.Error("tenant.deleted should still be enqueued despite the GIP failure")
	}
}

// TestDeleteAccount_Owner_TeardownFailure_FailsCall verifies the DB
// teardown failing DOES fail the call (unlike the post-commit steps).
func TestDeleteAccount_Owner_TeardownFailure_FailsCall(t *testing.T) {
	ctx := context.Background()
	fga := authz.NewFake()
	if err := fga.WriteOwnership(ctx, "owner-1", "t1"); err != nil {
		t.Fatalf("seed ownership: %v", err)
	}
	gip := &fakeGIP{}
	repo := &fakeTenantRepo{
		stores:      map[string][]string{"t1": {"s1"}},
		teardownErr: errors.New("db unavailable"),
	}
	ob := &fakeOutbox{}

	svc := NewService(nil, repo, fga, gip, ob.Enqueue, testLogger())

	if err := svc.DeleteAccount(ctx, "t1", "owner-1"); err == nil {
		t.Fatal("expected error when DB teardown fails")
	}
	if gip.deleted["owner-1"] {
		t.Error("gip deletion should not run when DB teardown fails")
	}
	if ob.has("tenant.deleted") {
		t.Error("tenant.deleted should not be enqueued when DB teardown fails")
	}
}
