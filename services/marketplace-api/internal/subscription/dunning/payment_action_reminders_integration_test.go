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

	"github.com/mark8ly/marketplace-api/internal/audit"
	"github.com/mark8ly/marketplace-api/internal/email"
	"github.com/mark8ly/marketplace-api/internal/subscription"
	"github.com/mark8ly/marketplace-api/internal/subscription/dunning"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

// TestSCAReminders_SendsAtT14T7T1 verifies that subscriptions in
// payment_action_required receive an email at each of the three offsets when
// their audit entry matches the target day.
func TestSCAReminders_SendsAtT14T7T1(t *testing.T) {
	db := testdb.NewDB(t, "payment_action_reminders", "audit_logs", "store_subscriptions")

	now := time.Now().UTC()
	hostedURL := "https://invoice.stripe.com/i/test"

	offsets := []struct {
		days     int
		key      string
		template email.TemplateID
	}{
		{14, "t_minus_14", email.TemplatePaymentActionReminder},
		{7, "t_minus_7", email.TemplatePaymentActionReminder},
		{1, "t_minus_1", email.TemplatePaymentActionReminder},
	}

	for _, o := range offsets {
		storeID := uuid.New()
		tenantID := uuid.New()
		seedStore(t, db, tenantID, storeID)
		merchantEmail := "merchant-sca-" + o.key + "@example.com"
		sub := subscription.StoreSubscription{
			ID:               uuid.New(),
			TenantID:         tenantID,
			StoreID:          storeID,
			StripeCustomerID: "cus_sca_" + o.key,
			Plan:             subscription.PlanStarter,
			Status:           subscription.StatusPaymentActionRequired,
			HostedInvoiceURL: &hostedURL,
			Email:            &merchantEmail,
		}
		if err := db.Create(&sub).Error; err != nil {
			t.Fatalf("seed sub (%s): %v", o.key, err)
		}
		auditEntry := audit.Entry{
			ID:           uuid.New(),
			TenantID:     tenantID,
			StoreID:      &storeID,
			ActorType:    audit.ActorSystem,
			Action:       "subscription.state_transition",
			ResourceType: "subscription",
			Status:       audit.StatusSuccess,
			Severity:     audit.SeverityWarning,
			Metadata:     audit.Metadata{"to_status": "payment_action_required", "from_status": "active"},
			CreatedAt:    now.AddDate(0, 0, -o.days),
		}
		if err := db.Create(&auditEntry).Error; err != nil {
			t.Fatalf("seed audit (%s): %v", o.key, err)
		}
	}

	client := &capturingClient{}
	cron := dunning.NewSendPaymentActionReminders(db, client, slog.Default(), nil, func() time.Time { return now })
	if err := cron.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if got := client.count(); got != 3 {
		t.Fatalf("expected 3 sends (t-14 + t-7 + t-1), got %d", got)
	}
}

// TestSCAReminders_Idempotent_OnConflict verifies that running the cron twice
// for the same subscription only sends the email once (idempotency via
// ON CONFLICT DO NOTHING).
func TestSCAReminders_Idempotent_OnConflict(t *testing.T) {
	db := testdb.NewDB(t, "payment_action_reminders", "audit_logs", "store_subscriptions")

	now := time.Now().UTC()
	hostedURL := "https://invoice.stripe.com/i/idempotency-test"
	storeID := uuid.New()
	tenantID := uuid.New()
	seedStore(t, db, tenantID, storeID)
	fourteenDaysAgo := now.AddDate(0, 0, -14)

	merchantEmail := "merchant-sca-idem@example.com"
	sub := subscription.StoreSubscription{
		ID:               uuid.New(),
		TenantID:         tenantID,
		StoreID:          storeID,
		StripeCustomerID: "cus_idem",
		Plan:             subscription.PlanStarter,
		Status:           subscription.StatusPaymentActionRequired,
		HostedInvoiceURL: &hostedURL,
		Email:            &merchantEmail,
	}
	if err := db.Create(&sub).Error; err != nil {
		t.Fatalf("seed sub: %v", err)
	}
	auditEntry := audit.Entry{
		ID:           uuid.New(),
		TenantID:     tenantID,
		StoreID:      &storeID,
		ActorType:    audit.ActorSystem,
		Action:       "subscription.state_transition",
		ResourceType: "subscription",
		Status:       audit.StatusSuccess,
		Severity:     audit.SeverityWarning,
		Metadata:     audit.Metadata{"to_status": "payment_action_required", "from_status": "active"},
		CreatedAt:    fourteenDaysAgo,
	}
	if err := db.Create(&auditEntry).Error; err != nil {
		t.Fatalf("seed audit: %v", err)
	}

	client := &capturingClient{}
	cron := dunning.NewSendPaymentActionReminders(db, client, slog.Default(), nil, func() time.Time { return now })

	// First run: should send.
	if err := cron.Run(context.Background()); err != nil {
		t.Fatalf("Run 1: %v", err)
	}
	if got := client.count(); got != 1 {
		t.Fatalf("expected 1 send after first run, got %d", got)
	}

	// Second run: idempotency row already exists — no new send.
	if err := cron.Run(context.Background()); err != nil {
		t.Fatalf("Run 2: %v", err)
	}
	if got := client.count(); got != 1 {
		t.Fatalf("expected still 1 send after second run (idempotent), got %d", got)
	}
}

// TestSCAReminders_SkipsSubsWithoutHostedInvoiceURL verifies that a
// payment_action_required sub without a hosted_invoice_url is not emailed
// (it has no actionable link to include).
func TestSCAReminders_SkipsSubsWithoutHostedInvoiceURL(t *testing.T) {
	db := testdb.NewDB(t, "payment_action_reminders", "audit_logs", "store_subscriptions")

	now := time.Now().UTC()
	storeID := uuid.New()
	tenantID := uuid.New()
	seedStore(t, db, tenantID, storeID)
	fourteenDaysAgo := now.AddDate(0, 0, -14)

	merchantEmail := "merchant-sca-no-url@example.com"
	sub := subscription.StoreSubscription{
		ID:               uuid.New(),
		TenantID:         tenantID,
		StoreID:          storeID,
		StripeCustomerID: "cus_no_url",
		Plan:             subscription.PlanStarter,
		Status:           subscription.StatusPaymentActionRequired,
		HostedInvoiceURL: nil, // no URL
		Email:            &merchantEmail,
	}
	if err := db.Create(&sub).Error; err != nil {
		t.Fatalf("seed sub: %v", err)
	}
	auditEntry := audit.Entry{
		ID:           uuid.New(),
		TenantID:     tenantID,
		StoreID:      &storeID,
		ActorType:    audit.ActorSystem,
		Action:       "subscription.state_transition",
		ResourceType: "subscription",
		Status:       audit.StatusSuccess,
		Severity:     audit.SeverityWarning,
		Metadata:     audit.Metadata{"to_status": "payment_action_required", "from_status": "active"},
		CreatedAt:    fourteenDaysAgo,
	}
	if err := db.Create(&auditEntry).Error; err != nil {
		t.Fatalf("seed audit: %v", err)
	}

	client := &capturingClient{}
	cron := dunning.NewSendPaymentActionReminders(db, client, slog.Default(), nil, func() time.Time { return now })
	if err := cron.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if got := client.count(); got != 0 {
		t.Fatalf("expected 0 sends (no hosted_invoice_url), got %d", got)
	}
}

// TestPaymentActionReminders_UndeliverableCountsSkippedNotSent verifies that
// a payment_action_required subscription seeded with a placeholder (.local)
// address is not mailed, and the skip lands on the skip counter rather than
// the sent counter.
func TestPaymentActionReminders_UndeliverableCountsSkippedNotSent(t *testing.T) {
	db := testdb.NewDB(t, "payment_action_reminders", "audit_logs", "store_subscriptions", "stores")

	now := time.Now().UTC()
	hostedURL := "https://invoice.stripe.com/i/placeholder-test"
	storeID := uuid.New()
	tenantID := uuid.New()
	seedStore(t, db, tenantID, storeID)
	placeholder := "billing+7f3a@mark8ly.local"
	fourteenDaysAgo := now.AddDate(0, 0, -14)

	sub := subscription.StoreSubscription{
		ID:               uuid.New(),
		TenantID:         tenantID,
		StoreID:          storeID,
		StripeCustomerID: "cus_placeholder",
		Plan:             subscription.PlanStarter,
		Status:           subscription.StatusPaymentActionRequired,
		HostedInvoiceURL: &hostedURL,
		Email:            &placeholder,
	}
	require.NoError(t, db.Create(&sub).Error)
	auditEntry := audit.Entry{
		ID:           uuid.New(),
		TenantID:     tenantID,
		StoreID:      &storeID,
		ActorType:    audit.ActorSystem,
		Action:       "subscription.state_transition",
		ResourceType: "subscription",
		Status:       audit.StatusSuccess,
		Severity:     audit.SeverityWarning,
		Metadata:     audit.Metadata{"to_status": "payment_action_required", "from_status": "active"},
		CreatedAt:    fourteenDaysAgo,
	}
	require.NoError(t, db.Create(&auditEntry).Error)

	client := &stubClient{}
	sent, skipped := &stubVec{}, &stubSkip{}
	cron := dunning.NewSendPaymentActionReminders(db, client, nil, sent, func() time.Time { return now }).
		WithSkipCounter(skipped)

	require.NoError(t, cron.Run(context.Background()))

	require.Empty(t, client.sent, "mailed a .local address")
	require.Zero(t, sent.n["t_minus_14"], "sent counter incremented for mail never sent")
	require.Equal(t, 1, skipped.n["payment_action_reminder/placeholder_address"])
}

// TestPaymentActionReminders_UndeliverableReleasesClaimSoRetryCanSend verifies
// that a send failure caused by an undeliverable address (missing/placeholder)
// releases the idempotency slot, so a later run (after backfill or a
// customer.updated webhook lands a real address) can still deliver the notice.
func TestPaymentActionReminders_UndeliverableReleasesClaimSoRetryCanSend(t *testing.T) {
	db := testdb.NewDB(t, "payment_action_reminders", "audit_logs", "store_subscriptions", "stores")

	now := time.Now().UTC()
	hostedURL := "https://invoice.stripe.com/i/release-test"
	storeID := uuid.New()
	tenantID := uuid.New()
	seedStore(t, db, tenantID, storeID)
	placeholder := "billing+7f3a@mark8ly.local"
	fourteenDaysAgo := now.AddDate(0, 0, -14)

	sub := subscription.StoreSubscription{
		ID:               uuid.New(),
		TenantID:         tenantID,
		StoreID:          storeID,
		StripeCustomerID: "cus_release",
		Plan:             subscription.PlanStarter,
		Status:           subscription.StatusPaymentActionRequired,
		HostedInvoiceURL: &hostedURL,
		Email:            &placeholder,
	}
	require.NoError(t, db.Create(&sub).Error)
	auditEntry := audit.Entry{
		ID:           uuid.New(),
		TenantID:     tenantID,
		StoreID:      &storeID,
		ActorType:    audit.ActorSystem,
		Action:       "subscription.state_transition",
		ResourceType: "subscription",
		Status:       audit.StatusSuccess,
		Severity:     audit.SeverityWarning,
		Metadata:     audit.Metadata{"to_status": "payment_action_required", "from_status": "active"},
		CreatedAt:    fourteenDaysAgo,
	}
	require.NoError(t, db.Create(&auditEntry).Error)

	client := &stubClient{}
	run := func() *dunning.SendPaymentActionReminders {
		return dunning.NewSendPaymentActionReminders(db, client, nil, &stubVec{}, func() time.Time { return now }).
			WithSkipCounter(&stubSkip{})
	}
	require.NoError(t, run().Run(context.Background()))
	require.Empty(t, client.sent, "mailed an undeliverable address")

	var claims int64
	require.NoError(t, db.Raw(`SELECT count(*) FROM payment_action_reminders`).Scan(&claims).Error)
	require.EqualValues(t, 0, claims, "claim was burned; a later run can never deliver this notice")

	good := "merchant@example.com"
	require.NoError(t, db.Exec(`UPDATE store_subscriptions SET email = ? WHERE id = ?`, good, sub.ID).Error)
	require.NoError(t, run().Run(context.Background()))
	require.Equal(t, []string{good}, client.sent, "retry after backfill did not deliver")
}

// TestPaymentActionReminders_TransportFailureKeepsClaimBurned pins the
// deliberate at-most-once contract: a transport failure must NOT release the
// claim, or a retry could duplicate the billing notice.
func TestPaymentActionReminders_TransportFailureKeepsClaimBurned(t *testing.T) {
	db := testdb.NewDB(t, "payment_action_reminders", "audit_logs", "store_subscriptions", "stores")

	now := time.Now().UTC()
	hostedURL := "https://invoice.stripe.com/i/transport-test"
	storeID := uuid.New()
	tenantID := uuid.New()
	seedStore(t, db, tenantID, storeID)
	addr := "merchant@example.com"
	fourteenDaysAgo := now.AddDate(0, 0, -14)

	sub := subscription.StoreSubscription{
		ID:               uuid.New(),
		TenantID:         tenantID,
		StoreID:          storeID,
		StripeCustomerID: "cus_transport",
		Plan:             subscription.PlanStarter,
		Status:           subscription.StatusPaymentActionRequired,
		HostedInvoiceURL: &hostedURL,
		Email:            &addr,
	}
	require.NoError(t, db.Create(&sub).Error)
	auditEntry := audit.Entry{
		ID:           uuid.New(),
		TenantID:     tenantID,
		StoreID:      &storeID,
		ActorType:    audit.ActorSystem,
		Action:       "subscription.state_transition",
		ResourceType: "subscription",
		Status:       audit.StatusSuccess,
		Severity:     audit.SeverityWarning,
		Metadata:     audit.Metadata{"to_status": "payment_action_required", "from_status": "active"},
		CreatedAt:    fourteenDaysAgo,
	}
	require.NoError(t, db.Create(&auditEntry).Error)

	client := &stubClient{err: errors.New("sendgrid 503")}
	cron := dunning.NewSendPaymentActionReminders(db, client, nil, &stubVec{}, func() time.Time { return now }).
		WithSkipCounter(&stubSkip{})
	require.NoError(t, cron.Run(context.Background()))

	var claims int64
	require.NoError(t, db.Raw(`SELECT count(*) FROM payment_action_reminders`).Scan(&claims).Error)
	require.EqualValues(t, 1, claims, "transport failure released the claim; a retry could duplicate")
}
