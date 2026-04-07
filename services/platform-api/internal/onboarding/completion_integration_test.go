//go:build integration

package onboarding

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/mark8ly/platform-api/internal/authz"
	"github.com/mark8ly/platform-api/internal/notification"
	"github.com/mark8ly/platform-api/internal/outbox"
	"github.com/mark8ly/platform-api/internal/tenant"
	"github.com/mark8ly/platform-api/pkg/testdb"
)

// TestIntegration_Complete_TenantAndOutboxCommitAtomically is the
// regression test for auth-bug #2 (FGA tuple race).
//
// The bug was: tenant row was committed BEFORE the FGA tuple was written,
// so a user redirected to admin saw "tenant not found" because admin's
// FGA check ran before the tuple existed.
//
// The fix: tenant + outbox row commit in the SAME tx. Drainer ships FGA.
// auth-bff side does check-with-retry to close the race.
//
// This test verifies:
//   1. After Complete returns, the tenant row exists
//   2. After Complete returns, the session is marked completed
//   3. After Complete returns, the outbox row exists with status='pending'
//   4. After the drainer ticks, the outbox row is 'completed'
//   5. After the drainer ticks, the FakeClient has the membership tuple
func TestIntegration_Complete_TenantAndOutboxCommitAtomically(t *testing.T) {
	db := testdb.NewDB(t,
		"outbox_events",
		"verification_tokens",
		"onboarding_sessions",
		"tenants",
	)

	// ─── Set up the service stack against the real DB ─────────────────
	tenantRepo := tenant.NewRepository(db)
	onboardingRepo := NewRepository(db)
	svc := NewService(Config{
		DB:         db,
		Repo:       onboardingRepo,
		TenantRepo: tenantRepo,
		Sender:     notification.NoopSender{},
		EmailFrom:  "noreply@test.local",
		SupportEmail: "help@test.local",
	})

	ctx := context.Background()

	// ─── Set up a session in the verified state ────────────────────────
	now := time.Now()
	sess := &Session{
		Email:           "founder@completion-test.local",
		Draft:           json.RawMessage(`{}`),
		Status:          StatusInProgress,
		EmailVerifiedAt: &now,
	}
	if err := onboardingRepo.Create(ctx, sess); err != nil {
		t.Fatalf("seed session: %v", err)
	}

	// ─── Run the bug-fix transaction ───────────────────────────────────
	res, err := svc.Complete(ctx, CompleteRequest{
		SessionID:    sess.ID,
		BusinessName: "Atomicity Test Co",
		Slug:         "atomicity-test",
		OwnerUserID:  "gip-uid-completion",
		OwnerEmail:   "founder@completion-test.local",
		CountryCode:  "US",
		CurrencyCode: "USD",
		Timezone:     "America/New_York",
	})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if res.TenantID == "" {
		t.Fatal("Complete returned empty TenantID")
	}

	// ─── Step 1: tenant row exists ─────────────────────────────────────
	got, err := tenantRepo.GetByID(ctx, res.TenantID)
	if err != nil {
		t.Fatalf("tenant lookup: %v", err)
	}
	if got.Slug != "atomicity-test" {
		t.Errorf("Slug = %q, want atomicity-test", got.Slug)
	}

	// ─── Step 2: session is completed ──────────────────────────────────
	updated, err := onboardingRepo.GetByID(ctx, sess.ID)
	if err != nil {
		t.Fatalf("session lookup: %v", err)
	}
	if updated.Status != StatusCompleted {
		t.Errorf("session.Status = %q, want %q", updated.Status, StatusCompleted)
	}
	if updated.TenantID == nil || *updated.TenantID != res.TenantID {
		t.Errorf("session.TenantID = %v, want %q", updated.TenantID, res.TenantID)
	}

	// ─── Step 3: outbox row exists, pending ────────────────────────────
	var ev outbox.Event
	if err := db.Where("kind = ?", FGAOutboxKind).First(&ev).Error; err != nil {
		t.Fatalf("outbox row missing: %v", err)
	}
	if ev.Status != outbox.StatusPending {
		t.Errorf("outbox row Status = %q, want pending", ev.Status)
	}

	// Verify payload shape
	var payload fgaWritePayload
	if err := json.Unmarshal(ev.Payload, &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if payload.UserID != "gip-uid-completion" {
		t.Errorf("payload.UserID = %q, want gip-uid-completion", payload.UserID)
	}
	if payload.TenantID != res.TenantID {
		t.Errorf("payload.TenantID = %q, want %q", payload.TenantID, res.TenantID)
	}

	// ─── Step 4 + 5: drainer ticks, outbox completes, FGA has tuple ────
	fake := authz.NewFake()
	d := outbox.NewDrainer(db, slog.New(slog.NewTextHandler(io.Discard, nil)), outbox.Config{})
	d.Register(FGAOutboxKind, NewFGAOutboxHandler(fake))

	if err := d.Tick(ctx); err != nil {
		t.Fatalf("drainer tick: %v", err)
	}

	if err := db.Where("kind = ?", FGAOutboxKind).First(&ev).Error; err != nil {
		t.Fatal(err)
	}
	if ev.Status != outbox.StatusCompleted {
		t.Errorf("after drain: outbox Status = %q, want completed", ev.Status)
	}
	if !fake.HasMembership("gip-uid-completion", res.TenantID) {
		t.Error("FGA membership tuple was NOT written after drainer tick")
	}
	if !fake.HasOwnership("gip-uid-completion", res.TenantID) {
		t.Error("FGA ownership tuple was NOT written after drainer tick")
	}
}

// TestIntegration_Complete_RejectsUnverifiedSession ensures completion
// cannot happen without a verified email — this is a security gate.
func TestIntegration_Complete_RejectsUnverifiedSession(t *testing.T) {
	db := testdb.NewDB(t,
		"outbox_events",
		"verification_tokens",
		"onboarding_sessions",
		"tenants",
	)

	tenantRepo := tenant.NewRepository(db)
	onboardingRepo := NewRepository(db)
	svc := NewService(Config{
		DB:         db,
		Repo:       onboardingRepo,
		TenantRepo: tenantRepo,
		Sender:     notification.NoopSender{},
		EmailFrom:  "noreply@test.local",
	})

	ctx := context.Background()
	sess := &Session{
		Email:  "unverified@test.local",
		Draft:  json.RawMessage(`{}`),
		Status: StatusInProgress,
		// EmailVerifiedAt intentionally nil
	}
	if err := onboardingRepo.Create(ctx, sess); err != nil {
		t.Fatal(err)
	}

	_, err := svc.Complete(ctx, CompleteRequest{
		SessionID:    sess.ID,
		BusinessName: "X",
		Slug:         "rejected-slug",
		OwnerUserID:  "uid",
		OwnerEmail:   "unverified@test.local",
		CountryCode:  "US",
		CurrencyCode: "USD",
		Timezone:     "America/New_York",
	})
	if err == nil {
		t.Fatal("Complete should reject session with no email_verified_at")
	}

	// Verify NOTHING was written (no tenant, no outbox row).
	var tenantCount int64
	db.Model(&tenant.Tenant{}).Where("slug = ?", "rejected-slug").Count(&tenantCount)
	if tenantCount != 0 {
		t.Errorf("rejected completion left a tenant row behind")
	}

	var outboxCount int64
	db.Model(&outbox.Event{}).Count(&outboxCount)
	if outboxCount != 0 {
		t.Errorf("rejected completion left %d outbox row(s)", outboxCount)
	}
}

// TestIntegration_Complete_DuplicateSlugRollsBackEverything is the
// crucial atomicity test: if the tenant insert fails (slug taken), the
// session must NOT be marked completed and the outbox row must NOT exist.
// Otherwise we'd have orphaned outbox events for nonexistent tenants.
func TestIntegration_Complete_DuplicateSlugRollsBackEverything(t *testing.T) {
	db := testdb.NewDB(t,
		"outbox_events",
		"verification_tokens",
		"onboarding_sessions",
		"tenants",
	)

	tenantRepo := tenant.NewRepository(db)
	onboardingRepo := NewRepository(db)
	svc := NewService(Config{
		DB:         db,
		Repo:       onboardingRepo,
		TenantRepo: tenantRepo,
		Sender:     notification.NoopSender{},
		EmailFrom:  "noreply@test.local",
	})

	ctx := context.Background()

	// Seed a tenant whose slug we will collide with.
	if err := db.Create(&tenant.Tenant{
		Slug: "taken-slug", Name: "Existing", OwnerUserID: "u1",
		OwnerEmail: "a@test.local", CountryCode: "US", CurrencyCode: "USD",
		Timezone: "America/New_York", Status: tenant.StatusActive,
	}).Error; err != nil {
		t.Fatal(err)
	}

	now := time.Now()
	sess := &Session{
		Email:           "collide@test.local",
		Draft:           json.RawMessage(`{}`),
		Status:          StatusInProgress,
		EmailVerifiedAt: &now,
	}
	if err := onboardingRepo.Create(ctx, sess); err != nil {
		t.Fatal(err)
	}

	_, err := svc.Complete(ctx, CompleteRequest{
		SessionID:    sess.ID,
		BusinessName: "Collider",
		Slug:         "taken-slug",
		OwnerUserID:  "uid-collider",
		OwnerEmail:   "collide@test.local",
		CountryCode:  "US",
		CurrencyCode: "USD",
		Timezone:     "America/New_York",
	})
	if err == nil {
		t.Fatal("Complete should fail on duplicate slug")
	}

	// Session should still be in_progress (rollback worked).
	updated, err := onboardingRepo.GetByID(ctx, sess.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Status == StatusCompleted {
		t.Errorf("session was marked completed despite tenant insert failing")
	}
	if updated.TenantID != nil {
		t.Errorf("session.TenantID was set despite tenant insert failing")
	}

	// Outbox should be empty.
	var outboxCount int64
	db.Model(&outbox.Event{}).Count(&outboxCount)
	if outboxCount != 0 {
		t.Errorf("outbox has %d row(s) despite rollback", outboxCount)
	}
}
