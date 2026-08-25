//go:build integration

package trial_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/billing/trial"
	"github.com/mark8ly/marketplace-api/internal/subscription"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

// extendAsOf is the fixed "now" every test in this file passes, so
// boundaries are pinned rather than racing the wall clock.
var extendAsOf = time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)

// Happy path. The subscription was created 80 days ago (derived end 10 days
// out); the operator moves it to 60 days out. The assertion is on the value
// stored AND on what EndsAt reports afterwards — storing a value nothing
// reads is the defect #353 existed to remove.
func TestExtend_SetsTheNewEndAndReportsThePrevious(t *testing.T) {
	db := testdb.NewDB(t, "trial_reminders", "store_subscriptions", "stores")

	derivedEnd := extendAsOf.Add(10 * 24 * time.Hour)
	seeded := seedExpiringRow(t, db, derivedEnd, nil)
	newEnd := extendAsOf.Add(60 * 24 * time.Hour)

	res, err := trial.NewExtender(nil).Extend(context.Background(), db, seeded.StoreID, newEnd, extendAsOf, "")
	require.NoError(t, err)

	require.Equal(t, seeded.StoreID, res.StoreID)
	require.Equal(t, seeded.TenantID, res.TenantID)
	require.True(t, derivedEnd.Equal(res.PreviousEndsAt),
		"previous must be the EFFECTIVE end before the write: want %s got %s", derivedEnd, res.PreviousEndsAt)
	require.True(t, newEnd.Equal(res.NewEndsAt))

	var after subscription.StoreSubscription
	require.NoError(t, db.First(&after, "store_id = ?", seeded.StoreID).Error)
	require.NotNil(t, after.TrialEndsAt)
	require.True(t, newEnd.Equal(*after.TrialEndsAt))
	require.True(t, newEnd.Equal(trial.EndsAt(after)),
		"EndsAt must report the stored value — otherwise nothing downstream sees the extension")
}

// The reminder cadence must re-arm. trial_reminders' PK is
// (subscription_id, offset_key) with ON CONFLICT DO NOTHING, so a warning
// already sent can never re-send; leaving the rows would mean a merchant
// extended past their T-15 gets no notice before the date they are charged.
func TestExtend_ClearsSentReminders(t *testing.T) {
	db := testdb.NewDB(t, "trial_reminders", "store_subscriptions", "stores")

	seeded := seedExpiringRow(t, db, extendAsOf.Add(10*24*time.Hour), nil)
	require.NoError(t, db.Exec(
		`INSERT INTO trial_reminders (subscription_id, tenant_id, store_id, offset_key, sent_at)
		 VALUES (?, ?, ?, 'no_pm_t_minus_15', ?)`,
		seeded.ID, seeded.TenantID, seeded.StoreID, extendAsOf,
	).Error)

	res, err := trial.NewExtender(nil).Extend(context.Background(), db,
		seeded.StoreID, extendAsOf.Add(60*24*time.Hour), extendAsOf, "")
	require.NoError(t, err)
	require.Equal(t, int64(1), res.RemindersCleared)

	var n int64
	require.NoError(t, db.Table("trial_reminders").
		Where("subscription_id = ?", seeded.ID).Count(&n).Error)
	require.Equal(t, int64(0), n, "the sent reminder must be cleared so the cadence re-arms")
}

// A converted subscription is REFUSED, not silently ignored — #286's own
// acceptance criterion. The control in the same test proves the refusal is
// not simply always firing.
func TestExtend_RefusesConverted(t *testing.T) {
	db := testdb.NewDB(t, "trial_reminders", "store_subscriptions", "stores")

	converted := seedExpiringRow(t, db, extendAsOf.Add(10*24*time.Hour),
		func(r *subscription.StoreSubscription) { r.Status = subscription.StatusActive })
	trialing := seedExpiringRow(t, db, extendAsOf.Add(10*24*time.Hour), nil)

	_, err := trial.NewExtender(nil).Extend(context.Background(), db,
		converted.StoreID, extendAsOf.Add(60*24*time.Hour), extendAsOf, "")
	require.ErrorIs(t, err, trial.ErrAlreadyConverted)

	var untouched subscription.StoreSubscription
	require.NoError(t, db.First(&untouched, "store_id = ?", converted.StoreID).Error)
	require.Nil(t, untouched.TrialEndsAt, "a refused extension must not write anything")

	_, err = trial.NewExtender(nil).Extend(context.Background(), db,
		trialing.StoreID, extendAsOf.Add(60*24*time.Hour), extendAsOf, "")
	require.NoError(t, err, "the control MUST succeed — otherwise this test passes because everything is refused")
}

// A card-backed trial is refused: Stripe holds that billing date, and
// writing locally without telling Stripe is the split-brain #353 removed.
func TestExtend_RefusesStripeManaged(t *testing.T) {
	db := testdb.NewDB(t, "trial_reminders", "store_subscriptions", "stores")

	subID := "sub_live_abc123"
	managed := seedExpiringRow(t, db, extendAsOf.Add(10*24*time.Hour),
		func(r *subscription.StoreSubscription) { r.StripeSubscriptionID = &subID })

	_, err := trial.NewExtender(nil).Extend(context.Background(), db,
		managed.StoreID, extendAsOf.Add(60*24*time.Hour), extendAsOf, "")
	require.ErrorIs(t, err, trial.ErrStripeManaged)

	var untouched subscription.StoreSubscription
	require.NoError(t, db.First(&untouched, "store_id = ?", managed.StoreID).Error)
	require.Nil(t, untouched.TrialEndsAt)
}

// Statuses outside the trial states are refused with their own error, so
// the console can tell "converted" from "expired". `signup` and `trialing`
// are BOTH accepted — the reminder cron targets both.
func TestExtend_StatusMatrix(t *testing.T) {
	db := testdb.NewDB(t, "trial_reminders", "store_subscriptions", "stores")

	cases := []struct {
		status  subscription.SubscriptionStatus
		wantErr error
	}{
		{subscription.StatusTrialing, nil},
		{subscription.StatusSignup, nil},
		{subscription.StatusActive, trial.ErrAlreadyConverted},
		{subscription.StatusExpired, trial.ErrNotTrialing},
	}

	for _, tc := range cases {
		t.Run(string(tc.status), func(t *testing.T) {
			st := tc.status
			seeded := seedExpiringRow(t, db, extendAsOf.Add(10*24*time.Hour),
				func(r *subscription.StoreSubscription) { r.Status = st })

			_, err := trial.NewExtender(nil).Extend(context.Background(), db,
				seeded.StoreID, extendAsOf.Add(60*24*time.Hour), extendAsOf, "")
			if tc.wantErr == nil {
				require.NoError(t, err)
				return
			}
			require.ErrorIs(t, err, tc.wantErr)
		})
	}
}

// An end in the past is refused: it is indistinguishable from expiring the
// trial, which the cron already does. The boundary is `now` itself — the
// instant where a `>` and a `>=` implementation disagree.
func TestExtend_RefusesEndNotInFuture(t *testing.T) {
	db := testdb.NewDB(t, "trial_reminders", "store_subscriptions", "stores")

	seeded := seedExpiringRow(t, db, extendAsOf.Add(10*24*time.Hour), nil)

	_, err := trial.NewExtender(nil).Extend(context.Background(), db, seeded.StoreID, extendAsOf, extendAsOf, "")
	require.ErrorIs(t, err, trial.ErrEndNotInFuture, "exactly `now` is not in the future")

	_, err = trial.NewExtender(nil).Extend(context.Background(), db,
		seeded.StoreID, extendAsOf.Add(-time.Hour), extendAsOf, "")
	require.ErrorIs(t, err, trial.ErrEndNotInFuture)

	_, err = trial.NewExtender(nil).Extend(context.Background(), db,
		seeded.StoreID, extendAsOf.Add(time.Second), extendAsOf, "")
	require.NoError(t, err, "one second after `now` IS in the future")
}

// Shortening to an earlier — but still future — date is allowed. EndsAt
// honours a stored value even when earlier than the derived one, and an
// operator correcting an over-generous grant is legitimate.
func TestExtend_AllowsAnEarlierButStillFutureEnd(t *testing.T) {
	db := testdb.NewDB(t, "trial_reminders", "store_subscriptions", "stores")

	seeded := seedExpiringRow(t, db, extendAsOf.Add(40*24*time.Hour), nil)
	earlier := extendAsOf.Add(5 * 24 * time.Hour)

	res, err := trial.NewExtender(nil).Extend(context.Background(), db, seeded.StoreID, earlier, extendAsOf, "")
	require.NoError(t, err)
	require.True(t, earlier.Equal(res.NewEndsAt))
}

// An unknown store is a distinct error, not a silent no-op.
func TestExtend_UnknownStore(t *testing.T) {
	db := testdb.NewDB(t, "trial_reminders", "store_subscriptions", "stores")

	_, err := trial.NewExtender(nil).Extend(context.Background(), db,
		uuid.New(), extendAsOf.Add(60*24*time.Hour), extendAsOf, "")
	require.ErrorIs(t, err, trial.ErrNoSubscription)
}

// THE enforcement test, and the assertion #287 lacked. Extend a trial past
// its derived end, run the expiry cron, and assert it survives — while an
// unextended control in the same fixture IS expired, so the test cannot
// pass by the cron doing nothing.
func TestExtend_ExtendedTrialSurvivesTheExpiryCron(t *testing.T) {
	db := testdb.NewDB(t, "trial_reminders", "store_subscriptions", "stores", "audit_logs")

	// Derived end is still in the FUTURE at extension time — a trial whose
	// effective end has already passed cannot be extended at all since
	// #286's lapsed-trial refusal (ErrNotTrialing); see
	// TestExtend_RefusesLapsedButNotYetSweptTrial. cronNow, below, is what
	// puts the ORIGINAL end in the past by the time the cron runs.
	derivedEnd := extendAsOf.Add(10 * 24 * time.Hour)
	protected := seedExpiringRow(t, db, derivedEnd, nil)
	control := seedExpiringRow(t, db, derivedEnd, nil)

	_, err := trial.NewExtender(nil).Extend(context.Background(), db,
		protected.StoreID, extendAsOf.Add(30*24*time.Hour), extendAsOf, "")
	require.NoError(t, err)

	// Past the ORIGINAL derived end (+10d) but before the NEW extended end
	// (+30d): the control (never extended) must expire at this instant,
	// while the protected row must not.
	cronNow := extendAsOf.Add(15 * 24 * time.Hour)
	cron := trial.NewExpiryCron(db, nil, nil, func() time.Time { return cronNow })
	require.NoError(t, cron.Run(context.Background()))

	var after subscription.StoreSubscription
	require.NoError(t, db.First(&after, "store_id = ?", protected.StoreID).Error)
	require.Equal(t, subscription.StatusTrialing, after.Status,
		"an extended trial must survive the cron — this is what makes the endpoint mean anything")

	var ctl subscription.StoreSubscription
	require.NoError(t, db.First(&ctl, "store_id = ?", control.StoreID).Error)
	require.Equal(t, subscription.StatusExpired, ctl.Status,
		"the unextended control MUST expire, or this test passes because the cron did nothing")
}

// The trap this closes: between a trial's effective end passing and the
// 00:15 expiry cron sweeping it to `not_trialing`, status alone still reads
// `trialing`. Without this refusal the SAME request would succeed before
// the cron runs and fail with ErrNotTrialing after it — the operator's
// answer depending on the hour. Using ErrNotTrialing here, not a distinct
// error, is what makes the answer consistent across that boundary.
func TestExtend_RefusesLapsedButNotYetSweptTrial(t *testing.T) {
	db := testdb.NewDB(t, "trial_reminders", "store_subscriptions", "stores")

	lapsed := extendAsOf.Add(-time.Hour) // effective end already passed, status still trialing
	seeded := seedExpiringRow(t, db, lapsed, nil)

	_, err := trial.NewExtender(nil).Extend(context.Background(), db,
		seeded.StoreID, extendAsOf.Add(30*24*time.Hour), extendAsOf, "")
	require.ErrorIs(t, err, trial.ErrNotTrialing)

	var untouched subscription.StoreSubscription
	require.NoError(t, db.First(&untouched, "store_id = ?", seeded.StoreID).Error)
	require.Nil(t, untouched.TrialEndsAt, "a refused extension must not write anything")
}

// A non-nil pointer to an EMPTY string is not Stripe-managed. Only a
// pointer to a real subscription id means Stripe owns the billing date —
// the empty string is what a webhook or a migration can leave behind, and
// treating it as "managed" would make every such row permanently
// unextendable through this endpoint.
func TestExtend_EmptyStringStripeSubscriptionIDIsNotStripeManaged(t *testing.T) {
	db := testdb.NewDB(t, "trial_reminders", "store_subscriptions", "stores")

	empty := ""
	seeded := seedExpiringRow(t, db, extendAsOf.Add(10*24*time.Hour),
		func(r *subscription.StoreSubscription) { r.StripeSubscriptionID = &empty })

	newEnd := extendAsOf.Add(60 * 24 * time.Hour)
	res, err := trial.NewExtender(nil).Extend(context.Background(), db, seeded.StoreID, newEnd, extendAsOf, "")
	require.NoError(t, err, "an empty-string StripeSubscriptionID must NOT be treated as stripe-managed")
	require.True(t, newEnd.Equal(res.NewEndsAt))
}
