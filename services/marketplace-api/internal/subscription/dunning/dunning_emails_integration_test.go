//go:build integration

package dunning_test

import (
	"context"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/audit"
	"github.com/mark8ly/marketplace-api/internal/email"
	"github.com/mark8ly/marketplace-api/internal/emailtemplates"
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
	// Mirror the production client's contract: it refuses an
	// undeliverable recipient and never reports success for mail it
	// did not send. A double that skips this would let the cron pass
	// tests the real client would fail.
	if err := email.ValidateRecipient(to); err != nil {
		return err
	}
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
	db := testdb.NewDB(t, "audit_logs", "store_subscriptions", "billing_email_sends")

	now := time.Now().UTC()
	fiveDaysAgo := now.AddDate(0, 0, -5)
	sevenDaysAgo := now.AddDate(0, 0, -7)

	// Sub A: entered past_due 5 days ago.
	storeA := uuid.New()
	tenantA := uuid.New()
	seedStore(t, db, tenantA, storeA)
	emailA := "merchant-a@example.com"
	subA := subscription.StoreSubscription{
		ID:               uuid.New(),
		TenantID:         tenantA,
		StoreID:          storeA,
		StripeCustomerID: "cus_a",
		Plan:             subscription.PlanStarter,
		Status:           subscription.StatusPastDue,
		Email:            &emailA,
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
	seedStore(t, db, tenantB, storeB)
	emailB := "merchant-b@example.com"
	subB := subscription.StoreSubscription{
		ID:               uuid.New(),
		TenantID:         tenantB,
		StoreID:          storeB,
		StripeCustomerID: "cus_b",
		Plan:             subscription.PlanStarter,
		Status:           subscription.StatusPastDue,
		Email:            &emailB,
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
	db := testdb.NewDB(t, "audit_logs", "store_subscriptions", "billing_email_sends")

	now := time.Now().UTC()
	fiveDaysAgo := now.AddDate(0, 0, -5)

	storeID := uuid.New()
	tenantID := uuid.New()
	seedStore(t, db, tenantID, storeID)
	emailRecovered := "merchant-recovered@example.com"
	sub := subscription.StoreSubscription{
		ID:               uuid.New(),
		TenantID:         tenantID,
		StoreID:          storeID,
		StripeCustomerID: "cus_recovered",
		Plan:             subscription.PlanStarter,
		Status:           subscription.StatusActive, // recovered
		Email:            &emailRecovered,
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

// stubClient records recipients and can fail on demand.
type stubClient struct {
	sent []string
	err  error
}

func (c *stubClient) Send(_ context.Context, _ email.TemplateID, to string, _ map[string]any) error {
	// Mirror the production client's contract: it refuses an
	// undeliverable recipient and never reports success for mail it
	// did not send. A double that skips this would let the cron pass
	// tests the real client would fail.
	if err := email.ValidateRecipient(to); err != nil {
		return err
	}
	if c.err != nil {
		return c.err
	}
	c.sent = append(c.sent, to)
	return nil
}

// stubVec / stubSkip count increments by label.
type stubVec struct{ n map[string]int }

func (s *stubVec) WithDay(day string) dunning.CounterIncrementer {
	if s.n == nil {
		s.n = map[string]int{}
	}
	return stubInc{s.n, day}
}

type stubSkip struct{ n map[string]int }

func (s *stubSkip) WithTemplateReason(template, reason string) dunning.CounterIncrementer {
	if s.n == nil {
		s.n = map[string]int{}
	}
	return stubInc{s.n, template + "/" + reason}
}

type stubInc struct {
	n   map[string]int
	key string
}

func (s stubInc) Inc() { s.n[s.key]++ }

func TestDunning_UndeliverableCountsSkippedNotSent(t *testing.T) {
	db := testdb.NewDB(t, "store_subscriptions", "stores", "audit_logs", "billing_email_sends")
	now := time.Now().UTC()

	placeholder := "billing+7f3a@mark8ly.local"
	seedPastDueSubscription(t, db, now.AddDate(0, 0, -5), &placeholder)

	client := &stubClient{}
	sent, skipped := &stubVec{}, &stubSkip{}
	cron := dunning.NewSendDunningEmails(db, client, nil, sent, func() time.Time { return now }).
		WithSkipCounter(skipped)

	require.NoError(t, cron.Run(context.Background()))

	require.Empty(t, client.sent, "mailed a .local address")
	require.Zero(t, sent.n["day_5"], "sent counter incremented for mail never sent — the #381 lie")
	require.Equal(t, 1, skipped.n["dunning_day_5/placeholder_address"])
}

func TestDunning_SecondRunSameDayDoesNotResend(t *testing.T) {
	db := testdb.NewDB(t, "store_subscriptions", "stores", "audit_logs", "billing_email_sends")
	now := time.Now().UTC()

	addr := "merchant@example.com"
	seedPastDueSubscription(t, db, now.AddDate(0, 0, -5), &addr)

	client := &stubClient{}
	newCron := func() *dunning.SendDunningEmails {
		return dunning.NewSendDunningEmails(db, client, nil, &stubVec{}, func() time.Time { return now })
	}

	require.NoError(t, newCron().Run(context.Background()))
	require.Len(t, client.sent, 1, "first run should send exactly once")

	require.NoError(t, newCron().Run(context.Background()))
	require.Len(t, client.sent, 1, "second run re-sent — duplicate dunning mail")
}

// captureSender records every Message handed to it — a tiny email.Sender
// double used to prove the REAL email.templateClient's contract (recipient
// validation, error classification) end-to-end through the dunning cron,
// rather than trusting a hand-rolled client double to imitate it correctly.
type captureSender struct {
	mu   sync.Mutex
	msgs []email.Message
}

func (s *captureSender) Send(_ context.Context, msg email.Message) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.msgs = append(s.msgs, msg)
	return nil
}

// TestDunning_RealClientRefusesPlaceholderAddress wires the production
// email.Client (NewTemplateClient) instead of a test double, so this test
// fails if recipient validation is ever removed from the real client — a
// double-only test wouldn't catch that regression.
func TestDunning_RealClientRefusesPlaceholderAddress(t *testing.T) {
	db := testdb.NewDB(t, "audit_logs", "store_subscriptions", "billing_email_sends")
	now := time.Now().UTC()

	placeholder := "billing+7f3a@mark8ly.local"
	seedPastDueSubscription(t, db, now.AddDate(0, 0, -5), &placeholder)

	// The real template client over a capturing transport — not a double.
	loader := emailtemplates.NewLoader(nil)
	email.RegisterFallbacks(loader)
	sender := &captureSender{}
	client := email.NewTemplateClient(loader, sender, "noreply@mark8ly.com", nil)

	sent, skipped := &stubVec{}, &stubSkip{}
	cron := dunning.NewSendDunningEmails(db, client, nil, sent, func() time.Time { return now }).
		WithSkipCounter(skipped)

	require.NoError(t, cron.Run(context.Background()))

	require.Empty(t, sender.msgs, "a .local address reached the transport")
	require.Zero(t, sent.n["day_5"], "counted a delivery that never happened")
	require.Equal(t, 1, skipped.n["dunning_day_5/placeholder_address"])
}
