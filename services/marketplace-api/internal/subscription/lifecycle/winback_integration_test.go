//go:build integration

package lifecycle_test

import (
	"context"
	"errors"
	"log/slog"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/email"
	"github.com/mark8ly/marketplace-api/internal/emailtemplates"
	"github.com/mark8ly/marketplace-api/internal/promo"
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
// TestWinBack_OverlappingRunsAcrossClockDoNotResend uses two DIFFERENT
// clocks (12h apart) whose windows both cover the same seeded row, unlike
// TestWinBack_SecondRunSameDayDoesNotResend which reuses one clock and so
// cannot distinguish a row-anchored period key from a wall-clock one. Under
// a wall-clock key (`windowStart.Format(...)`), the two runs' windowStart
// values fall on different calendar dates here, so the row claims under two
// different period keys and gets mailed twice — the bug this task exists to
// close. Under a row-anchored key the two runs agree on the same period key
// and the second claim loses.
func TestWinBack_OverlappingRunsAcrossClockDoNotResend(t *testing.T) {
	db := testdb.NewDB(t, "store_subscriptions", "stores", "billing_email_sends")
	// Fixed anchor (not time.Now()) so the two windowStart values are
	// guaranteed to land on different calendar dates regardless of when
	// this test runs.
	now := time.Date(2026, 8, 20, 23, 0, 0, 0, time.UTC)

	addr := "merchant@example.com"
	seedExpired(t, db, now, &addr)

	client := &stubClient{}
	firstRunNow := now
	secondRunNow := now.Add(12 * time.Hour)

	firstCron := lifecycle.NewWinBackCron(db, client, nil, func() time.Time { return firstRunNow })
	require.NoError(t, firstCron.Run(context.Background()))
	require.Len(t, client.sent, 1, "first run should send exactly once")

	secondCron := lifecycle.NewWinBackCron(db, client, nil, func() time.Time { return secondRunNow })
	require.NoError(t, secondCron.Run(context.Background()))
	require.Len(t, client.sent, 1, "second run (different clock, overlapping window) re-sent — duplicate win-back mail")
}

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

func TestWinBack_UndeliverableReleasesClaimSoRetryCanSend(t *testing.T) {
	db := testdb.NewDB(t, "store_subscriptions", "stores", "billing_email_sends")
	now := time.Now().UTC()

	placeholder := "billing+7f3a@mark8ly.local"
	sub := seedExpired(t, db, now, &placeholder)

	client := &stubClient{}
	run := func() *lifecycle.WinBackCron {
		return lifecycle.NewWinBackCron(db, client, nil, func() time.Time { return now }).
			WithSkipCounter(&stubSkip{})
	}
	require.NoError(t, run().Run(context.Background()))
	require.Empty(t, client.sent, "mailed an undeliverable address")

	var claims int64
	require.NoError(t, db.Raw(`SELECT count(*) FROM billing_email_sends`).Scan(&claims).Error)
	require.EqualValues(t, 0, claims, "claim was burned; a later run can never deliver this notice")

	good := "merchant@example.com"
	require.NoError(t, db.Exec(`UPDATE store_subscriptions SET email = ? WHERE id = ?`, good, sub.ID).Error)
	require.NoError(t, run().Run(context.Background()))
	require.Equal(t, []string{good}, client.sent, "retry after backfill did not deliver")
}

func TestWinBack_TransportFailureKeepsClaimBurned(t *testing.T) {
	db := testdb.NewDB(t, "store_subscriptions", "stores", "billing_email_sends")
	now := time.Now().UTC()

	addr := "merchant@example.com"
	seedExpired(t, db, now, &addr)

	client := &stubClient{err: errors.New("sendgrid 503")}
	cron := lifecycle.NewWinBackCron(db, client, nil, func() time.Time { return now }).
		WithSkipCounter(&stubSkip{})
	require.NoError(t, cron.Run(context.Background()))

	var claims int64
	require.NoError(t, db.Raw(`SELECT count(*) FROM billing_email_sends`).Scan(&claims).Error)
	require.EqualValues(t, 1, claims, "transport failure released the claim; a retry could duplicate")
}

// ---------------------------------------------------------------------------
// #727 — the offer the email states must be one that exists.
// ---------------------------------------------------------------------------

// winBackTables is the cleanup set for the promo-aware tests. promo_codes and
// promo_redemptions are added because the offer check reads both.
var winBackTables = []string{
	"promo_redemptions", "promo_codes", "store_subscriptions", "stores", "billing_email_sends",
}

// seedWinBackCode inserts the console-authored row the win-back offers. The
// shape mirrors what internal/billing/consolepromo writes: a percentage
// discount in basis points, bounded by max_duration_months.
func seedWinBackCode(t *testing.T, db *gorm.DB, mutate func(*promo.PromoCode)) {
	t.Helper()
	typ := promo.DiscountTypePercentage
	val := 2000
	months := 6
	coupon := "co_test_winback"
	pc := &promo.PromoCode{
		Code:              lifecycle.WinBackPromoCode,
		StripeCouponID:    &coupon,
		DiscountType:      &typ,
		DiscountValue:     &val,
		MaxDurationMonths: &months,
		MaxPerEmail:       1,
		ValidFrom:         time.Now().UTC().Add(-24 * time.Hour),
		CreatedBy:         "console:promo-catalog",
	}
	if mutate != nil {
		mutate(pc)
	}
	require.NoError(t, promo.NewRepository().Create(context.Background(), db, pc))
}

// onStarterUSD moves a seeded row onto a real paid plan and currency, which
// is what a lapsed paying merchant looks like. Uses a raw UPDATE so GORM does
// not restamp updated_at and push the row out of the 30-day window.
func onStarterUSD(t *testing.T, db *gorm.DB, id uuid.UUID) {
	t.Helper()
	require.NoError(t, db.Exec(
		`UPDATE store_subscriptions
		    SET plan = 'starter', billing_currency = 'usd',
		        subscription_period = 'monthly', price_tier = 'developed'
		  WHERE id = ?`, id).Error)
}

// winBackMessage runs the cron with the REAL email client and promo service
// and returns the single message that reached the transport.
func winBackMessage(t *testing.T, db *gorm.DB, now time.Time) email.Message {
	t.Helper()
	loader := emailtemplates.NewLoader(nil)
	email.RegisterFallbacks(loader)
	sender := &captureSender{}
	client := email.NewTemplateClient(loader, sender, "noreply@mark8ly.com", nil)

	cron := lifecycle.NewWinBackCron(db, client, slog.Default(), func() time.Time { return now }).
		WithPromo(promo.NewService(db, promo.NewRepository(), nil, slog.Default()))

	require.NoError(t, cron.Run(context.Background()))
	require.Len(t, sender.msgs, 1, "expected exactly one win-back message")
	return sender.msgs[0]
}

// requireNoDiscountClaim asserts the rendered message promises nothing. This
// is the property #727 is about: the day-30 mail quantified an offer that
// nothing could honour.
func requireNoDiscountClaim(t *testing.T, msg email.Message) {
	t.Helper()
	tag := regexp.MustCompile(`<[^>]*>`)
	parts := map[string]string{
		"subject": msg.Subject,
		"text":    msg.TextBody,
		"html":    tag.ReplaceAllString(msg.HTMLBody, " "),
	}
	for name, body := range parts {
		lower := strings.ToLower(body)
		for _, claim := range []string{"%", "discount", "off your", "promo", lifecycle.WinBackPromoCode} {
			require.NotContainsf(t, lower, strings.ToLower(claim),
				"%s promises %q with no code behind it: %s", name, claim, body)
		}
	}
	require.Contains(t, msg.TextBody, "Nothing has been deleted",
		"the offer-less win-back must still be worth sending")
}

// The state production is in today: the catalog holds no win-back code. The
// email goes out — a merchant a month past expiry does not know their
// catalogue survived — and states no discount.
func TestWinBack_NoCodeSendsWithoutAnOffer(t *testing.T) {
	db := testdb.NewDB(t, winBackTables...)
	now := time.Now().UTC()

	addr := "merchant@example.com"
	sub := seedExpired(t, db, now, &addr)
	onStarterUSD(t, db, sub.ID)

	msg := winBackMessage(t, db, now)
	require.Equal(t, string(email.TemplateWinBackNoOffer), msg.CustomArgs["kind"])
	requireNoDiscountClaim(t, msg)
}

// The mutation the honesty property is worth testing under: the code EXISTS,
// so a happy-path-only test would be green, but it is no longer redeemable.
// The email must fall back to stating nothing rather than naming a code that
// would be refused.
func TestWinBack_ExpiredCodeSendsWithoutAnOffer(t *testing.T) {
	db := testdb.NewDB(t, winBackTables...)
	now := time.Now().UTC()

	past := time.Now().UTC().Add(-time.Hour)
	seedWinBackCode(t, db, func(pc *promo.PromoCode) { pc.ValidUntil = &past })

	addr := "merchant@example.com"
	sub := seedExpired(t, db, now, &addr)
	onStarterUSD(t, db, sub.ID)

	msg := winBackMessage(t, db, now)
	require.Equal(t, string(email.TemplateWinBackNoOffer), msg.CustomArgs["kind"])
	requireNoDiscountClaim(t, msg)
}

// The same mutation from the other direction: the code is live, but this
// merchant's address has already used its one permitted redemption.
func TestWinBack_AlreadyRedeemedSendsWithoutAnOffer(t *testing.T) {
	db := testdb.NewDB(t, winBackTables...)
	now := time.Now().UTC()

	seedWinBackCode(t, db, nil)
	addr := "merchant@example.com"
	sub := seedExpired(t, db, now, &addr)
	onStarterUSD(t, db, sub.ID)

	repo := promo.NewRepository()
	pc, err := repo.GetByCode(context.Background(), db, lifecycle.WinBackPromoCode)
	require.NoError(t, err)
	require.NoError(t, repo.CreateRedemption(context.Background(), db, &promo.Redemption{
		PromoCodeID:    pc.ID,
		StoreID:        uuid.New(),
		SubscriptionID: uuid.New(),
		Email:          addr,
		RedeemedAt:     time.Now().UTC(),
	}))

	msg := winBackMessage(t, db, now)
	require.Equal(t, string(email.TemplateWinBackNoOffer), msg.CustomArgs["kind"])
	requireNoDiscountClaim(t, msg)
}

// And the offer itself: a redeemable code produces an email that names it and
// quotes the row's own terms.
func TestWinBack_RedeemableCodeIsNamedInTheEmail(t *testing.T) {
	db := testdb.NewDB(t, winBackTables...)
	now := time.Now().UTC()

	seedWinBackCode(t, db, nil)
	addr := "merchant@example.com"
	sub := seedExpired(t, db, now, &addr)
	onStarterUSD(t, db, sub.ID)

	msg := winBackMessage(t, db, now)
	require.Equal(t, string(email.TemplateWinBack), msg.CustomArgs["kind"])
	require.Contains(t, msg.TextBody, lifecycle.WinBackPromoCode)
	require.Contains(t, msg.TextBody, "20% off your first 6 months")
	require.Contains(t, msg.Subject, "20% off for 6 months")
}

// Asking whether a code is offerable must not consume it. max_per_email is 1,
// so a redemption recorded at email time would leave the merchant holding a
// code the apply-promo endpoint refuses.
func TestWinBack_OfferCheckRecordsNoRedemption(t *testing.T) {
	db := testdb.NewDB(t, winBackTables...)
	now := time.Now().UTC()

	seedWinBackCode(t, db, nil)
	addr := "merchant@example.com"
	sub := seedExpired(t, db, now, &addr)
	onStarterUSD(t, db, sub.ID)

	winBackMessage(t, db, now)

	var n int64
	require.NoError(t, db.Raw(`SELECT count(*) FROM promo_redemptions`).Scan(&n).Error)
	require.EqualValues(t, 0, n, "the win-back redeemed the code on the merchant's behalf")
}
