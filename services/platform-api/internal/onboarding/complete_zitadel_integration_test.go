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
	"github.com/mark8ly/platform-api/internal/store"
	"github.com/mark8ly/platform-api/internal/tenant"
	"github.com/mark8ly/platform-api/pkg/testdb"
)

// Issue #685 — the committed half of onboarding completion on the
// Zitadel path, against a real database.
//
// What this pins that the unit tests cannot: that a completed onboarding
// leaves BOTH FGA owner tuples in place — one keyed by the merchant's
// lowercased email, which is what apps/admin's resolveWorkspaceTenant
// reads BEFORE authentication, and one keyed by the Zitadel user id,
// which is what the bearer-token API reads. Writing either one alone
// produces a merchant who can sign in but gets 403 from every API call,
// or one who is told "We couldn't find a store for this account. Did you
// finish onboarding?" at the login screen. Before this fix onboarding
// wrote a third key — the GIP uid — which matches neither.

type stubOwnerProvisioner struct {
	uid   string
	calls int
	pass  string
}

func (p *stubOwnerProvisioner) ProvisionStaff(_ context.Context, _, _, _, password string) (string, error) {
	p.calls++
	p.pass = password
	return p.uid, nil
}

func TestIntegration_Complete_ZitadelWritesBothOwnerTuples(t *testing.T) {
	db := testdb.NewDB(t,
		"outbox_events",
		"verification_tokens",
		"onboarding_sessions",
		"stores",
		"tenants",
	)

	prov := &stubOwnerProvisioner{uid: "zitadel-user-685"}
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
		Provisioner:  prov,
	})

	ctx := context.Background()
	now := time.Now()
	sess := &Session{
		Email:           "founder@zitadel-onboarding.local",
		Draft:           json.RawMessage(`{}`),
		Status:          StatusInProgress,
		EmailVerifiedAt: &now,
	}
	if err := onboardingRepo.Create(ctx, sess); err != nil {
		t.Fatalf("seed session: %v", err)
	}

	res, err := svc.Complete(ctx, CompleteRequest{
		SessionID:    sess.ID,
		BusinessName: "Zitadel Onboarding Co",
		Slug:         "zitadel-onboarding",
		// Deliberately mixed case: the tuple must be lowercased.
		OwnerEmail:   "Founder@Zitadel-Onboarding.local",
		CountryCode:  "AU",
		CurrencyCode: "AUD",
		Timezone:     "Australia/Sydney",
		FirstName:    "Ada",
		LastName:     "Lovelace",
		Password:     "Not-A-Real-Password-1!",
	})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if prov.calls != 1 {
		t.Fatalf("provisioner called %d times, want exactly 1", prov.calls)
	}
	if prov.pass != "Not-A-Real-Password-1!" {
		t.Fatalf("password not forwarded to the provisioner: %q", prov.pass)
	}

	// The tenant's owner is the ZITADEL uid, not the caller's — it is
	// the id the bearer API sees and the one later membership writes
	// key on.
	tn, err := tenantRepo.GetByID(ctx, res.TenantID)
	if err != nil {
		t.Fatalf("tenant lookup: %v", err)
	}
	if tn.OwnerUserID != "zitadel-user-685" {
		t.Errorf("tenant.OwnerUserID = %q, want zitadel-user-685", tn.OwnerUserID)
	}

	// The GIP tenant_id claim is a GIP-only concept keyed by GIP uid.
	// Under Zitadel it would resolve nothing and be retried forever, so
	// no row must be enqueued at all.
	var claimCount int64
	if err := db.Model(&outbox.Event{}).Where("kind = ?", GIPClaimOutboxKind).Count(&claimCount).Error; err != nil {
		t.Fatal(err)
	}
	if claimCount != 0 {
		t.Errorf("GIP claim outbox rows = %d, want 0 on the Zitadel path", claimCount)
	}

	fake := authz.NewFake()
	d := outbox.NewDrainer(db, slog.New(slog.NewTextHandler(io.Discard, nil)), outbox.Config{})
	d.Register(FGAOutboxKind, NewFGAOutboxHandler(fake))
	if err := d.Tick(ctx); err != nil {
		t.Fatalf("drainer tick: %v", err)
	}

	// Both readers, both keys.
	if !fake.HasOwnership("founder@zitadel-onboarding.local", res.TenantID) {
		t.Error("no email-keyed owner tuple — admin sign-in resolves membership by email and would say 'no store found'")
	}
	if !fake.HasOwnership("zitadel-user-685", res.TenantID) {
		t.Error("no uid-keyed owner tuple — every bearer-token API call would 403")
	}
	// And never the pre-fix key.
	if fake.HasOwnership("Founder@Zitadel-Onboarding.local", res.TenantID) {
		t.Error("a mixed-case email tuple was written; every email-keyed tuple must be lowercased")
	}
}
