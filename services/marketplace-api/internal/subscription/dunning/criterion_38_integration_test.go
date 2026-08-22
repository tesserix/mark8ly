//go:build integration

package dunning_test

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/mark8ly/marketplace-api/internal/audit"
	"github.com/mark8ly/marketplace-api/internal/subscription"
	"github.com/mark8ly/marketplace-api/internal/subscription/dunning"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

// TestCriterion38_DunningLeavesPaymentActionRequiredAlone verifies that:
//
//  1. The daily ladder does not touch a payment_action_required subscription
//     (ladder only processes past_due, expired, store_closed).
//  2. The dunning emails cron sends no email (it queries status='past_due' only).
//  3. The SCA reminders cron DOES send an email for a T-14 PAR subscription.
func TestCriterion38_DunningLeavesPaymentActionRequiredAlone(t *testing.T) {
	db := testdb.NewDB(t, "payment_action_reminders", "audit_logs", "store_subscriptions")
	ctx := context.Background()

	storeID := uuid.New()
	tenantID := uuid.New()
	hostedURL := "https://stripe.example/i/criterion38"

	// FirstChargeAt 30 days ago simulates an established sub — well outside the
	// 14-day refund window if it were in past_due. Irrelevant for PAR logic but
	// provided for completeness.
	thirtyDaysAgo := time.Now().UTC().Add(-30 * 24 * time.Hour)
	fourteenDaysAgo := time.Now().UTC().Add(-14 * 24 * time.Hour)

	sub := subscription.StoreSubscription{
		ID:               uuid.New(),
		TenantID:         tenantID,
		StoreID:          storeID,
		StripeCustomerID: "cus_c38_" + storeID.String()[:8],
		Plan:             subscription.PlanStarter,
		Status:           subscription.StatusPaymentActionRequired,
		HostedInvoiceURL: &hostedURL,
		FirstChargeAt:    &thirtyDaysAgo,
	}
	if err := db.Create(&sub).Error; err != nil {
		t.Fatalf("seed sub: %v", err)
	}

	// Seed the PAR audit row 14 days ago so the T-14 SCA reminder fires.
	auditPAR := audit.Entry{
		ID:           uuid.New(),
		TenantID:     tenantID,
		StoreID:      &storeID,
		ActorType:    audit.ActorSystem,
		Action:       "subscription.state_transition",
		ResourceType: "subscription",
		Status:       audit.StatusSuccess,
		Severity:     audit.SeverityWarning,
		Metadata:     audit.Metadata{"from_status": "active", "to_status": "payment_action_required"},
		CreatedAt:    fourteenDaysAgo,
	}
	if err := db.Create(&auditPAR).Error; err != nil {
		t.Fatalf("seed audit row: %v", err)
	}

	now := time.Now().UTC()

	// --- Assertion 1: ladder does NOT touch payment_action_required ---
	ladder := dunning.NewStepDailyLadder(db, nil, slog.Default(), nil, fixedClock(now))
	if err := ladder.Run(ctx); err != nil {
		t.Fatalf("ladder.Run: %v", err)
	}
	var afterLadder subscription.StoreSubscription
	if err := db.Where("store_id = ?", storeID).First(&afterLadder).Error; err != nil {
		t.Fatalf("reload after ladder: %v", err)
	}
	if afterLadder.Status != subscription.StatusPaymentActionRequired {
		t.Fatalf("ladder must not touch payment_action_required; got %q", afterLadder.Status)
	}

	// --- Assertion 2: dunning emails cron sends zero emails (filters status='past_due') ---
	dunningMailer := &recordingMailer{}
	dunningEmailsCron := dunning.NewSendDunningEmails(db, dunningMailer, slog.Default(), nil, fixedClock(now))
	if err := dunningEmailsCron.Run(ctx); err != nil {
		t.Fatalf("dunning emails Run: %v", err)
	}
	if got := dunningMailer.callCount(); got != 0 {
		t.Fatalf("dunning emails cron must send 0 emails for payment_action_required sub, got %d", got)
	}

	// --- Assertion 3: SCA reminders cron sends one email (T-14 match) ---
	scaMailer := &recordingMailer{}
	scaCron := dunning.NewSendPaymentActionReminders(db, scaMailer, slog.Default(), nil, fixedClock(now))
	if err := scaCron.Run(ctx); err != nil {
		t.Fatalf("SCA reminders Run: %v", err)
	}
	if got := scaMailer.callCount(); got != 1 {
		t.Fatalf("SCA reminders cron must send 1 email (T-14), got %d", got)
	}
	scaMailer.mu.Lock()
	scaCall := scaMailer.Calls[0]
	scaMailer.mu.Unlock()
	if scaCall.To != storeID.String() {
		t.Fatalf("SCA reminder To = %q; want store_id %q", scaCall.To, storeID.String())
	}
}
