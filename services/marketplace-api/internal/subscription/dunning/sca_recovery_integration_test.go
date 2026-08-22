//go:build integration

package dunning_test

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"

	"github.com/mark8ly/marketplace-api/internal/audit"
	"github.com/mark8ly/marketplace-api/internal/billing/dispatch"
	"github.com/mark8ly/marketplace-api/internal/email"
	"github.com/mark8ly/marketplace-api/internal/subscription"
	"github.com/mark8ly/marketplace-api/internal/subscription/dunning"
	"github.com/mark8ly/marketplace-api/internal/webhookevents"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

// TestSCARecovery_WebhookPaymentActionRequired_SCAReminders_InvoicePaidClears
// is an end-to-end test of the SCA (3D Secure) recovery path:
//
//  1. invoice.payment_action_required webhook → status=payment_action_required, hosted_invoice_url set
//  2. SCA reminders cron at T-14 → email sent + idempotency row inserted
//  3. invoice.paid webhook → hosted_invoice_url cleared, first_charge_at stamped
//  4. Second SCA reminders run → no new email (idempotency row exists)
func TestSCARecovery_WebhookPaymentActionRequired_SCAReminders_InvoicePaidClears(t *testing.T) {
	db := testdb.NewDB(t, "payment_action_reminders", "audit_logs", "store_subscriptions")
	ctx := context.Background()

	storeID := uuid.New()
	tenantID := uuid.New()
	stripeCustomerID := "cus_sca_recovery_" + storeID.String()[:8]
	hostedURL := "https://stripe.example/i/test"

	// Seed an active subscription.
	sub := subscription.StoreSubscription{
		ID:               uuid.New(),
		TenantID:         tenantID,
		StoreID:          storeID,
		StripeCustomerID: stripeCustomerID,
		Plan:             subscription.PlanStarter,
		Status:           subscription.StatusActive,
	}
	if err := db.Create(&sub).Error; err != nil {
		t.Fatalf("seed sub: %v", err)
	}

	// --- Step 1: dispatch invoice.payment_action_required ---
	parPayload, err := json.Marshal(map[string]any{
		"data": map[string]any{
			"object": map[string]any{
				"customer":           stripeCustomerID,
				"hosted_invoice_url": hostedURL,
			},
		},
	})
	if err != nil {
		t.Fatalf("marshal PAR payload: %v", err)
	}

	dispatcher := dispatch.New(nil) // nil emitter — test doesn't need audit emission here
	parEvent := webhookevents.StripeWebhookEvent{
		EventID:   "evt_par_" + storeID.String(),
		EventType: "invoice.payment_action_required",
		Payload:   datatypes.JSON(parPayload),
	}
	if err := dispatcher.Dispatch(ctx, db, parEvent); err != nil {
		t.Fatalf("dispatch PAR: %v", err)
	}

	// Assert status = payment_action_required and hosted_invoice_url is set.
	var afterPAR subscription.StoreSubscription
	if err := db.Where("store_id = ?", storeID).First(&afterPAR).Error; err != nil {
		t.Fatalf("reload after PAR: %v", err)
	}
	if afterPAR.Status != subscription.StatusPaymentActionRequired {
		t.Fatalf("expected payment_action_required after PAR event, got %q", afterPAR.Status)
	}
	if afterPAR.HostedInvoiceURL == nil || *afterPAR.HostedInvoiceURL != hostedURL {
		t.Fatalf("expected hosted_invoice_url = %q, got %v", hostedURL, afterPAR.HostedInvoiceURL)
	}

	// --- Step 2: seed audit row 14 days ago so the T-14 SCA reminder fires ---
	fourteenDaysAgo := time.Now().UTC().Add(-14 * 24 * time.Hour)
	auditPAR := audit.Entry{
		ID:           uuid.New(),
		TenantID:     tenantID,
		StoreID:      storeID,
		ActorType:    audit.ActorSystem,
		Action:       "subscription.state_transition",
		ResourceType: "subscription",
		Status:       audit.StatusSuccess,
		Severity:     audit.SeverityWarning,
		Metadata:     audit.Metadata{"from_status": "active", "to_status": "payment_action_required"},
		CreatedAt:    fourteenDaysAgo,
	}
	if err := db.Create(&auditPAR).Error; err != nil {
		t.Fatalf("seed PAR audit row: %v", err)
	}

	// --- Step 3: run SCA reminders cron; assert one email sent ---
	mailer := &recordingMailer{}
	now := time.Now().UTC()
	remindersCron := dunning.NewSendPaymentActionReminders(db, mailer, slog.Default(), nil, fixedClock(now))
	if err := remindersCron.Run(ctx); err != nil {
		t.Fatalf("reminders Run 1: %v", err)
	}

	if got := mailer.callCount(); got != 1 {
		t.Fatalf("expected 1 email after first reminders run, got %d", got)
	}
	mailer.mu.Lock()
	call := mailer.Calls[0]
	mailer.mu.Unlock()
	if call.Template != email.TemplatePaymentActionReminder {
		t.Fatalf("expected template %q, got %q", email.TemplatePaymentActionReminder, call.Template)
	}
	if call.To != storeID.String() {
		t.Fatalf("expected To = store_id %q, got %q", storeID.String(), call.To)
	}

	// Assert idempotency row inserted for (store_id, t_minus_14).
	var parRows int64
	if err := db.Raw(`
		SELECT COUNT(*) FROM payment_action_reminders
		WHERE subscription_id = ? AND offset_key = 't_minus_14'`,
		storeID,
	).Scan(&parRows).Error; err != nil {
		t.Fatalf("count payment_action_reminders: %v", err)
	}
	if parRows != 1 {
		t.Fatalf("expected 1 payment_action_reminders row, got %d", parRows)
	}

	// --- Step 4: dispatch invoice.paid → hosted_invoice_url cleared, first_charge_at stamped ---
	paidPayload, err := json.Marshal(map[string]any{
		"data": map[string]any{
			"object": map[string]any{
				"customer": stripeCustomerID,
			},
		},
	})
	if err != nil {
		t.Fatalf("marshal paid payload: %v", err)
	}
	paidEvent := webhookevents.StripeWebhookEvent{
		EventID:   "evt_paid_" + storeID.String(),
		EventType: "invoice.paid",
		Payload:   datatypes.JSON(paidPayload),
	}
	if err := dispatcher.Dispatch(ctx, db, paidEvent); err != nil {
		t.Fatalf("dispatch invoice.paid: %v", err)
	}

	var afterPaid subscription.StoreSubscription
	if err := db.Where("store_id = ?", storeID).First(&afterPaid).Error; err != nil {
		t.Fatalf("reload after invoice.paid: %v", err)
	}
	// hosted_invoice_url must be cleared.
	if afterPaid.HostedInvoiceURL != nil {
		t.Fatalf("expected hosted_invoice_url = nil after invoice.paid, got %q", *afterPaid.HostedInvoiceURL)
	}
	// first_charge_at must be stamped.
	if afterPaid.FirstChargeAt == nil {
		t.Fatal("expected first_charge_at to be stamped after invoice.paid")
	}

	// --- Step 5: second reminders run — idempotency row exists, no new email ---
	if err := remindersCron.Run(ctx); err != nil {
		t.Fatalf("reminders Run 2: %v", err)
	}
	// Still only 1 email total (the t_minus_14 idempotency row blocks a duplicate).
	if got := mailer.callCount(); got != 1 {
		t.Fatalf("expected still 1 email after second reminders run (idempotent), got %d", got)
	}

	// Verify: a second audit row insert is not needed; the query filter for
	// payment_action_required status already excludes rows where status advanced.
	_ = fmt.Sprintf("store %s passed SCA recovery end-to-end", storeID)
}
