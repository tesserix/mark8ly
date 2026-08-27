//go:build integration

package dispatch_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/billing/dispatch"
	"github.com/mark8ly/marketplace-api/internal/email"
	"github.com/mark8ly/marketplace-api/internal/postcommit"
	"github.com/mark8ly/marketplace-api/internal/subscription"
	"github.com/mark8ly/marketplace-api/internal/webhookevents"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

// seedFirstChargeSub inserts a store + a subscription with first_charge_at
// NULL, returning the db handle, the row and its Stripe customer id.
func seedFirstChargeSub(t *testing.T) (*gorm.DB, subscription.StoreSubscription, string) {
	t.Helper()
	db := testdb.NewDB(t, "store_subscriptions", "stripe_webhook_events", "billing_email_sends")

	tenantID, storeID := uuid.New(), uuid.New()
	testdb.SeedStore(t, db, tenantID, storeID)
	addr := "merchant@example.com"
	customerID := "cus_" + uuid.NewString()[:12]
	sub := subscription.StoreSubscription{
		TenantID:         tenantID,
		StoreID:          storeID,
		StripeCustomerID: customerID,
		Plan:             subscription.PlanStarter,
		Status:           subscription.StatusActive,
		Email:            &addr,
	}
	require.NoError(t, db.Create(&sub).Error)
	return db, sub, customerID
}

// dispatchInvoicePaid dispatches one first-charge invoice.paid event through
// the PRODUCTION shape: a post-commit collector installed in the context, and
// drained once Dispatch returns.
//
// It installs a collector by default on purpose. With a bare context every
// test here took the inline fallback instead, where the claim and the send
// run side by side unconditionally — so a regression in the deferred path
// (the one production actually uses) left this suite green. The fallback
// keeps exactly one dedicated test, below.
//
// Returns only the Dispatch error. A deferred send that fails is non-fatal by
// contract — the transaction has already committed — so drain errors are
// logged, and tests assert on the observable outcome (recipients, claim rows)
// instead.
func dispatchInvoicePaid(t *testing.T, d *dispatch.Dispatcher, db *gorm.DB, customerID string) error {
	t.Helper()
	ctx, deferred := postcommit.WithDeferredSends(context.Background())
	err := dispatchInvoicePaidOn(t, ctx, d, db, customerID)
	for _, sendErr := range deferred.Run(ctx) {
		t.Logf("deferred send failed (non-fatal by contract): %v", sendErr)
	}
	return err
}

// dispatchInvoicePaidOn is dispatchInvoicePaid without the collector, for
// callers that supply their own context (or deliberately supply none).
func dispatchInvoicePaidOn(t *testing.T, ctx context.Context, d *dispatch.Dispatcher, db *gorm.DB, customerID string) error {
	t.Helper()
	eventID := "evt_" + uuid.NewString()[:12]
	payload := []byte(`{"id":"` + eventID + `","type":"invoice.paid","data":{"object":{"customer":"` + customerID + `"}}}`)
	return d.Dispatch(ctx, db, webhookevents.StripeWebhookEvent{
		EventID: eventID, EventType: "invoice.paid", Payload: payload,
	})
}

// first_charge_at can return to NULL after the confirmation has gone out —
// a compensating correction, a restore, an operator undo — and Stripe can
// redeliver invoice.paid afterwards, at which point the handler sees a
// "first charge" again. The claim is written on a NON-transactional handle
// precisely so it outlives whatever put first_charge_at back, and the retry
// then loses the claim and sends nothing. This test reproduces that by
// resetting the column directly.
//
// Note what this test does NOT cover: it drains its collector immediately
// after Dispatch returns, so it never sees a transaction that rolled back
// before the drain. That case — the send discarded, and with it the claim —
// is covered by
// TestInvoicePaid_TrialBilled_RolledBackWebhookStillDeliversOnRetry below.
func TestInvoicePaid_TrialBilled_NotResentAfterRollback(t *testing.T) {
	db, sub, customerID := seedFirstChargeSub(t)

	client := &captureEmailClient{}
	d := dispatch.New(nil).WithEmail(client).WithDB(db)

	require.NoError(t, dispatchInvoicePaid(t, d, db, customerID))
	require.Len(t, client.recipients, 1, "first charge must send one confirmation")

	// Simulate the transaction rolling back after the email went out.
	require.NoError(t, db.Exec(
		`UPDATE store_subscriptions SET first_charge_at = NULL WHERE id = ?`, sub.ID).Error)

	// Stripe retries (or redelivers as a fresh event).
	require.NoError(t, dispatchInvoicePaid(t, d, db, customerID))

	require.Len(t, client.recipients, 1,
		"a second trial-billed confirmation was sent after a rollback")
}

// The claim is the guarantee, so it must exist after a successful send —
// keyed by the constant first_charge period, i.e. once per subscription.
func TestInvoicePaid_TrialBilled_ClaimsRow(t *testing.T) {
	db, sub, customerID := seedFirstChargeSub(t)

	d := dispatch.New(nil).WithEmail(&captureEmailClient{}).WithDB(db)
	require.NoError(t, dispatchInvoicePaid(t, d, db, customerID))

	var n int64
	require.NoError(t, db.Raw(
		`SELECT count(*) FROM billing_email_sends
		 WHERE subscription_id = ? AND template_key = ? AND period_key = 'first_charge'`,
		sub.ID, string(email.TemplateTrialStartedBilled)).Scan(&n).Error)
	require.EqualValues(t, 1, n, "no claim row written — at-most-once is unenforced")
}

// Without a claim store there is no way to make the send at-most-once, so it
// is skipped rather than risked. The webhook's own side effects still run.
func TestInvoicePaid_TrialBilled_NoClaimStoreSkipsSend(t *testing.T) {
	db, sub, customerID := seedFirstChargeSub(t)

	client := &captureEmailClient{}
	d := dispatch.New(nil).WithEmail(client) // no WithDB
	require.NoError(t, dispatchInvoicePaid(t, d, db, customerID))

	require.Empty(t, client.recipients, "sent without a claim store — cannot be at-most-once")

	var reloaded subscription.StoreSubscription
	require.NoError(t, db.Where("id = ?", sub.ID).First(&reloaded).Error)
	require.NotNil(t, reloaded.FirstChargeAt, "webhook side effects must still run")
}

// seedFirstChargeSubWithPlan is seedFirstChargeSub with an explicit plan, so
// the two tests below differ only in the plan column.
func seedFirstChargeSubWithPlan(t *testing.T, plan subscription.SubscriptionPlan) (*gorm.DB, subscription.StoreSubscription, string) {
	t.Helper()
	db := testdb.NewDB(t, "store_subscriptions", "stripe_webhook_events", "billing_email_sends")

	tenantID, storeID := uuid.New(), uuid.New()
	testdb.SeedStore(t, db, tenantID, storeID)
	addr := "merchant@example.com"
	customerID := "cus_" + uuid.NewString()[:12]
	sub := subscription.StoreSubscription{
		TenantID:         tenantID,
		StoreID:          storeID,
		StripeCustomerID: customerID,
		Plan:             plan,
		Status:           subscription.StatusActive,
		Email:            &addr,
	}
	require.NoError(t, db.Create(&sub).Error)
	return db, sub, customerID
}

// The trial-billed template names the plan the merchant is now being billed
// for. plan='trial' at first charge means no plan was ever recorded (e.g. the
// legacy CreateCheckoutSession route, which never persists it), so the email
// would tell the merchant their "trial plan" is active and billed monthly.
// Skip the send instead.
func TestInvoicePaid_TrialBilled_SkippedWhenPlanStillTrial(t *testing.T) {
	db, sub, customerID := seedFirstChargeSubWithPlan(t, subscription.PlanTrial)

	client := &captureEmailClient{}
	d := dispatch.New(nil).WithEmail(client).WithDB(db)
	require.NoError(t, dispatchInvoicePaid(t, d, db, customerID))

	require.Empty(t, client.recipients, "confirmed a 'trial plan' the merchant was never billed for")

	// The claim must NOT be burned: the skip sends nothing, so there is no
	// double-send to guard against, and a claim would suppress a correct
	// confirmation if the plan is resolved before a retry lands.
	var n int64
	require.NoError(t, db.Raw(
		`SELECT count(*) FROM billing_email_sends
		 WHERE subscription_id = ? AND template_key = ?`,
		sub.ID, string(email.TemplateTrialStartedBilled)).Scan(&n).Error)
	require.EqualValues(t, 0, n, "the at-most-once slot was burned on a send that never happened")

	// The webhook's own side effects still run.
	var reloaded subscription.StoreSubscription
	require.NoError(t, db.Where("id = ?", sub.ID).First(&reloaded).Error)
	require.NotNil(t, reloaded.FirstChargeAt, "webhook side effects must still run")
}

// The counterpart: with a real plan the confirmation must still be sent.
// Without this the guard could silently disable the template entirely.
func TestInvoicePaid_TrialBilled_StillSendsWithRealPlan(t *testing.T) {
	db, _, customerID := seedFirstChargeSubWithPlan(t, subscription.PlanStarter)

	client := &captureEmailClient{}
	d := dispatch.New(nil).WithEmail(client).WithDB(db)
	require.NoError(t, dispatchInvoicePaid(t, d, db, customerID))

	require.Len(t, client.recipients, 1, "a merchant on a real plan must still get the confirmation")
}

// TestInvoicePaid_TrialBilled_RolledBackWebhookStillDeliversOnRetry pins the
// delivery guarantee on the real deferred path.
//
// Since the send moved out of the transaction, a rollback discards it: the
// collector is never drained (stripe.go and orphan_resolver.go both return
// early on the lock's error), so nothing was sent and — because the claim is
// taken inside that same discarded unit of work — nothing was claimed either.
// The merchant is therefore owed the confirmation, and Stripe's retry must
// deliver it.
//
// This is the test that catches claiming too early. Take the claim inside the
// transaction instead and it survives the rollback that dropped the send, the
// retry finds the slot burned, and the merchant receives NOTHING — permanently,
// because the retry commits first_charge_at and the template never fires
// again. That failure is silent: no error, no retry, no bounce.
//
// The sibling test above cannot catch it. It exercises the inline fallback,
// where claim and send run together unconditionally.
func TestInvoicePaid_TrialBilled_RolledBackWebhookStillDeliversOnRetry(t *testing.T) {
	db, sub, customerID := seedFirstChargeSub(t)

	client := &captureEmailClient{}
	d := dispatch.New(nil).WithEmail(client).WithDB(db)

	// Attempt 1: the transaction rolls back after the handler ran.
	ctx, pending := postcommit.WithDeferredSends(context.Background())
	errLaterStage := errors.New("a later stage of the chain failed")
	eventID := "evt_" + uuid.NewString()[:12]
	payload := []byte(`{"id":"` + eventID + `","type":"invoice.paid","data":{"object":{"customer":"` + customerID + `"}}}`)
	err := subscription.WithAdvisoryLock(ctx, db, sub.StoreID, func(tx *gorm.DB) error {
		if derr := d.Dispatch(ctx, tx, webhookevents.StripeWebhookEvent{
			EventID: eventID, EventType: "invoice.paid", Payload: payload,
		}); derr != nil {
			return derr
		}
		return errLaterStage
	})
	require.ErrorIs(t, err, errLaterStage)

	// The lock returned an error, so the collector is deliberately never
	// drained — exactly what both production call sites do. Whatever the
	// handler registered on it is discarded here, unrun and unreported.
	_ = pending
	require.Empty(t, client.recipients,
		"a send registered on the collector must not run when the transaction rolled back")

	// Confirm the rollback really happened, so the assertions below are
	// about a rolled-back transaction and not a silently committed one.
	var reloaded subscription.StoreSubscription
	require.NoError(t, db.Where("id = ?", sub.ID).First(&reloaded).Error)
	require.Nil(t, reloaded.FirstChargeAt, "the transaction did not roll back")

	// No claim: it lives inside the discarded unit of work, so the retry
	// still has its one shot. A claim here would be a burned slot paid for
	// by an email that never left.
	require.EqualValues(t, 0, claimCount(t, db, sub.ID),
		"the rollback burned the at-most-once slot without sending anything")

	// Attempt 2: Stripe retries. This one commits and drains.
	ctx2, deferred2 := postcommit.WithDeferredSends(context.Background())
	eventID2 := "evt_" + uuid.NewString()[:12]
	payload2 := []byte(`{"id":"` + eventID2 + `","type":"invoice.paid","data":{"object":{"customer":"` + customerID + `"}}}`)
	require.NoError(t, subscription.WithAdvisoryLock(ctx2, db, sub.StoreID, func(tx *gorm.DB) error {
		return d.Dispatch(ctx2, tx, webhookevents.StripeWebhookEvent{
			EventID: eventID2, EventType: "invoice.paid", Payload: payload2,
		})
	}))
	require.Empty(t, deferred2.Run(ctx2))

	require.Len(t, client.recipients, 1,
		"the merchant got %d confirmations across a rollback and its retry, want exactly 1",
		len(client.recipients))
	require.EqualValues(t, 1, claimCount(t, db, sub.ID),
		"the delivered send must leave a claim behind, or a redelivery would send again")
}

// claimCount returns the number of trial-billed claim rows for a subscription.
func claimCount(t *testing.T, db *gorm.DB, subID uuid.UUID) int64 {
	t.Helper()
	var n int64
	require.NoError(t, db.Raw(
		`SELECT count(*) FROM billing_email_sends
		 WHERE subscription_id = ? AND template_key = ? AND period_key = 'first_charge'`,
		subID, string(email.TemplateTrialStartedBilled)).Scan(&n).Error)
	return n
}

// The inline fallback, kept as one explicit test now that the helper above
// installs a collector. A caller that never opted in (a future entry point
// that forgets postcommit.WithDeferredSends) must still deliver the email —
// slowly, inside the transaction, with a warning — rather than drop it.
func TestInvoicePaid_TrialBilled_InlineFallbackStillSends(t *testing.T) {
	db, sub, customerID := seedFirstChargeSub(t)

	client := &captureEmailClient{}
	d := dispatch.New(nil).WithEmail(client).WithDB(db)

	// Bare context: no collector, so postcommit.Add reports false.
	require.NoError(t, dispatchInvoicePaidOn(t, context.Background(), d, db, customerID))

	require.Len(t, client.recipients, 1,
		"the inline fallback dropped the email instead of sending it")
	require.EqualValues(t, 1, claimCount(t, db, sub.ID),
		"the inline send must leave a claim behind")
}

// An address failure must NOT burn the one-per-subscription slot.
//
// period_key is the constant "first_charge", so the claim is per-subscription
// for life. A merchant still carrying a billing+<uuid>@mark8ly.local
// placeholder at first charge is refused before any network call — and if that
// refusal kept the claim, the real address arriving an hour later could never
// redeem it: first_charge_at is committed, so wasFirstCharge is false forever
// and the template never fires again. The merchant would never receive a
// trial-billed confirmation at all.
func TestInvoicePaid_TrialBilled_UndeliverableAddressReleasesTheClaim(t *testing.T) {
	db, sub, customerID := seedFirstChargeSub(t)

	// The exact placeholder subscription/service.go mints at bootstrap.
	placeholder := "billing+" + uuid.NewString() + "@mark8ly.local"
	require.NoError(t, db.Exec(
		`UPDATE store_subscriptions SET email = ? WHERE id = ?`, placeholder, sub.ID).Error)

	client := &captureEmailClient{}
	d := dispatch.New(nil).WithEmail(client).WithDB(db)

	require.NoError(t, dispatchInvoicePaid(t, d, db, customerID),
		"an undeliverable address must not fail the webhook")

	require.Empty(t, client.recipients, "a .local address must never reach a provider")
	require.EqualValues(t, 0, claimCount(t, db, sub.ID),
		"the address failure burned the at-most-once slot; this merchant can now "+
			"never receive a trial-billed confirmation")

	// The webhook's own side effects still ran — which is exactly why the
	// released claim is the merchant's only remaining route to this email.
	var reloaded subscription.StoreSubscription
	require.NoError(t, db.Where("id = ?", sub.ID).First(&reloaded).Error)
	require.NotNil(t, reloaded.FirstChargeAt, "webhook side effects must still run")

	// The real address lands (backfill, or a customer.updated webhook), and a
	// later qualifying event delivers.
	//
	// first_charge_at has to be reset for that event to qualify at all: a
	// natural Stripe retry of the SAME charge can no longer see a first
	// charge. Resetting it models the documented paths that do put it back —
	// a compensating correction, a restore — and is the only way to reach the
	// send a second time. What is being pinned is the released claim, not the
	// reset: with the claim still held, this second attempt sends nothing.
	require.NoError(t, db.Exec(
		`UPDATE store_subscriptions SET email = ?, first_charge_at = NULL WHERE id = ?`,
		"merchant@example.com", sub.ID).Error)

	require.NoError(t, dispatchInvoicePaid(t, d, db, customerID))

	require.Len(t, client.recipients, 1,
		"the released claim was not redeemed — the merchant lost the confirmation for good")
	require.Equal(t, "merchant@example.com", client.recipients[0])
	require.EqualValues(t, 1, claimCount(t, db, sub.ID),
		"the delivered send must leave a claim behind")
}
