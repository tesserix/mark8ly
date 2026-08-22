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

// capturingClient records every Send call for assertion.
type capturingClient struct {
	mu    sync.Mutex
	sends []capturedSend
}

type capturedSend struct {
	Template email.TemplateID
	To       string
}

func (c *capturingClient) Send(_ context.Context, tmpl email.TemplateID, to string, _ map[string]any) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.sends = append(c.sends, capturedSend{Template: tmpl, To: to})
	return nil
}

func (c *capturingClient) count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.sends)
}

// TestSendDunningEmails_SendsOnDay5AndDay7 verifies that a past_due sub with
// an audit entry 5 days ago receives the day-5 email, and one with a 7-day
// audit entry receives the day-7 email.
func TestSendDunningEmails_SendsOnDay5AndDay7(t *testing.T) {
	db := testdb.NewDB(t, "audit_logs", "store_subscriptions")

	now := time.Now().UTC()
	fiveDaysAgo := now.AddDate(0, 0, -5)
	sevenDaysAgo := now.AddDate(0, 0, -7)

	// Sub A: entered past_due 5 days ago.
	storeA := uuid.New()
	tenantA := uuid.New()
	subA := subscription.StoreSubscription{
		ID:               uuid.New(),
		TenantID:         tenantA,
		StoreID:          storeA,
		StripeCustomerID: "cus_a",
		Plan:             subscription.PlanStarter,
		Status:           subscription.StatusPastDue,
	}
	if err := db.Create(&subA).Error; err != nil {
		t.Fatalf("seed subA: %v", err)
	}
	auditA := audit.Entry{
		ID:           uuid.New(),
		TenantID:     tenantA,
		StoreID:      &storeA,
		ActorType:    audit.ActorSystem,
		Action:       "subscription.state_transition",
		ResourceType: "subscription",
		Status:       audit.StatusSuccess,
		Severity:     audit.SeverityInfo,
		Metadata:     audit.Metadata{"to_status": "past_due", "from_status": "active"},
		CreatedAt:    fiveDaysAgo,
	}
	if err := db.Create(&auditA).Error; err != nil {
		t.Fatalf("seed auditA: %v", err)
	}

	// Sub B: entered past_due 7 days ago.
	storeB := uuid.New()
	tenantB := uuid.New()
	subB := subscription.StoreSubscription{
		ID:               uuid.New(),
		TenantID:         tenantB,
		StoreID:          storeB,
		StripeCustomerID: "cus_b",
		Plan:             subscription.PlanStarter,
		Status:           subscription.StatusPastDue,
	}
	if err := db.Create(&subB).Error; err != nil {
		t.Fatalf("seed subB: %v", err)
	}
	auditB := audit.Entry{
		ID:           uuid.New(),
		TenantID:     tenantB,
		StoreID:      &storeB,
		ActorType:    audit.ActorSystem,
		Action:       "subscription.state_transition",
		ResourceType: "subscription",
		Status:       audit.StatusSuccess,
		Severity:     audit.SeverityInfo,
		Metadata:     audit.Metadata{"to_status": "past_due", "from_status": "active"},
		CreatedAt:    sevenDaysAgo,
	}
	if err := db.Create(&auditB).Error; err != nil {
		t.Fatalf("seed auditB: %v", err)
	}

	client := &capturingClient{}
	cron := dunning.NewSendDunningEmails(db, client, slog.Default(), nil, func() time.Time { return now })
	if err := cron.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if got := client.count(); got != 2 {
		t.Fatalf("expected 2 sends (day-5 + day-7), got %d", got)
	}
}

// TestSendDunningEmails_NoEmailIfSubNoLongerPastDue verifies that a sub that
// entered past_due 5 days ago but is now active receives no email.
func TestSendDunningEmails_NoEmailIfSubNoLongerPastDue(t *testing.T) {
	db := testdb.NewDB(t, "audit_logs", "store_subscriptions")

	now := time.Now().UTC()
	fiveDaysAgo := now.AddDate(0, 0, -5)

	storeID := uuid.New()
	tenantID := uuid.New()
	sub := subscription.StoreSubscription{
		ID:               uuid.New(),
		TenantID:         tenantID,
		StoreID:          storeID,
		StripeCustomerID: "cus_recovered",
		Plan:             subscription.PlanStarter,
		Status:           subscription.StatusActive, // recovered
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
		Severity:     audit.SeverityInfo,
		Metadata:     audit.Metadata{"to_status": "past_due", "from_status": "active"},
		CreatedAt:    fiveDaysAgo,
	}
	if err := db.Create(&auditEntry).Error; err != nil {
		t.Fatalf("seed audit: %v", err)
	}

	client := &capturingClient{}
	cron := dunning.NewSendDunningEmails(db, client, slog.Default(), nil, func() time.Time { return now })
	if err := cron.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if got := client.count(); got != 0 {
		t.Fatalf("expected 0 sends (sub recovered), got %d", got)
	}
}
