//go:build integration

package onboarding

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/mark8ly/platform-api/internal/notification"
	"github.com/mark8ly/platform-api/internal/store"
	"github.com/mark8ly/platform-api/internal/tenant"
	apperrors "github.com/mark8ly/platform-api/pkg/errors"
	"github.com/mark8ly/platform-api/pkg/testdb"
)

// newUniquenessSvc builds the onboarding service stack against a real DB
// with the tables the owner-email guard touches.
func newUniquenessSvc(t *testing.T) (*Service, Repository, tenant.Repository) {
	t.Helper()
	db := testdb.NewDB(t,
		"outbox_events",
		"verification_tokens",
		"onboarding_sessions",
		"stores",
		"tenants",
	)
	tenantRepo := tenant.NewRepository(db)
	onboardingRepo := NewRepository(db)
	svc := NewService(Config{
		DB:           db,
		Repo:         onboardingRepo,
		TenantRepo:   tenantRepo,
		StoreRepo:    store.NewRepository(db),
		Sender:       notification.NoopSender{},
		EmailFrom:    "noreply@test.local",
		SupportEmail: "help@test.local",
	})
	return svc, onboardingRepo, tenantRepo
}

// seedVerifiedSession creates an onboarding session already past email
// verification, which is the precondition Complete enforces.
func seedVerifiedSession(t *testing.T, repo Repository, email string) *Session {
	t.Helper()
	now := time.Now()
	sess := &Session{
		Email:           email,
		Draft:           json.RawMessage(`{}`),
		Status:          StatusInProgress,
		EmailVerifiedAt: &now,
	}
	if err := repo.Create(context.Background(), sess); err != nil {
		t.Fatalf("seed session: %v", err)
	}
	return sess
}

// completeFor runs a full onboarding for the given email/slug.
func completeFor(t *testing.T, svc *Service, repo Repository, email, slug, uid string) error {
	t.Helper()
	sess := seedVerifiedSession(t, repo, email)
	_, err := svc.Complete(context.Background(), CompleteRequest{
		SessionID:    sess.ID,
		BusinessName: "Store " + slug,
		Slug:         slug,
		OwnerUserID:  uid,
		OwnerEmail:   email,
		CountryCode:  "US",
		CurrencyCode: "USD",
		Timezone:     "America/New_York",
	})
	return err
}

// TestIntegration_Create_RejectsEmailOwningAnotherTenant is the
// regression test for the prod bug: onboarding let a merchant walk the
// entire wizard with an email that already administered another store,
// only failing at the very end (or at GIP's EMAIL_EXISTS, which reads
// as an auth error). The guard must fire at step 1.
func TestIntegration_Create_RejectsEmailOwningAnotherTenant(t *testing.T) {
	svc, repo, _ := newUniquenessSvc(t)
	ctx := context.Background()

	if err := completeFor(t, svc, repo, "founder@dupe-test.local", "first-store", "gip-uid-1"); err != nil {
		t.Fatalf("first onboarding should succeed: %v", err)
	}

	_, err := svc.Create(ctx, CreateRequest{Email: "founder@dupe-test.local"})
	if err == nil {
		t.Fatal("Create accepted an email that already owns a tenant; want conflict")
	}
	assertOwnerEmailConflict(t, err)
}

// The guard must be case-insensitive — Founder@ and founder@ are the
// same human, and the unique index compares on lower(owner_email).
func TestIntegration_Create_EmailComparisonIsCaseInsensitive(t *testing.T) {
	svc, repo, _ := newUniquenessSvc(t)
	ctx := context.Background()

	if err := completeFor(t, svc, repo, "founder@case-test.local", "case-first", "gip-uid-2"); err != nil {
		t.Fatalf("first onboarding should succeed: %v", err)
	}

	for _, variant := range []string{
		"Founder@case-test.local",
		"FOUNDER@CASE-TEST.LOCAL",
		"  founder@case-test.local  ",
	} {
		if _, err := svc.Create(ctx, CreateRequest{Email: variant}); err == nil {
			t.Errorf("Create(%q) accepted a case/whitespace variant of a taken email", variant)
		}
	}
}

// TestIntegration_Complete_RejectsEmailOwningAnotherTenant covers the
// bypass path: the old guard lived only in the Next.js server action, so
// calling POST /onboarding/sessions/:id/complete directly could mint an
// unlimited number of tenants under one owner email. Complete must
// enforce it server-side, independent of Create.
func TestIntegration_Complete_RejectsEmailOwningAnotherTenant(t *testing.T) {
	svc, repo, tenantRepo := newUniquenessSvc(t)
	ctx := context.Background()

	if err := completeFor(t, svc, repo, "founder@bypass-test.local", "bypass-first", "gip-uid-3"); err != nil {
		t.Fatalf("first onboarding should succeed: %v", err)
	}

	// Seed a verified session directly, skipping Create entirely — this
	// is exactly what a direct API call to :id/complete looks like.
	err := completeFor(t, svc, repo, "founder@bypass-test.local", "bypass-second", "gip-uid-4")
	if err == nil {
		t.Fatal("Complete minted a second tenant for an email that already owns one")
	}
	assertOwnerEmailConflict(t, err)

	// And the original tenant is still intact.
	exists, qErr := tenantRepo.OwnerEmailExists(ctx, "founder@bypass-test.local")
	if qErr != nil {
		t.Fatalf("OwnerEmailExists: %v", qErr)
	}
	if !exists {
		t.Fatal("first tenant vanished")
	}
}

// A fresh, unused email must still sail through — the guard should not
// be so blunt that it blocks legitimate signups.
func TestIntegration_Create_AllowsUnusedEmail(t *testing.T) {
	svc, repo, _ := newUniquenessSvc(t)
	ctx := context.Background()

	if err := completeFor(t, svc, repo, "taken@fresh-test.local", "fresh-first", "gip-uid-5"); err != nil {
		t.Fatalf("first onboarding should succeed: %v", err)
	}

	if _, err := svc.Create(ctx, CreateRequest{Email: "brand-new@fresh-test.local"}); err != nil {
		t.Fatalf("Create rejected an unused email: %v", err)
	}
}

func assertOwnerEmailConflict(t *testing.T, err error) {
	t.Helper()
	appErr, ok := apperrors.As(err)
	if !ok {
		t.Fatalf("error %v (%T) is not an *apperrors.AppError", err, err)
	}
	if appErr.Code != "owner_email_already_in_use" {
		t.Errorf("Code = %q, want owner_email_already_in_use", appErr.Code)
	}
	// The message is user-facing — it must actually tell the merchant
	// what to do, not just that something conflicted.
	if !strings.Contains(strings.ToLower(appErr.Message), "different email") {
		t.Errorf("Message = %q, want it to tell the user to use a different email", appErr.Message)
	}
}
