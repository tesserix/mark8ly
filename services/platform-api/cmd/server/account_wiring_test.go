package main

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"gorm.io/gorm"

	"github.com/mark8ly/platform-api/internal/tenant"
)

// stubTeardownRepo is a minimal account.TenantRepo. It needs no database:
// account.NewService with a nil *gorm.DB runs the teardown steps without a
// transaction wrapper, which is exactly enough to reach the POST-COMMIT
// cleanup this test is about.
type stubTeardownRepo struct{ snap *tenant.TeardownSnapshot }

func (r *stubTeardownRepo) ListStoreIDs(context.Context, *gorm.DB, string) ([]string, error) {
	return nil, nil
}

func (r *stubTeardownRepo) DeleteInTx(context.Context, *gorm.DB, string) error { return nil }

func (r *stubTeardownRepo) SnapshotForTeardown(_ context.Context, _ *gorm.DB, tenantID string) (*tenant.TeardownSnapshot, error) {
	return r.snap, nil
}

// TestNewAccountService_OperatorPurgeSurvivesAbsentGIPAndFGA is the
// regression test for the typed-nil hazard newAccountService exists to
// close.
//
// platform-api deployed without GIP_PROJECT_ID/GIP_TENANT_ID/
// GIP_WEB_API_KEY leaves gipAdmin a nil *gipadmin.AdminClient. Handed
// straight to account.NewService it becomes a NON-NIL interface holding a
// nil pointer: cleanupAfterTeardown's `if s.gip != nil` passes,
// DeleteAccount runs on a nil receiver, and it panics dereferencing the
// client's config — AFTER the teardown transaction has committed. The
// operator then sees 503 for a tenant that is already destroyed.
//
// So this test drives the WHOLE PurgeTenant path (which ends in
// cleanupAfterTeardown) with both clients absent, and requires it to
// return normally. MUTATION: pass gipAdmin straight through in
// newAccountService instead of via the guarded interface variable and this
// test panics.
func TestNewAccountService_OperatorPurgeSurvivesAbsentGIPAndFGA(t *testing.T) {
	repo := &stubTeardownRepo{snap: &tenant.TeardownSnapshot{
		TenantID:    "11111111-1111-1111-1111-111111111111",
		Name:        "Tenant A",
		OwnerUserID: "uid_owner",
		Stores:      []tenant.StoreRef{{ID: "22222222-2222-2222-2222-222222222222", Slug: "store-a"}},
	}}

	enqueued := 0
	enqueue := func(*gorm.DB, string, any, time.Duration) error { enqueued++; return nil }

	// nil conn, nil fga, nil gipAdmin — precisely the unconfigured
	// deployment the startup warning in main.go describes.
	svc := newAccountService(nil, repo, nil, nil, enqueue,
		slog.New(slog.NewTextHandler(io.Discard, nil)))

	res, err := svc.PurgeTenant(context.Background(), repo.snap.TenantID, []string{"store-a"})
	if err != nil {
		t.Fatalf("PurgeTenant with no GIP and no FGA must succeed, got: %v", err)
	}
	if res.TenantName != "Tenant A" {
		t.Fatalf("tenant name = %q, want %q", res.TenantName, "Tenant A")
	}
	if len(res.StoreIDs) != 1 || res.StoreIDs[0] != "22222222-2222-2222-2222-222222222222" {
		t.Fatalf("store ids = %v, want the snapshot's single store", res.StoreIDs)
	}
	if enqueued != 1 {
		t.Fatalf("tenant.deleted outbox enqueues = %d, want 1", enqueued)
	}
}
