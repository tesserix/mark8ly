//go:build integration

package dunning_test

import (
	"context"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/mark8ly/marketplace-api/internal/audit"
	"github.com/mark8ly/marketplace-api/internal/email"
	"github.com/mark8ly/marketplace-api/internal/subscription"
	"github.com/mark8ly/marketplace-api/internal/subscription/dunning"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

// recordingMailer implements email.Client and records every Send call.
type recordingMailer struct {
	mu    sync.Mutex
	Calls []recordedCall
}

type recordedCall struct {
	Template email.TemplateID
	To       string
	Data     map[string]any
}

func (m *recordingMailer) Send(_ context.Context, template email.TemplateID, to string, data map[string]any) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Calls = append(m.Calls, recordedCall{Template: template, To: to, Data: data})
	return nil
}

func (m *recordingMailer) callCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.Calls)
}

// fixedClock returns a clock function that always returns t.
func fixedClock(t time.Time) func() time.Time { return func() time.Time { return t } }

// TestE2E_FullLadder_PastDueToPendingDelete walks the full dunning ladder:
// past_due → expired → store_closed → pending_hard_delete using backdated
// audit_logs rows to simulate elapsed time at each step.
func TestE2E_FullLadder_PastDueToPendingDelete(t *testing.T) {
	db := testdb.NewDB(t, "payment_action_reminders", "audit_logs", "store_subscriptions")
	ctx := context.Background()

	storeID := uuid.New()
	tenantID := uuid.New()
	seedStore(t, db, tenantID, storeID)

	// Seed the subscription as active.
	sub := subscription.StoreSubscription{
		ID:               uuid.New(),
		TenantID:         tenantID,
		StoreID:          storeID,
		StripeCustomerID: "cus_e2e_ladder",
		Plan:             subscription.PlanStarter,
		Status:           subscription.StatusActive,
	}
	if err := db.Create(&sub).Error; err != nil {
		t.Fatalf("seed sub: %v", err)
	}

	// --- Step 1: transition active → past_due, backdate to 9 days ago ---
	nineDaysAgo := time.Now().UTC().Add(-9 * 24 * time.Hour)

	if err := db.Exec(`
		UPDATE store_subscriptions
		SET status = ?, updated_at = ?
		WHERE store_id = ?`,
		string(subscription.StatusPastDue), nineDaysAgo, storeID,
	).Error; err != nil {
		t.Fatalf("set past_due: %v", err)
	}

	auditPastDue := audit.Entry{
		ID:           uuid.New(),
		TenantID:     tenantID,
		StoreID:      &storeID,
		ActorType:    audit.ActorSystem,
		Action:       "subscription.state_transition",
		ResourceType: "subscription",
		Status:       audit.StatusSuccess,
		Severity:     audit.SeverityWarning,
		Metadata:     audit.Metadata{"from_status": "active", "to_status": "past_due"},
		CreatedAt:    nineDaysAgo,
	}
	if err := db.Create(&auditPastDue).Error; err != nil {
		t.Fatalf("seed audit past_due: %v", err)
	}

	// --- Step 2: run ladder at "today"; past_due (9d) → expired ---
	now := time.Now().UTC()
	ladder := dunning.NewStepDailyLadder(db, nil, slog.Default(), nil, fixedClock(now))
	if err := ladder.Run(ctx); err != nil {
		t.Fatalf("ladder run 1 (past_due→expired): %v", err)
	}

	var afterStep1 subscription.StoreSubscription
	if err := db.Where("store_id = ?", storeID).First(&afterStep1).Error; err != nil {
		t.Fatalf("reload after step 1: %v", err)
	}
	if afterStep1.Status != subscription.StatusExpired {
		t.Fatalf("step 1: expected expired, got %q", afterStep1.Status)
	}

	// --- Step 3: backdate the expired audit row to 15 days ago, run ladder → store_closed ---
	fifteenDaysAgo := now.Add(-15 * 24 * time.Hour)
	if err := db.Exec(`
		UPDATE audit_logs
		SET created_at = ?
		WHERE store_id = ?
		  AND action = 'subscription.state_transition'
		  AND metadata->>'to_status' = 'expired'`,
		fifteenDaysAgo, storeID,
	).Error; err != nil {
		t.Fatalf("backdate expired audit: %v", err)
	}

	ladder2 := dunning.NewStepDailyLadder(db, nil, slog.Default(), nil, fixedClock(now))
	if err := ladder2.Run(ctx); err != nil {
		t.Fatalf("ladder run 2 (expired→store_closed): %v", err)
	}

	var afterStep2 subscription.StoreSubscription
	if err := db.Where("store_id = ?", storeID).First(&afterStep2).Error; err != nil {
		t.Fatalf("reload after step 2: %v", err)
	}
	if afterStep2.Status != subscription.StatusStoreClosed {
		t.Fatalf("step 2: expected store_closed, got %q", afterStep2.Status)
	}

	// --- Step 4: backdate the store_closed audit row to 91 days ago, run ladder → pending_hard_delete ---
	ninetyOneDaysAgo := now.Add(-91 * 24 * time.Hour)
	if err := db.Exec(`
		UPDATE audit_logs
		SET created_at = ?
		WHERE store_id = ?
		  AND action = 'subscription.state_transition'
		  AND metadata->>'to_status' = 'store_closed'`,
		ninetyOneDaysAgo, storeID,
	).Error; err != nil {
		t.Fatalf("backdate store_closed audit: %v", err)
	}

	ladder3 := dunning.NewStepDailyLadder(db, nil, slog.Default(), nil, fixedClock(now))
	if err := ladder3.Run(ctx); err != nil {
		t.Fatalf("ladder run 3 (store_closed→pending_hard_delete): %v", err)
	}

	var afterStep3 subscription.StoreSubscription
	if err := db.Where("store_id = ?", storeID).First(&afterStep3).Error; err != nil {
		t.Fatalf("reload after step 3: %v", err)
	}
	if afterStep3.Status != subscription.StatusPendingHardDelete {
		t.Fatalf("step 3: expected pending_hard_delete, got %q", afterStep3.Status)
	}
}

// testCounter is a simple CounterIncrementer stub that records Inc calls.
type testCounter struct {
	mu    sync.Mutex
	count int
}

func (c *testCounter) Inc() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.count++
}

func (c *testCounter) value() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.count
}

// TestE2E_RefundWindow_BlocksPastDueToExpired verifies that a past_due
// subscription whose FirstChargeAt is within the 14-day refund window is NOT
// advanced to expired, and the suppressed counter is incremented.
func TestE2E_RefundWindow_BlocksPastDueToExpired(t *testing.T) {
	db := testdb.NewDB(t, "audit_logs", "store_subscriptions")
	ctx := context.Background()

	storeID := uuid.New()
	tenantID := uuid.New()
	seedStore(t, db, tenantID, storeID)

	// FirstChargeAt is 3 days ago — within the 14-day refund window.
	threeDaysAgo := time.Now().UTC().Add(-3 * 24 * time.Hour)
	nineDaysAgo := time.Now().UTC().Add(-9 * 24 * time.Hour)

	sub := subscription.StoreSubscription{
		ID:               uuid.New(),
		TenantID:         tenantID,
		StoreID:          storeID,
		StripeCustomerID: "cus_refund_window",
		Plan:             subscription.PlanStarter,
		Status:           subscription.StatusPastDue,
		FirstChargeAt:    &threeDaysAgo,
		UpdatedAt:        nineDaysAgo,
		CreatedAt:        nineDaysAgo,
	}
	if err := db.Create(&sub).Error; err != nil {
		t.Fatalf("seed sub: %v", err)
	}

	// Seed audit row so statusEnteredAt returns nineDaysAgo (9 days ≥ threshold of 8).
	auditEntry := audit.Entry{
		ID:           uuid.New(),
		TenantID:     tenantID,
		StoreID:      &storeID,
		ActorType:    audit.ActorSystem,
		Action:       "subscription.state_transition",
		ResourceType: "subscription",
		Status:       audit.StatusSuccess,
		Severity:     audit.SeverityWarning,
		Metadata:     audit.Metadata{"from_status": "active", "to_status": "past_due"},
		CreatedAt:    nineDaysAgo,
	}
	if err := db.Create(&auditEntry).Error; err != nil {
		t.Fatalf("seed audit: %v", err)
	}

	counter := &testCounter{}
	now := time.Now().UTC()
	ladder := dunning.NewStepDailyLadder(db, nil, slog.Default(), counter, fixedClock(now))
	if err := ladder.Run(ctx); err != nil {
		t.Fatalf("Run: %v", err)
	}

	// Status must remain past_due — refund window guard should block transition.
	var refreshed subscription.StoreSubscription
	if err := db.Where("store_id = ?", storeID).First(&refreshed).Error; err != nil {
		t.Fatalf("reload: %v", err)
	}
	if refreshed.Status != subscription.StatusPastDue {
		t.Fatalf("expected status past_due (refund window guard), got %q", refreshed.Status)
	}

	// The suppressed counter must have been incremented exactly once.
	if got := counter.value(); got != 1 {
		t.Fatalf("expected suppressed counter = 1, got %d", got)
	}
}
