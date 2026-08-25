//go:build integration

package trial_test

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	billingstripe "github.com/mark8ly/marketplace-api/internal/billing/stripe"
	"github.com/mark8ly/marketplace-api/internal/billing/trial"
	"github.com/mark8ly/marketplace-api/internal/subscription"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

var stripeExtendAsOf = time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)

// A card-less trial must behave IDENTICALLY through the Extender as it did
// through the package function. This is the regression guard for the
// refactor: if the new code path changes the common support case, this fails.
func TestExtender_CardlessPathUnchanged(t *testing.T) {
	db := testdb.NewDB(t, "trial_reminders", "store_subscriptions", "stores")

	derivedEnd := stripeExtendAsOf.Add(10 * 24 * time.Hour)
	seeded := seedExpiringRow(t, db, derivedEnd, nil)
	newEnd := stripeExtendAsOf.Add(60 * 24 * time.Hour)

	res, err := trial.NewExtender(nil).Extend(context.Background(), db, seeded.StoreID, newEnd, stripeExtendAsOf)
	require.NoError(t, err)
	require.True(t, derivedEnd.Equal(res.PreviousEndsAt))
	require.True(t, newEnd.Equal(res.NewEndsAt))
	require.False(t, res.StripeApplied, "a card-less extension touches no Stripe subscription")

	var after subscription.StoreSubscription
	require.NoError(t, db.First(&after, "store_id = ?", seeded.StoreID).Error)
	require.NotNil(t, after.TrialEndsAt)
	require.True(t, newEnd.Equal(*after.TrialEndsAt))
}

// A card-backed trial on a build with NO Stripe configured must refuse
// exactly as it does today. This is the guarantee that an unconfigured pod
// can never silently extend a Stripe-managed trial locally.
func TestExtender_NilUpdater_RefusesCardBacked(t *testing.T) {
	db := testdb.NewDB(t, "trial_reminders", "store_subscriptions", "stores")

	subID := "sub_nil_updater"
	seeded := seedExpiringRow(t, db, stripeExtendAsOf.Add(10*24*time.Hour),
		func(s *subscription.StoreSubscription) { s.StripeSubscriptionID = &subID })

	_, err := trial.NewExtender(nil).Extend(context.Background(), db,
		seeded.StoreID, stripeExtendAsOf.Add(60*24*time.Hour), stripeExtendAsOf)
	require.ErrorIs(t, err, trial.ErrStripeManaged)

	var after subscription.StoreSubscription
	require.NoError(t, db.First(&after, "store_id = ?", seeded.StoreID).Error)
	require.Nil(t, after.TrialEndsAt, "a refused extension must write nothing")
}

// fakeUpdater records what it was asked to do and returns what it is told to.
type fakeUpdater struct {
	get         *billingstripe.Subscription
	getErr      error
	updated     *billingstripe.Subscription
	updateErr   error
	seenParams  billingstripe.UpdateTrialEndParams
	updateCalls int
}

func (f *fakeUpdater) GetSubscription(ctx context.Context, id string) (*billingstripe.Subscription, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	return f.get, nil
}

func (f *fakeUpdater) UpdateTrialEnd(ctx context.Context, in billingstripe.UpdateTrialEndParams) (*billingstripe.Subscription, error) {
	f.updateCalls++
	f.seenParams = in
	if f.updateErr != nil {
		return nil, f.updateErr
	}
	return f.updated, nil
}

func seedCardBacked(t *testing.T, db *gorm.DB, subID string, derivedEnd time.Time) subscription.StoreSubscription {
	t.Helper()
	return seedExpiringRow(t, db, derivedEnd, func(s *subscription.StoreSubscription) {
		s.StripeSubscriptionID = &subID
	})
}

// seedReminder inserts one already-sent reminder slot so a refusal test can
// prove the reminder rows SURVIVE the refusal. Extend clears these on a
// successful extension, so their presence afterwards is direct evidence that
// no extension happened.
func seedReminder(t *testing.T, db *gorm.DB, sub subscription.StoreSubscription) {
	t.Helper()
	require.NoError(t, db.Exec(
		`INSERT INTO trial_reminders (subscription_id, tenant_id, store_id, offset_key, sent_at)
		 VALUES (?, ?, ?, 'no_pm_t_minus_15', ?)`,
		sub.ID, sub.TenantID, sub.StoreID, stripeExtendAsOf,
	).Error)
}

// requireUntouched asserts a refusal left BOTH the subscription row and the
// reminder rows exactly as they were. Asserting only the sentinel cannot
// distinguish "refused" from "refused after already writing".
func requireUntouched(t *testing.T, db *gorm.DB, sub subscription.StoreSubscription) {
	t.Helper()

	var after subscription.StoreSubscription
	require.NoError(t, db.First(&after, "store_id = ?", sub.StoreID).Error)
	require.Nil(t, after.TrialEndsAt, "a refusal must not write trial_ends_at")

	var reminders int64
	require.NoError(t, db.Table("trial_reminders").
		Where("subscription_id = ?", sub.ID).Count(&reminders).Error)
	require.Equal(t, int64(1), reminders, "a refusal must not clear the reminder rows")
}

// The acceptance criterion, at the domain layer: the EXACT Unix second must
// be handed to Stripe, and the local row must agree with it.
func TestExtender_CardBacked_SendsExactUnixSecondAndWritesLocally(t *testing.T) {
	db := testdb.NewDB(t, "trial_reminders", "store_subscriptions", "stores")

	const subID = "sub_card_ok"
	derivedEnd := stripeExtendAsOf.Add(10 * 24 * time.Hour)
	seeded := seedCardBacked(t, db, subID, derivedEnd)
	newEnd := stripeExtendAsOf.Add(60 * 24 * time.Hour)

	f := &fakeUpdater{
		get: &billingstripe.Subscription{
			ID: subID, Status: "trialing",
			TrialEnd:           derivedEnd.Unix(),
			BillingCycleAnchor: derivedEnd.Unix(),
		},
		updated: &billingstripe.Subscription{
			ID: subID, Status: "trialing",
			TrialEnd:           newEnd.Unix(),
			BillingCycleAnchor: newEnd.Unix(),
		},
	}

	res, err := trial.NewExtender(f).Extend(context.Background(), db, seeded.StoreID, newEnd, stripeExtendAsOf)
	require.NoError(t, err)

	require.Equal(t, 1, f.updateCalls)
	require.Equal(t, newEnd.Unix(), f.seenParams.TrialEnd,
		"the exact integer sent to Stripe is the acceptance criterion")
	require.Equal(t, subID, f.seenParams.SubscriptionID)
	// F1: the key is part of the wire contract, so it is pinned exactly.
	// It encodes the ABSOLUTE target second, which is what makes a replay
	// re-assert the same date rather than shift the trial again — Stripe's
	// own dedupe only lasts 24h and cannot be relied on beyond that.
	require.Equal(t,
		"trial_extend:"+seeded.StoreID.String()+":"+strconv.FormatInt(newEnd.Unix(), 10),
		f.seenParams.IdempotencyKey)
	require.Equal(t, seeded.StoreID.String(), f.seenParams.Metadata["mark8ly_store_id"])
	require.Equal(t, seeded.TenantID.String(), f.seenParams.Metadata["mark8ly_tenant_id"])

	require.True(t, res.StripeApplied)
	require.Equal(t, subID, res.StripeSubscriptionID)
	require.Equal(t, newEnd.Unix(), res.StripeTrialEnd, "read back from Stripe's reply, not echoed from the request")
	require.Equal(t, derivedEnd.Unix(), res.PreviousStripeTrialEnd)
	require.Equal(t, derivedEnd.Unix(), res.PreviousBillingAnchor)

	var after subscription.StoreSubscription
	require.NoError(t, db.First(&after, "store_id = ?", seeded.StoreID).Error)
	require.NotNil(t, after.TrialEndsAt)
	require.True(t, newEnd.Equal(*after.TrialEndsAt))
}

// THE FAILURE ORDERING, AS A TEST. Stripe fails => nothing is written
// locally. This is the decision the issue required be made deliberately;
// deleting the rollback must break this.
func TestExtender_CardBacked_StripeFailure_WritesNothingLocally(t *testing.T) {
	db := testdb.NewDB(t, "trial_reminders", "store_subscriptions", "stores")

	const subID = "sub_card_fail"
	derivedEnd := stripeExtendAsOf.Add(10 * 24 * time.Hour)
	seeded := seedCardBacked(t, db, subID, derivedEnd)
	require.NoError(t, db.Exec(
		`INSERT INTO trial_reminders (subscription_id, tenant_id, store_id, offset_key, sent_at)
		 VALUES (?, ?, ?, 'no_pm_t_minus_15', ?)`,
		seeded.ID, seeded.TenantID, seeded.StoreID, stripeExtendAsOf,
	).Error)

	f := &fakeUpdater{
		get: &billingstripe.Subscription{ID: subID, Status: "trialing",
			TrialEnd: derivedEnd.Unix(), BillingCycleAnchor: derivedEnd.Unix()},
		updateErr: errors.New("stripe: boom"),
	}

	_, err := trial.NewExtender(f).Extend(context.Background(), db,
		seeded.StoreID, stripeExtendAsOf.Add(60*24*time.Hour), stripeExtendAsOf)
	require.ErrorIs(t, err, trial.ErrStripeCall)

	var after subscription.StoreSubscription
	require.NoError(t, db.First(&after, "store_id = ?", seeded.StoreID).Error)
	require.Nil(t, after.TrialEndsAt, "Stripe failed: the local row must be untouched")

	var reminders int64
	require.NoError(t, db.Table("trial_reminders").
		Where("subscription_id = ?", seeded.ID).Count(&reminders).Error)
	require.Equal(t, int64(1), reminders, "Stripe failed: the reminder rows must survive")
}

// Local says trialing, Stripe says active. Refuse rather than reconcile —
// and do not call update.
func TestExtender_CardBacked_StripeNotTrialing_Refuses(t *testing.T) {
	db := testdb.NewDB(t, "trial_reminders", "store_subscriptions", "stores")

	const subID = "sub_card_active"
	seeded := seedCardBacked(t, db, subID, stripeExtendAsOf.Add(10*24*time.Hour))
	seedReminder(t, db, seeded)

	f := &fakeUpdater{get: &billingstripe.Subscription{ID: subID, Status: "active",
		TrialEnd: 0, BillingCycleAnchor: stripeExtendAsOf.Unix()}}

	_, err := trial.NewExtender(f).Extend(context.Background(), db,
		seeded.StoreID, stripeExtendAsOf.Add(60*24*time.Hour), stripeExtendAsOf)
	require.ErrorIs(t, err, trial.ErrStripeStateConflict)
	require.Equal(t, 0, f.updateCalls, "a refusal must not reach Stripe's update")
	// F3: the sentinel alone cannot see a refusal that failed to refuse.
	// Under the Stripe-after-COMMIT mutation this test passed while the row
	// HAD been extended; the data assertions are what close that.
	requireUntouched(t, db, seeded)
}

// THE BOUND, ON THE BOUNDARY. Stripe caps trial_end at two years from the
// CURRENT anchor. Exactly two years passes; one second past it refuses.
// "Close to the edge" is not the edge.
func TestExtender_CardBacked_TwoYearBound_AtTheBoundary(t *testing.T) {
	anchor := stripeExtendAsOf
	twoYears := 2 * 365 * 24 * time.Hour

	t.Run("exactly two years from anchor is allowed", func(t *testing.T) {
		db := testdb.NewDB(t, "trial_reminders", "store_subscriptions", "stores")
		const subID = "sub_bound_ok"
		seeded := seedCardBacked(t, db, subID, stripeExtendAsOf.Add(10*24*time.Hour))
		newEnd := anchor.Add(twoYears)

		f := &fakeUpdater{
			get: &billingstripe.Subscription{ID: subID, Status: "trialing",
				TrialEnd: stripeExtendAsOf.Unix(), BillingCycleAnchor: anchor.Unix()},
			updated: &billingstripe.Subscription{ID: subID, Status: "trialing",
				TrialEnd: newEnd.Unix(), BillingCycleAnchor: newEnd.Unix()},
		}
		_, err := trial.NewExtender(f).Extend(context.Background(), db, seeded.StoreID, newEnd, stripeExtendAsOf)
		require.NoError(t, err)
		require.Equal(t, 1, f.updateCalls)
	})

	t.Run("one second past two years refuses", func(t *testing.T) {
		db := testdb.NewDB(t, "trial_reminders", "store_subscriptions", "stores")
		const subID = "sub_bound_bad"
		seeded := seedCardBacked(t, db, subID, stripeExtendAsOf.Add(10*24*time.Hour))
		seedReminder(t, db, seeded)
		newEnd := anchor.Add(twoYears + time.Second)

		f := &fakeUpdater{get: &billingstripe.Subscription{ID: subID, Status: "trialing",
			TrialEnd: stripeExtendAsOf.Unix(), BillingCycleAnchor: anchor.Unix()}}
		_, err := trial.NewExtender(f).Extend(context.Background(), db, seeded.StoreID, newEnd, stripeExtendAsOf)
		require.ErrorIs(t, err, trial.ErrTrialEndTooFar)
		require.Equal(t, 0, f.updateCalls)
		requireUntouched(t, db, seeded) // F3
	})
}

// The bound is measured from Stripe's ANCHOR, not from now. This is the
// fixture that discriminates between the two implementations: the anchor is
// deliberately far from `now`, so a now-based bound gives a different answer.
func TestExtender_CardBacked_BoundIsMeasuredFromAnchorNotNow(t *testing.T) {
	db := testdb.NewDB(t, "trial_reminders", "store_subscriptions", "stores")
	const subID = "sub_anchor_far"
	seeded := seedCardBacked(t, db, subID, stripeExtendAsOf.Add(10*24*time.Hour))

	// Anchor is 18 months in the PAST, so anchor+2y is only 6 months out.
	anchor := stripeExtendAsOf.Add(-18 * 30 * 24 * time.Hour)
	newEnd := stripeExtendAsOf.Add(12 * 30 * 24 * time.Hour) // legal under now+2y, illegal under anchor+2y

	f := &fakeUpdater{get: &billingstripe.Subscription{ID: subID, Status: "trialing",
		TrialEnd: stripeExtendAsOf.Unix(), BillingCycleAnchor: anchor.Unix()}}
	_, err := trial.NewExtender(f).Extend(context.Background(), db, seeded.StoreID, newEnd, stripeExtendAsOf)
	require.ErrorIs(t, err, trial.ErrTrialEndTooFar,
		"a now-based bound would allow this; the bound is from the anchor")
	require.Equal(t, 0, f.updateCalls)
}

// Two stores, one Extender: a card-backed extension must move exactly the
// subscription it was asked for. One store cannot prove scoping (trap 13,
// which cost #286 a Critical).
func TestExtender_CardBacked_ScopedToTheRequestedStore(t *testing.T) {
	db := testdb.NewDB(t, "trial_reminders", "store_subscriptions", "stores")

	derivedEnd := stripeExtendAsOf.Add(10 * 24 * time.Hour)
	target := seedCardBacked(t, db, "sub_target", derivedEnd)
	other := seedCardBacked(t, db, "sub_other", derivedEnd)
	newEnd := stripeExtendAsOf.Add(60 * 24 * time.Hour)

	f := &fakeUpdater{
		get: &billingstripe.Subscription{ID: "sub_target", Status: "trialing",
			TrialEnd: derivedEnd.Unix(), BillingCycleAnchor: derivedEnd.Unix()},
		updated: &billingstripe.Subscription{ID: "sub_target", Status: "trialing",
			TrialEnd: newEnd.Unix(), BillingCycleAnchor: newEnd.Unix()},
	}
	_, err := trial.NewExtender(f).Extend(context.Background(), db, target.StoreID, newEnd, stripeExtendAsOf)
	require.NoError(t, err)
	require.Equal(t, "sub_target", f.seenParams.SubscriptionID)

	var untouched subscription.StoreSubscription
	require.NoError(t, db.First(&untouched, "store_id = ?", other.StoreID).Error)
	require.Nil(t, untouched.TrialEndsAt, "the other store must not be extended")
}

// F2. The typed *billingstripe.APIError must survive the wrap. Without it a
// card decline, a rate limit and a 500 all reach the handler as one opaque
// ErrStripeCall, and Task 4 cannot pick an HTTP code. Both claims are
// asserted together: the sentinel is what the handler switches on, the typed
// cause is what it reports.
func TestExtender_CardBacked_StripeAPIErrorSurvivesTheWrap(t *testing.T) {
	db := testdb.NewDB(t, "trial_reminders", "store_subscriptions", "stores")

	const subID = "sub_typed_err"
	derivedEnd := stripeExtendAsOf.Add(10 * 24 * time.Hour)
	seeded := seedCardBacked(t, db, subID, derivedEnd)
	seedReminder(t, db, seeded)

	cause := &billingstripe.APIError{
		HTTPStatus: 402, Type: "card_error", Code: "card_declined", RequestID: "req_typed",
	}
	f := &fakeUpdater{
		get: &billingstripe.Subscription{ID: subID, Status: "trialing",
			TrialEnd: derivedEnd.Unix(), BillingCycleAnchor: derivedEnd.Unix()},
		updateErr: cause,
	}

	_, err := trial.NewExtender(f).Extend(context.Background(), db,
		seeded.StoreID, stripeExtendAsOf.Add(60*24*time.Hour), stripeExtendAsOf)

	require.ErrorIs(t, err, trial.ErrStripeCall, "the sentinel the handler switches on must still match")

	var apiErr *billingstripe.APIError
	require.True(t, errors.As(err, &apiErr), "the typed cause must be recoverable through the wrap")
	require.Equal(t, 402, apiErr.HTTPStatus)
	require.Equal(t, "card_error", apiErr.Type)
	require.Equal(t, "card_declined", apiErr.Code)

	requireUntouched(t, db, seeded)
}

// F4. GetSubscription failing is a distinct path from UpdateTrialEnd failing:
// it must refuse BEFORE reaching the update, and write nothing.
func TestExtender_CardBacked_GetSubscriptionFailure_RefusesBeforeUpdate(t *testing.T) {
	db := testdb.NewDB(t, "trial_reminders", "store_subscriptions", "stores")

	const subID = "sub_get_fail"
	seeded := seedCardBacked(t, db, subID, stripeExtendAsOf.Add(10*24*time.Hour))
	seedReminder(t, db, seeded)

	f := &fakeUpdater{getErr: errors.New("stripe: unreachable")}

	_, err := trial.NewExtender(f).Extend(context.Background(), db,
		seeded.StoreID, stripeExtendAsOf.Add(60*24*time.Hour), stripeExtendAsOf)
	require.ErrorIs(t, err, trial.ErrStripeCall)
	require.Contains(t, err.Error(), "get subscription", "the failing call must be identifiable")
	require.Equal(t, 0, f.updateCalls, "a failed read must never be followed by a write to Stripe")

	requireUntouched(t, db, seeded)
}

// F6. A (nil, nil) return from the updater must not panic inside the open
// FOR UPDATE transaction. The real client never does this; the guard exists
// because a panic while holding a row lock is a far worse failure than an
// error.
func TestExtender_CardBacked_NilSubscriptionFromStripe_IsAnErrorNotAPanic(t *testing.T) {
	db := testdb.NewDB(t, "trial_reminders", "store_subscriptions", "stores")

	const subID = "sub_nil_reply"
	derivedEnd := stripeExtendAsOf.Add(10 * 24 * time.Hour)
	seeded := seedCardBacked(t, db, subID, derivedEnd)
	seedReminder(t, db, seeded)

	f := &fakeUpdater{
		get: &billingstripe.Subscription{ID: subID, Status: "trialing",
			TrialEnd: derivedEnd.Unix(), BillingCycleAnchor: derivedEnd.Unix()},
		updated: nil, // and updateErr nil: the pathological (nil, nil)
	}

	_, err := trial.NewExtender(f).Extend(context.Background(), db,
		seeded.StoreID, stripeExtendAsOf.Add(60*24*time.Hour), stripeExtendAsOf)
	require.ErrorIs(t, err, trial.ErrStripeCall)
	requireUntouched(t, db, seeded)
}

// F5. THE ONE DIVERGENCE THIS DESIGN ACCEPTS. Stripe moved, the local write
// then failed: the merchant is now billed on a date the console does not
// know about. That must be distinguishable from a routine DB error, and the
// message must carry what a human needs to reconcile by hand.
func TestExtender_CardBacked_StripeAppliedThenLocalWriteFails_IsItsOwnError(t *testing.T) {
	db := testdb.NewDB(t, "trial_reminders", "store_subscriptions", "stores")

	const subID = "sub_diverged"
	derivedEnd := stripeExtendAsOf.Add(10 * 24 * time.Hour)
	seeded := seedCardBacked(t, db, subID, derivedEnd)
	newEnd := stripeExtendAsOf.Add(60 * 24 * time.Hour)

	f := &fakeUpdater{
		get: &billingstripe.Subscription{ID: subID, Status: "trialing",
			TrialEnd: derivedEnd.Unix(), BillingCycleAnchor: derivedEnd.Unix()},
		updated: &billingstripe.Subscription{ID: subID, Status: "trialing",
			TrialEnd: newEnd.Unix(), BillingCycleAnchor: newEnd.Unix()},
	}

	// Force the LOCAL write to fail while leaving the reads working, so the
	// Stripe call has already succeeded by the time the failure lands.
	// testdb.NewDB opens a fresh *gorm.DB per call, so this callback cannot
	// leak into any other test.
	require.NoError(t, db.Callback().Update().Before("gorm:update").
		Register("test:force_update_failure", func(tx *gorm.DB) {
			tx.AddError(errors.New("disk on fire"))
		}))

	_, err := trial.NewExtender(f).Extend(context.Background(), db, seeded.StoreID, newEnd, stripeExtendAsOf)

	require.ErrorIs(t, err, trial.ErrStripeAppliedLocalWriteFailed,
		"a moved billing anchor with no local record is not a routine DB error")
	require.NotErrorIs(t, err, trial.ErrStripeCall, "Stripe did not fail; the database did")

	// The reconciliation facts a human needs, in the message itself.
	msg := err.Error()
	require.Contains(t, msg, subID)
	require.Contains(t, msg, fmt.Sprintf("trial_end=%d", newEnd.Unix()))
	require.Contains(t, msg, fmt.Sprintf("previously trial_end=%d", derivedEnd.Unix()))
	require.Contains(t, msg, fmt.Sprintf("billing_cycle_anchor=%d", derivedEnd.Unix()))
	require.Contains(t, msg, "disk on fire", "the underlying cause must not be swallowed")

	require.Equal(t, 1, f.updateCalls, "Stripe really was called; that is the whole problem")
}

// F7. Nothing else proves Postgres actually receives FOR UPDATE — the lock is
// what removes the window in which the row could convert while Stripe is in
// flight, and a dropped Clauses() call would be silent. Asserts on the SQL
// gorm emits rather than on timing: a flaky concurrency test would be worse
// than this gap.
func TestExtender_LocksTheSubscriptionRowForUpdate(t *testing.T) {
	db := testdb.NewDB(t, "trial_reminders", "store_subscriptions", "stores")

	const subID = "sub_locking"
	derivedEnd := stripeExtendAsOf.Add(10 * 24 * time.Hour)
	seeded := seedCardBacked(t, db, subID, derivedEnd)
	newEnd := stripeExtendAsOf.Add(60 * 24 * time.Hour)

	var selects []string
	require.NoError(t, db.Callback().Query().After("gorm:query").
		Register("test:capture_select", func(tx *gorm.DB) {
			selects = append(selects, tx.Statement.SQL.String())
		}))

	f := &fakeUpdater{
		get: &billingstripe.Subscription{ID: subID, Status: "trialing",
			TrialEnd: derivedEnd.Unix(), BillingCycleAnchor: derivedEnd.Unix()},
		updated: &billingstripe.Subscription{ID: subID, Status: "trialing",
			TrialEnd: newEnd.Unix(), BillingCycleAnchor: newEnd.Unix()},
	}
	_, err := trial.NewExtender(f).Extend(context.Background(), db, seeded.StoreID, newEnd, stripeExtendAsOf)
	require.NoError(t, err)

	var locked string
	for _, q := range selects {
		if strings.Contains(q, "store_subscriptions") && strings.Contains(q, "FOR UPDATE") {
			locked = q
			break
		}
	}
	require.NotEmpty(t, locked,
		"the store_subscriptions SELECT must carry FOR UPDATE; captured: %v", selects)
}
