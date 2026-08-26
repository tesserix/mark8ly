//go:build integration

package lifecycle_test

import (
	"context"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/email"
	"github.com/mark8ly/marketplace-api/internal/emailtemplates"
	"github.com/mark8ly/marketplace-api/internal/subscription"
	"github.com/mark8ly/marketplace-api/internal/subscription/lifecycle"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

// stubClient mirrors the production client's contract: it refuses an
// undeliverable recipient and never reports success for mail it did not
// send. A double that skips this would let the cron pass tests the real
// client would fail.
type stubClient struct {
	sent []string
	err  error
}

func (c *stubClient) Send(_ context.Context, _ email.TemplateID, to string, _ map[string]any) error {
	if err := email.ValidateRecipient(to); err != nil {
		return err
	}
	if c.err != nil {
		return c.err
	}
	c.sent = append(c.sent, to)
	return nil
}

type stubSkip struct{ n map[string]int }

func (s *stubSkip) WithTemplateReason(template, reason string) lifecycle.CounterIncrementer {
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

// captureSender records every Message handed to it — a tiny email.Sender
// double used to prove the REAL email.templateClient's contract (recipient
// validation, error classification) end-to-end through the win-back cron,
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

// seedStore inserts a minimal stores row for (tenantID, storeID) so that a
// store_subscriptions row referencing storeID satisfies
// store_subscriptions_store_id_fkey. Copied from the dunning package's
// helper of the same name — lifecycle has no shared test helper package to
// draw from and neither package imports the other's test files.
func seedStore(t *testing.T, db *gorm.DB, tenantID, storeID uuid.UUID) {
	t.Helper()

	slug := "tst-" + strings.ReplaceAll(storeID.String(), "-", "")[:20]

	err := db.Exec(
		`INSERT INTO stores (id, tenant_id, slug, name, country_code, currency_code, timezone, status, storefront_customer_portal_secret)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, encode(gen_random_bytes(32), 'hex'))`,
		storeID, tenantID, slug, "Test Store", "IE", "EUR", "Europe/Dublin", "active",
	).Error
	if err != nil {
		t.Fatalf("seedStore: insert stores row: %v", err)
	}

	t.Cleanup(func() {
		db.Exec("DELETE FROM stores WHERE id = ?", storeID)
	})
}

// seedExpired inserts an expired subscription whose updated_at sits inside
// the 30-31 day win-back window, then forces updated_at past GORM's
// autoupdate (which would otherwise stamp now()).
func seedExpired(t *testing.T, db *gorm.DB, now time.Time, addr *string) subscription.StoreSubscription {
	t.Helper()

	storeID := uuid.New()
	tenantID := uuid.New()
	seedStore(t, db, tenantID, storeID)

	sub := subscription.StoreSubscription{
		TenantID:         tenantID,
		StoreID:          storeID,
		StripeCustomerID: "cus_" + uuid.NewString()[:12],
		Status:           subscription.StatusExpired,
		Email:            addr,
	}
	require.NoError(t, db.Create(&sub).Error)
	require.NoError(t, db.Exec(
		`UPDATE store_subscriptions SET updated_at = ? WHERE id = ?`,
		now.Add(-30*24*time.Hour-time.Hour), sub.ID).Error)
	return sub
}

func TestWinBack_UndeliverableIsSkippedNotSent(t *testing.T) {
	db := testdb.NewDB(t, "store_subscriptions", "stores", "billing_email_sends")
	now := time.Now().UTC()

	placeholder := "billing+7f3a@mark8ly.local"
	seedExpired(t, db, now, &placeholder)

	client := &stubClient{}
	skipped := &stubSkip{}
	cron := lifecycle.NewWinBackCron(db, client, nil, func() time.Time { return now }).
		WithSkipCounter(skipped)

	require.NoError(t, cron.Run(context.Background()))

	require.Empty(t, client.sent, "mailed a .local address")
	require.Equal(t, 1, skipped.n["win_back_day30/placeholder_address"])
}

func TestWinBack_SecondRunSameDayDoesNotResend(t *testing.T) {
	db := testdb.NewDB(t, "store_subscriptions", "stores", "billing_email_sends")
	now := time.Now().UTC()

	addr := "merchant@example.com"
	seedExpired(t, db, now, &addr)

	client := &stubClient{}
	newCron := func() *lifecycle.WinBackCron {
		return lifecycle.NewWinBackCron(db, client, nil, func() time.Time { return now })
	}

	require.NoError(t, newCron().Run(context.Background()))
	require.Len(t, client.sent, 1, "first run should send exactly once")

	require.NoError(t, newCron().Run(context.Background()))
	require.Len(t, client.sent, 1, "second run re-sent — duplicate win-back mail")
}

// TestWinBack_RealClientRefusesPlaceholderAddress wires the production
// email.Client (NewTemplateClient) instead of a test double, so this test
// fails if recipient validation is ever removed from the real client — a
// double-only test wouldn't catch that regression.
func TestWinBack_RealClientRefusesPlaceholderAddress(t *testing.T) {
	db := testdb.NewDB(t, "store_subscriptions", "stores", "billing_email_sends")
	now := time.Now().UTC()

	placeholder := "billing+7f3a@mark8ly.local"
	seedExpired(t, db, now, &placeholder)

	loader := emailtemplates.NewLoader(nil)
	email.RegisterFallbacks(loader)
	sender := &captureSender{}
	client := email.NewTemplateClient(loader, sender, "noreply@mark8ly.com", nil)

	skipped := &stubSkip{}
	cron := lifecycle.NewWinBackCron(db, client, slog.Default(), func() time.Time { return now }).
		WithSkipCounter(skipped)

	require.NoError(t, cron.Run(context.Background()))

	require.Empty(t, sender.msgs, "a .local address reached the transport")
	require.Equal(t, 1, skipped.n["win_back_day30/placeholder_address"])
}
