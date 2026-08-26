//go:build integration

package dunning_test

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/billing/trial"
	"github.com/mark8ly/marketplace-api/internal/email"
	"github.com/mark8ly/marketplace-api/internal/subscription"
	"github.com/mark8ly/marketplace-api/internal/subscription/dunning"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

// trialReminderCadenceCase encodes a single offset's expected behaviour:
// "a sub created N days ago, with this PM state, should receive this template".
// All offsets share the same setup pattern, so iterating over a table keeps
// the test compact.
type trialReminderCadenceCase struct {
	daysSinceSignup int
	hasPM           bool
	wantTemplate    email.TemplateID
	desc            string
}

// TestTrialReminders_SendsAtAllNoPMOffsets verifies the five-touch no-PM
// cadence (T-15, T-10, T-7, T-3, T-1) fires correctly when subscription
// created_at lines up with each offset day.
func TestTrialReminders_SendsAtAllNoPMOffsets(t *testing.T) {
	db := testdb.NewDB(t, "trial_reminders", "store_subscriptions")
	now := time.Now().UTC()

	cases := []trialReminderCadenceCase{
		{trial.TrialDays - 15, false, email.TemplateTrialNoPMT15, "no_pm_t15"},
		{trial.TrialDays - 10, false, email.TemplateTrialNoPMT10, "no_pm_t10"},
		{trial.TrialDays - 7, false, email.TemplateTrialNoPMT7, "no_pm_t7"},
		{trial.TrialDays - 3, false, email.TemplateTrialNoPMT3, "no_pm_t3"},
		{trial.TrialDays - 1, false, email.TemplateTrialNoPMT1, "no_pm_t1"},
	}

	for _, c := range cases {
		// Place created_at deep inside the target day to avoid boundary flake
		// when "now" is itself near midnight UTC.
		created := time.Date(now.Year(), now.Month(), now.Day(), 12, 0, 0, 0, time.UTC).
			AddDate(0, 0, -c.daysSinceSignup)
		tenantID := uuid.New()
		storeID := uuid.New()
		seedStore(t, db, tenantID, storeID)
		merchantEmail := "merchant-" + c.desc + "@example.com"
		sub := subscription.StoreSubscription{
			ID:                      uuid.New(),
			TenantID:                tenantID,
			StoreID:                 storeID,
			StripeCustomerID:        "cus_" + c.desc,
			Plan:                    subscription.PlanTrial,
			Status:                  subscription.StatusTrialing,
			HasDefaultPaymentMethod: c.hasPM,
			CreatedAt:               created,
			Email:                   &merchantEmail,
		}
		if err := db.Create(&sub).Error; err != nil {
			t.Fatalf("seed sub (%s): %v", c.desc, err)
		}
	}

	client := &capturingClient{}
	cron := dunning.NewSendTrialReminders(db, client, slog.Default(), nil, func() time.Time { return now })
	if err := cron.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if got := client.count(); got != len(cases) {
		t.Fatalf("expected %d sends across all no-PM offsets, got %d", len(cases), got)
	}

	// Verify each offset surfaced exactly once with the expected template.
	got := map[email.TemplateID]int{}
	for _, s := range client.sends {
		got[s.Template]++
	}
	for _, c := range cases {
		if got[c.wantTemplate] != 1 {
			t.Fatalf("offset %s: expected exactly 1 send, got %d", c.desc, got[c.wantTemplate])
		}
	}
}

// TestTrialReminders_HasPMSendsOnlyT1 verifies the has-PM cadence is a single
// T-1 reminder — even though the sub is in the cron's date window for the
// no-PM offsets, those rows are filtered out by has_default_payment_method=false.
func TestTrialReminders_HasPMSendsOnlyT1(t *testing.T) {
	db := testdb.NewDB(t, "trial_reminders", "store_subscriptions")
	now := time.Now().UTC()

	// Sub created 89 days ago with PM on file → only 'has_pm_t_minus_1' fires.
	created := time.Date(now.Year(), now.Month(), now.Day(), 12, 0, 0, 0, time.UTC).
		AddDate(0, 0, -(trial.TrialDays - 1))
	tenantID := uuid.New()
	storeID := uuid.New()
	seedStore(t, db, tenantID, storeID)
	merchantEmail := "merchant-haspm@example.com"
	sub := subscription.StoreSubscription{
		ID:                      uuid.New(),
		TenantID:                tenantID,
		StoreID:                 storeID,
		StripeCustomerID:        "cus_haspm",
		Plan:                    subscription.PlanStarter,
		Status:                  subscription.StatusTrialing,
		HasDefaultPaymentMethod: true,
		CreatedAt:               created,
		Email:                   &merchantEmail,
	}
	if err := db.Create(&sub).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}

	client := &capturingClient{}
	cron := dunning.NewSendTrialReminders(db, client, slog.Default(), nil, func() time.Time { return now })
	if err := cron.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if got := client.count(); got != 1 {
		t.Fatalf("expected exactly 1 send (has_pm_t_minus_1), got %d", got)
	}
	if client.sends[0].Template != email.TemplateTrialHasPMT1 {
		t.Fatalf("expected TemplateTrialHasPMT1, got %s", client.sends[0].Template)
	}
}

// TestTrialReminders_Idempotent verifies that running the cron twice for the
// same subscription on the same day only sends each reminder once. Failure
// here would mean a merchant gets duplicate emails on cron retries / pod
// restarts.
func TestTrialReminders_Idempotent(t *testing.T) {
	db := testdb.NewDB(t, "trial_reminders", "store_subscriptions")
	now := time.Now().UTC()

	created := time.Date(now.Year(), now.Month(), now.Day(), 12, 0, 0, 0, time.UTC).
		AddDate(0, 0, -(trial.TrialDays - 7))
	tenantID := uuid.New()
	storeID := uuid.New()
	seedStore(t, db, tenantID, storeID)
	merchantEmail := "merchant-idem@example.com"
	sub := subscription.StoreSubscription{
		ID:                      uuid.New(),
		TenantID:                tenantID,
		StoreID:                 storeID,
		StripeCustomerID:        "cus_idem",
		Plan:                    subscription.PlanTrial,
		Status:                  subscription.StatusTrialing,
		HasDefaultPaymentMethod: false,
		CreatedAt:               created,
		Email:                   &merchantEmail,
	}
	if err := db.Create(&sub).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}

	client := &capturingClient{}
	cron := dunning.NewSendTrialReminders(db, client, slog.Default(), nil, func() time.Time { return now })

	// First pass should send T-7 once.
	if err := cron.Run(context.Background()); err != nil {
		t.Fatalf("Run 1: %v", err)
	}
	if got := client.count(); got != 1 {
		t.Fatalf("first run: expected 1 send, got %d", got)
	}

	// Second pass on the same simulated day must be a no-op (ON CONFLICT).
	if err := cron.Run(context.Background()); err != nil {
		t.Fatalf("Run 2: %v", err)
	}
	if got := client.count(); got != 1 {
		t.Fatalf("second run: expected still 1 send (idempotent), got %d", got)
	}
}

// TestTrialReminders_TenantIsolation verifies that two tenants whose trials
// land on the same offset day each receive their own reminder — no cross-
// tenant leakage in the SQL filter, and the idempotency PK is correctly
// scoped per-subscription rather than per-offset alone.
func TestTrialReminders_TenantIsolation(t *testing.T) {
	db := testdb.NewDB(t, "trial_reminders", "store_subscriptions")
	now := time.Now().UTC()

	created := time.Date(now.Year(), now.Month(), now.Day(), 12, 0, 0, 0, time.UTC).
		AddDate(0, 0, -(trial.TrialDays - 3))

	for _, suffix := range []string{"a", "b"} {
		tenantID := uuid.New()
		storeID := uuid.New()
		seedStore(t, db, tenantID, storeID)
		merchantEmail := "merchant-iso-" + suffix + "@example.com"
		sub := subscription.StoreSubscription{
			ID:                      uuid.New(),
			TenantID:                tenantID,
			StoreID:                 storeID,
			StripeCustomerID:        "cus_iso_" + suffix,
			Plan:                    subscription.PlanTrial,
			Status:                  subscription.StatusTrialing,
			HasDefaultPaymentMethod: false,
			CreatedAt:               created,
			Email:                   &merchantEmail,
		}
		if err := db.Create(&sub).Error; err != nil {
			t.Fatalf("seed %s: %v", suffix, err)
		}
	}

	client := &capturingClient{}
	cron := dunning.NewSendTrialReminders(db, client, slog.Default(), nil, func() time.Time { return now })
	if err := cron.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if got := client.count(); got != 2 {
		t.Fatalf("expected 2 sends (one per tenant on T-3), got %d", got)
	}
}

// TestTrialReminders_ExpiredSubsAreSkipped verifies that subs whose status has
// already advanced past trialing/signup (e.g. expired, active) are not
// emailed even if their created_at falls in a target window. This guards
// against late-arriving cron ticks reminding someone whose trial ended weeks
// ago.
func TestTrialReminders_ExpiredSubsAreSkipped(t *testing.T) {
	db := testdb.NewDB(t, "trial_reminders", "store_subscriptions")
	now := time.Now().UTC()

	created := time.Date(now.Year(), now.Month(), now.Day(), 12, 0, 0, 0, time.UTC).
		AddDate(0, 0, -(trial.TrialDays - 1))
	tenantID := uuid.New()
	storeID := uuid.New()
	seedStore(t, db, tenantID, storeID)
	merchantEmail := "merchant-expired@example.com"
	sub := subscription.StoreSubscription{
		ID:                      uuid.New(),
		TenantID:                tenantID,
		StoreID:                 storeID,
		StripeCustomerID:        "cus_expired",
		Plan:                    subscription.PlanTrial,
		Status:                  subscription.StatusExpired,
		HasDefaultPaymentMethod: false,
		CreatedAt:               created,
		Email:                   &merchantEmail,
	}
	if err := db.Create(&sub).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}

	client := &capturingClient{}
	cron := dunning.NewSendTrialReminders(db, client, slog.Default(), nil, func() time.Time { return now })
	if err := cron.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := client.count(); got != 0 {
		t.Fatalf("expected 0 sends (expired sub), got %d", got)
	}
}

// TestTrialReminders_UndeliverableCountsSkippedNotSent verifies that a trial
// subscription seeded with a placeholder (.local) address is not mailed, and
// that the skip is recorded on the skip counter rather than the sent
// counter — the same #381-style lie this cron must not repeat.
func TestTrialReminders_UndeliverableCountsSkippedNotSent(t *testing.T) {
	db := testdb.NewDB(t, "store_subscriptions", "stores", "trial_reminders")
	now := time.Now().UTC()

	placeholder := "billing+7f3a@mark8ly.local"
	// A trial ending in 7 days, with no payment method — the t_minus_7 nudge.
	seedTrialSub(t, db, now.AddDate(0, 0, -83), nil, false, &placeholder)

	client := &stubClient{}
	sent, skipped := &stubVec{}, &stubSkip{}
	cron := dunning.NewSendTrialReminders(db, client, nil, sent, func() time.Time { return now }).
		WithSkipCounter(skipped)

	require.NoError(t, cron.Run(context.Background()))

	require.Empty(t, client.sent, "mailed a .local address")
	require.Zero(t, sent.n["t_minus_7"], "sent counter incremented for mail never sent")
	require.Equal(t, 1, skipped.n["trial_no_pm_t7/placeholder_address"])
}

// The claim is deliberately NOT released on failure: at-most-once beats a
// duplicate. This pins that contract so nobody "fixes" it into at-least-once.
func TestTrialReminders_FailedSendDoesNotReleaseTheClaim(t *testing.T) {
	db := testdb.NewDB(t, "store_subscriptions", "stores", "trial_reminders")
	now := time.Now().UTC()

	addr := "merchant@example.com"
	seedTrialSub(t, db, now.AddDate(0, 0, -83), nil, false, &addr)

	client := &stubClient{err: errors.New("sendgrid 503")}
	cron := dunning.NewSendTrialReminders(db, client, nil, &stubVec{}, func() time.Time { return now })
	require.NoError(t, cron.Run(context.Background()))

	var claims int64
	require.NoError(t, db.Raw(`SELECT count(*) FROM trial_reminders`).Scan(&claims).Error)
	require.EqualValues(t, 1, claims, "the burned slot must stay claimed")
}
