//go:build integration

package dunning_test

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"

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
		sub := subscription.StoreSubscription{
			ID:               uuid.New(),
			TenantID:         tenantID,
			StoreID:          storeID,
			StripeCustomerID: "cus_sca_" + o.key,
			Plan:             subscription.PlanStarter,
			Status:           subscription.StatusPaymentActionRequired,
			HostedInvoiceURL: &hostedURL,
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

	sub := subscription.StoreSubscription{
		ID:               uuid.New(),
		TenantID:         tenantID,
		StoreID:          storeID,
		StripeCustomerID: "cus_idem",
		Plan:             subscription.PlanStarter,
		Status:           subscription.StatusPaymentActionRequired,
		HostedInvoiceURL: &hostedURL,
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

	sub := subscription.StoreSubscription{
		ID:               uuid.New(),
		TenantID:         tenantID,
		StoreID:          storeID,
		StripeCustomerID: "cus_no_url",
		Plan:             subscription.PlanStarter,
		Status:           subscription.StatusPaymentActionRequired,
		HostedInvoiceURL: nil, // no URL
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
