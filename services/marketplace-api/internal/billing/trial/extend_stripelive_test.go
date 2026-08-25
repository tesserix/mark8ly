//go:build stripelive

// Package trial_test — extend_stripelive_test.go
//
// # WHY THIS FILE EXISTS
//
// Extend's central design decision (#358) is that a `SELECT ... FOR UPDATE`
// row lock on store_subscriptions is held ACROSS the Stripe network call,
// inside one transaction: validate -> Stripe -> local write -> commit. Every
// other test of Extend — including expiring_integration_test.go's own
// fixtures — drives it against a `fakeUpdater` that returns instantly. A
// fake, by construction, cannot exercise "hold a row lock across a real
// network round trip and then commit on the far side of it", because a fake
// has no network round trip. That is the one thing this file exists to
// prove: real Postgres + real Stripe test mode + one transaction, composed
// the way production actually runs it.
//
// # WHY IT IS ISOLATED BEHIND ITS OWN BUILD TAG
//
// `stripelive` is used NOWHERE else in this repository. That is deliberate:
// this file talks to the real Stripe network (test mode) and needs a real
// Postgres, a Stripe secret key, and can leave/clean up live Stripe objects.
// None of that belongs in `go build ./...`, `go test ./...`,
// `go vet -tags=integration ./...`, or CI — all of which must never compile
// this file. Only `-tags=stripelive` does.
//
// This file intentionally does NOT depend on expiring_integration_test.go's
// `seedExpiringRow`/`seedExpiringStore` helpers (those are gated behind
// `//go:build integration`, a DIFFERENT tag): a cross-build-tag dependency
// would mean `go vet -tags=stripelive` alone could never actually compile
// this file, defeating the one verification available without a database or
// a key. The seeding helpers below are self-contained duplicates of that
// same shape (store row, then a trialing store_subscriptions row) scoped
// only to this file.
//
// RUNNING IT
//
//	STRIPE_TEST_KEY=sk_test_... TEST_DATABASE_URL=postgres://... \
//	  go test -tags=stripelive ./internal/billing/trial/ -run TestExtend_RealStripeAndRealPostgres -v
package trial_test

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sdk "github.com/stripe/stripe-go/v82"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/mark8ly/marketplace-api/internal/billing/pricing"
	billingstripe "github.com/mark8ly/marketplace-api/internal/billing/stripe"
	"github.com/mark8ly/marketplace-api/internal/billing/trial"
	"github.com/mark8ly/marketplace-api/internal/subscription"
)

// realStripeAdapter adapts a real *billingstripe.Client to
// trial.StripeTrialUpdater. It is a deliberate duplicate of
// cmd/marketplace-api/main.go's unexported trialStripeAdapter — that type is
// package-private to cmd/marketplace-api and cannot be imported here, and
// this file must not modify production code to expose it. Keeping the same
// shape is the point: this test is verifying the composition production
// actually wires up, not a stand-in for it.
type realStripeAdapter struct{ c *billingstripe.Client }

func (a *realStripeAdapter) GetSubscription(ctx context.Context, id string) (*billingstripe.Subscription, error) {
	return billingstripe.GetSubscription(ctx, a.c, id)
}

func (a *realStripeAdapter) UpdateTrialEnd(ctx context.Context, in billingstripe.UpdateTrialEndParams) (*billingstripe.Subscription, error) {
	return billingstripe.UpdateTrialEnd(ctx, a.c, in)
}

// requireStep fails the test with the setup step's name prefixed onto the
// raw Stripe/DB error, rather than a bare error the next reader has to
// guess the origin of. Fix round 1 (F4): a prior failure here surfaced only
// as a raw Stripe 400 with no indication of which of the four setup calls
// produced it.
func requireStep(t *testing.T, step string, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("%s: %v", step, err)
	}
}

// seedStripeliveStore inserts a minimal stores row so a store_subscriptions
// row referencing storeID satisfies store_subscriptions_store_id_fkey.
// Self-contained copy of expiring_integration_test.go's seedExpiringStore —
// see the file doc comment for why this cannot simply call that helper.
func seedStripeliveStore(t *testing.T, db *gorm.DB, tenantID, storeID uuid.UUID) {
	t.Helper()

	slug := "sl-" + strings.ReplaceAll(storeID.String(), "-", "")[:20]

	err := db.Exec(
		`INSERT INTO stores (id, tenant_id, slug, name, country_code, currency_code, timezone, status, storefront_customer_portal_secret)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, encode(gen_random_bytes(32), 'hex'))`,
		storeID, tenantID, slug, "Stripelive Test Store", "IE", "EUR", "Europe/Dublin", "active",
	).Error
	require.NoError(t, err)

	t.Cleanup(func() {
		db.Exec("DELETE FROM stores WHERE id = ?", storeID)
	})
}

// TestExtend_RealStripeAndRealPostgres_HoldsLockAcrossNetworkCall is the one
// test in this file, by design (see the file doc comment) — a verification
// instrument, not a suite.
func TestExtend_RealStripeAndRealPostgres_HoldsLockAcrossNetworkCall(t *testing.T) {
	stripeKey := os.Getenv("STRIPE_TEST_KEY")
	dbURL := os.Getenv("TEST_DATABASE_URL")

	var missing []string
	if stripeKey == "" {
		missing = append(missing, "STRIPE_TEST_KEY")
	}
	if dbURL == "" {
		missing = append(missing, "TEST_DATABASE_URL")
	}
	if len(missing) > 0 {
		t.Skipf("skipping stripelive test: missing env var(s) %s (this test needs a real Stripe test-mode key AND a real Postgres to prove anything)", strings.Join(missing, ", "))
	}

	// REFUSE rather than skip on a key that isn't obviously test mode. A
	// silent skip here is indistinguishable from a pass; a Fatal on what
	// looks like a live key is the only safe default.
	if !strings.HasPrefix(stripeKey, "sk_test_") {
		t.Fatalf("STRIPE_TEST_KEY must start with sk_test_ (refusing to run against what does not look like a Stripe TEST-mode key); got a key with prefix %q", safePrefix(stripeKey))
	}

	ctx := context.Background()

	db, err := gorm.Open(postgres.Open(dbURL), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err, "open TEST_DATABASE_URL")
	t.Cleanup(func() {
		if sqlDB, derr := db.DB(); derr == nil {
			_ = sqlDB.Close()
		}
	})

	c := billingstripe.New(stripeKey)

	// storeID (and everything derived from it below) MUST be freshly random
	// per run, not a constant. Fix round 1 (F2): billingstripe derives
	// several idempotency keys straight from the store id —
	// CustomerIdempotencyKey(storeID), the product idempotency key below,
	// and CreateSubscription's SubscriptionIdempotencyKey(storeID, plan,
	// period). Stripe caches an idempotent response for 24h; a constant
	// storeID would make every rerun inside that window silently replay the
	// FIRST run's cached objects — including trial_end — rather than create
	// anything new, which would look like a pass while testing nothing.
	// uuid.New() is the source of that freshness; nothing below may
	// hardcode storeID's value.
	tenantID := uuid.New()
	storeID := uuid.New()
	seedStripeliveStore(t, db, tenantID, storeID)

	// runTag disambiguates the OTHER thing that collides across runs: the
	// Stripe Price's lookup_key. Fix round 1 (F1): pricing.MustGetDescriptor
	// returns the catalog's real descriptor, whose LookupKey is a FIXED
	// string ("mark8ly_starter_monthly_developed_v1") shared with
	// production's own bootstrap — the first run against any given Stripe
	// test account creates that Price and every subsequent run then fails
	// CreatePrice with "a price already uses that lookup key" (see the
	// round-1 failure log). storeID is already a fresh random uuid.New()
	// per run (see the comment above), so folding it into the lookup key is
	// enough on its own; the nanosecond timestamp is added only as a second,
	// independent source of uniqueness so this does not regress if storeID
	// generation ever changes.
	runTag := fmt.Sprintf("%s-%d", storeID.String(), time.Now().UnixNano())

	// --- Real Stripe customer, price, and TRIALING subscription -----------
	//
	// Built via our own billingstripe package (not raw HTTP): the point of
	// this file is to exercise what we ship.
	cust, err := billingstripe.CreateCustomer(ctx, c, billingstripe.CreateCustomerInput{
		StoreID:  storeID.String(),
		TenantID: tenantID.String(),
		Email:    fmt.Sprintf("stripelive-task8-%s@example.invalid", storeID.String()[:8]),
		Name:     "Stripelive Task 8 Test",
		Country:  "IE",
	})
	requireStep(t, "CreateCustomer", err)

	product, err := billingstripe.CreateProduct(ctx, c, "Stripelive Task 8 Test Product "+runTag, "starter",
		"stripelive-task8-product:"+runTag)
	requireStep(t, "CreateProduct", err)
	t.Cleanup(func() {
		// Fix round 1 (F3): every run now creates a uniquely-named product,
		// so the accumulation flagged after the first submission is worse
		// unless each run also deactivates its own. billingstripe has no
		// wrapper for this (checked product.go — CreateProduct/
		// FindProductByMetadata only), so this uses the raw SDK, same as
		// the subscription cancel below. Deactivating (not deleting) is
		// deliberate: Stripe test-mode products with prices attached
		// generally cannot be deleted, only archived via active=false.
		raw := sdk.NewClient(stripeKey)
		if _, cerr := raw.V1Products.Update(context.Background(), product.ID, &sdk.ProductUpdateParams{Active: sdk.Bool(false)}); cerr != nil {
			t.Logf("cleanup: failed to deactivate stripe test-mode product %s: %v", product.ID, cerr)
		}
	})

	// A hand-built descriptor, not pricing.MustGetDescriptor's catalog
	// value — see the runTag comment above for why the catalog's fixed
	// LookupKey cannot be reused here. Baseline/Tier/Plan/Period mirror the
	// catalog shape closely enough for CreatePrice's logic (single baseline
	// currency, TierDeveloped's per-currency-options loop over an empty
	// Options map is a no-op) without colliding with anything real.
	desc := pricing.PriceDescriptor{
		Plan:      pricing.PlanStarter,
		Period:    pricing.PeriodMonthly,
		Tier:      pricing.TierDeveloped,
		Baseline:  pricing.Amount{Currency: "usd", UnitAmountMinor: 999},
		Options:   map[string]pricing.Amount{},
		LookupKey: "stripelive_task8_" + runTag,
	}
	price, err := billingstripe.CreatePrice(ctx, c, product.ID, desc)
	requireStep(t, "CreatePrice", err)
	t.Cleanup(func() {
		// Same rationale as the product cleanup above: no billingstripe
		// wrapper exists (checked price.go), so this deactivates via the
		// raw SDK. A Price cannot be deleted once created, only archived.
		raw := sdk.NewClient(stripeKey)
		if _, cerr := raw.V1Prices.Update(context.Background(), price.ID, &sdk.PriceUpdateParams{Active: sdk.Bool(false)}); cerr != nil {
			t.Logf("cleanup: failed to deactivate stripe test-mode price %s: %v", price.ID, cerr)
		}
	})

	initialTrialEnd := time.Now().Add(48 * time.Hour).UTC()
	sdkSub, err := billingstripe.CreateSubscription(ctx, c, billingstripe.CreateSubscriptionInput{
		StoreID:    storeID.String(),
		Plan:       "starter",
		Period:     "monthly",
		CustomerID: cust.ID,
		PriceID:    price.ID,
		TrialEnd:   initialTrialEnd.Unix(),
	})
	requireStep(t, "CreateSubscription", err)
	require.Equal(t, "trialing", sdkSub.Status, "setup subscription must be trialing")

	// Cancel the Stripe test-mode subscription on the way out. This is
	// cleanup only — failing to cancel must not fail the test, only be
	// logged, since a leaked test-mode object is not this test's real
	// finding.
	t.Cleanup(func() {
		raw := sdk.NewClient(stripeKey)
		if _, cerr := raw.V1Subscriptions.Cancel(context.Background(), sdkSub.ID, nil); cerr != nil {
			t.Logf("cleanup: failed to cancel stripe test-mode subscription %s: %v", sdkSub.ID, cerr)
		}
	})

	// --- Seed the local row referencing the real Stripe subscription ------
	row := subscription.StoreSubscription{
		TenantID:             tenantID,
		StoreID:              storeID,
		StripeCustomerID:     cust.ID,
		StripeSubscriptionID: &sdkSub.ID,
		Status:               subscription.StatusTrialing,
		Plan:                 subscription.PlanTrial,
		SubscriptionPeriod:   subscription.PeriodMonthly,
		PriceTier:            subscription.PriceTierDeveloped,
		CreatedAt:            time.Now().UTC(),
	}
	require.NoError(t, db.Create(&row).Error, "seed store_subscriptions row")
	t.Cleanup(func() {
		db.Unscoped().Delete(&subscription.StoreSubscription{}, "id = ?", row.ID)
	})

	// Seed reminder rows that Extend must clear on a successful extension.
	require.NoError(t, db.Exec(
		`INSERT INTO trial_reminders (subscription_id, tenant_id, store_id, offset_key) VALUES (?, ?, ?, ?)`,
		row.ID, tenantID, storeID, "no_pm_t_minus_15",
	).Error, "seed trial_reminders row")
	t.Cleanup(func() {
		db.Exec("DELETE FROM trial_reminders WHERE subscription_id = ?", row.ID)
	})

	var remindersBefore int64
	require.NoError(t, db.Table("trial_reminders").Where("subscription_id = ?", row.ID).Count(&remindersBefore).Error)
	require.Equal(t, int64(1), remindersBefore, "sanity: reminder row must exist before Extend")

	// --- The call under test ------------------------------------------------
	adapter := &realStripeAdapter{c: c}
	extender := trial.NewExtender(adapter)

	newEnd := time.Now().Add(30 * 24 * time.Hour).UTC().Truncate(time.Second)

	start := time.Now()
	res, err := extender.Extend(ctx, db, storeID, newEnd, time.Now().UTC(), "")
	elapsed := time.Since(start)

	// This IS the number the design's safety argument depends on: the
	// FOR UPDATE row lock on store_subscriptions is held for exactly this
	// long, and stripeCallTimeout (10s) is what bounds it.
	t.Logf("row lock held for %s (Extend's stripeCallTimeout bound is 10s)", elapsed)

	require.NoError(t, err, "Extend")
	assert.True(t, res.StripeApplied, "StripeApplied must be true for a card-backed trial")
	assert.Equal(t, newEnd.Unix(), res.StripeTrialEnd, "ExtendResult.StripeTrialEnd must equal the new end's Unix second")

	// The LOCAL row's trial_ends_at equals the new end — proof the commit
	// happened AFTER the network call succeeded.
	var reread subscription.StoreSubscription
	require.NoError(t, db.Where("store_id = ?", storeID).First(&reread).Error, "reread local row")
	require.NotNil(t, reread.TrialEndsAt, "local trial_ends_at must be set")
	assert.True(t, newEnd.Equal(reread.TrialEndsAt.UTC()),
		"local trial_ends_at: want %s, got %s", newEnd, reread.TrialEndsAt.UTC())

	// Read the subscription back FROM STRIPE (not from our own request) —
	// trial_end and billing_cycle_anchor must both equal the new end, and
	// the price must be untouched.
	live, err := billingstripe.GetSubscription(ctx, c, sdkSub.ID)
	require.NoError(t, err, "GetSubscription (post-extend readback)")
	assert.Equal(t, newEnd.Unix(), live.TrialEnd, "stripe trial_end")
	assert.Equal(t, newEnd.Unix(), live.BillingCycleAnchor, "stripe billing_cycle_anchor must move with trial_end")
	require.Len(t, live.Items.Data, 1, "subscription must still have exactly one item")
	assert.Equal(t, price.ID, live.Items.Data[0].Price.ID, "extend must not have re-priced the subscription")

	// The reminder rows seeded before the call must be cleared.
	var remindersAfter int64
	require.NoError(t, db.Table("trial_reminders").Where("subscription_id = ?", row.ID).Count(&remindersAfter).Error)
	assert.Equal(t, int64(0), remindersAfter, "trial_reminders rows must be cleared by Extend")
}

// safePrefix returns a short, non-secret prefix of a key for an error
// message — enough to tell the operator which kind of key they used
// (sk_live_, rk_test_, ...) without ever printing the key itself.
func safePrefix(key string) string {
	if len(key) <= 8 {
		return "***"
	}
	return key[:8] + "***"
}
