//go:build integration

package cancel_test

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/subscription"
	"github.com/mark8ly/marketplace-api/internal/subscription/cancel"
	"github.com/mark8ly/marketplace-api/internal/subscription/lifecycle"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

// recordingCanceller stands in for Stripe and reports the period end Stripe
// would hold. It records the subscription ids it was asked about, so a test
// can prove the call was made at all — the whole defect was that it never was.
type recordingCanceller struct {
	periodEnd time.Time

	cancelled []string
	resumed   []string
}

func (r *recordingCanceller) CancelAtPeriodEnd(_ context.Context, subscriptionID string) (cancel.StripeState, error) {
	r.cancelled = append(r.cancelled, subscriptionID)
	return cancel.StripeState{CancelAtPeriodEnd: true, CurrentPeriodEnd: r.periodEnd}, nil
}

func (r *recordingCanceller) Resume(_ context.Context, subscriptionID string) (cancel.StripeState, error) {
	r.resumed = append(r.resumed, subscriptionID)
	return cancel.StripeState{CancelAtPeriodEnd: false, CurrentPeriodEnd: r.periodEnd}, nil
}

func seedCancellableSub(t *testing.T, db *gorm.DB, status subscription.SubscriptionStatus, stripeSubID *string) subscription.StoreSubscription {
	t.Helper()

	row := subscription.StoreSubscription{
		ID:                   uuid.New(),
		TenantID:             uuid.New(),
		StoreID:              uuid.New(),
		StripeCustomerID:     "cus_" + uuid.NewString()[:8],
		StripeSubscriptionID: stripeSubID,
		Plan:                 subscription.PlanStudio,
		Status:               status,
		SubscriptionPeriod:   subscription.PeriodMonthly,
		PriceTier:            subscription.PriceTierDeveloped,
		// Deliberately NULL: this is the state a subscriber was left in
		// while nothing wrote the period at subscribe time.
		CurrentPeriodEnd: nil,
	}
	testdb.SeedStore(t, db, row.TenantID, row.StoreID)
	require.NoError(t, db.Create(&row).Error)
	t.Cleanup(func() {
		db.Unscoped().Delete(&subscription.StoreSubscription{}, "id = ?", row.ID)
	})
	return row
}

func stringPtr(s string) *string { return &s }

// TestCancel_SchedulesAtStripeAndPersistsThePeriodEnd is the whole §1.5 fix in
// one pass: Stripe is told, the date it reports is written to the row, and the
// merchant is told that date.
func TestCancel_SchedulesAtStripeAndPersistsThePeriodEnd(t *testing.T) {
	db := testdb.NewDB(t, "store_subscriptions", "stores")

	row := seedCancellableSub(t, db, subscription.StatusActive, stringPtr("sub_live_1"))
	periodEnd := time.Now().Add(18 * 24 * time.Hour).UTC().Truncate(time.Second)
	stripe := &recordingCanceller{periodEnd: periodEnd}

	svc := cancel.NewService(db, subscription.NewRepository(), nil, slog.Default()).WithStripe(stripe)

	out, err := svc.Cancel(context.Background(), cancel.Input{
		TenantID:     row.TenantID,
		StoreID:      row.StoreID,
		Actor:        "user:" + uuid.NewString(),
		SurveyReason: "too expensive",
	})
	require.NoError(t, err)

	require.Equal(t, []string{"sub_live_1"}, stripe.cancelled,
		"the cancellation must reach Stripe — recording it locally is what left merchants being charged")
	assert.Equal(t, string(subscription.StatusCancelScheduled), out.Status)
	assert.Equal(t, periodEnd.Format("2006-01-02T15:04:05Z"), out.CancelsAt,
		"the merchant is told the date Stripe holds, not an empty string")

	var updated subscription.StoreSubscription
	require.NoError(t, db.Where("id = ?", row.ID).First(&updated).Error)
	assert.Equal(t, subscription.StatusCancelScheduled, updated.Status)
	assert.True(t, updated.CancelAtPeriodEnd)
	require.NotNil(t, updated.CurrentPeriodEnd)
	assert.Equal(t, periodEnd.Unix(), updated.CurrentPeriodEnd.UTC().Unix())
}

// TestCancel_KeepsAccessUntilThePeriodEnds is the regression the merchant
// actually feels. FinalizeCron treats a NULL current_period_end as "already
// ended", so a cancelled subscriber used to lose access on the next tick —
// while Stripe, never told, kept billing them.
func TestCancel_KeepsAccessUntilThePeriodEnds(t *testing.T) {
	db := testdb.NewDB(t, "store_subscriptions", "stores")

	row := seedCancellableSub(t, db, subscription.StatusActive, stringPtr("sub_live_2"))
	periodEnd := time.Now().Add(30 * 24 * time.Hour).UTC()
	stripe := &recordingCanceller{periodEnd: periodEnd}

	svc := cancel.NewService(db, subscription.NewRepository(), nil, slog.Default()).WithStripe(stripe)
	_, err := svc.Cancel(context.Background(), cancel.Input{
		TenantID: row.TenantID,
		StoreID:  row.StoreID,
		Actor:    "user:" + uuid.NewString(),
	})
	require.NoError(t, err)

	// The cron runs now; the period has not ended.
	cron := lifecycle.NewFinalizeCron(db, nil, slog.Default(), func() time.Time { return time.Now().UTC() })
	require.NoError(t, cron.Run(context.Background()))

	var after subscription.StoreSubscription
	require.NoError(t, db.Where("id = ?", row.ID).First(&after).Error)
	assert.Equal(t, subscription.StatusCancelScheduled, after.Status,
		"a merchant who cancelled keeps the period they paid for")

	// And it does expire once the period has passed.
	late := lifecycle.NewFinalizeCron(db, nil, slog.Default(), func() time.Time {
		return periodEnd.Add(time.Hour)
	})
	require.NoError(t, late.Run(context.Background()))

	require.NoError(t, db.Where("id = ?", row.ID).First(&after).Error)
	assert.Equal(t, subscription.StatusExpired, after.Status)
}

// TestSaveOffer_ClearsTheScheduleAtStripe — un-cancelling must reverse the
// Stripe schedule, or the merchant is still cancelled on the date they were
// just told they would not be.
func TestSaveOffer_ClearsTheScheduleAtStripe(t *testing.T) {
	db := testdb.NewDB(t, "store_subscriptions", "stores")

	row := seedCancellableSub(t, db, subscription.StatusCancelScheduled, stringPtr("sub_live_3"))
	require.NoError(t, db.Model(&subscription.StoreSubscription{}).
		Where("id = ?", row.ID).
		Update("cancel_at_period_end", true).Error)

	stripe := &recordingCanceller{periodEnd: time.Now().Add(12 * 24 * time.Hour).UTC()}
	svc := cancel.NewService(db, subscription.NewRepository(), nil, slog.Default()).WithStripe(stripe)

	out, err := svc.Cancel(context.Background(), cancel.Input{
		TenantID:        row.TenantID,
		StoreID:         row.StoreID,
		Actor:           "user:" + uuid.NewString(),
		AcceptSaveOffer: true,
	})
	require.NoError(t, err)

	require.Equal(t, []string{"sub_live_3"}, stripe.resumed)
	assert.Equal(t, string(subscription.StatusActive), out.Status)

	var updated subscription.StoreSubscription
	require.NoError(t, db.Where("id = ?", row.ID).First(&updated).Error)
	assert.Equal(t, subscription.StatusActive, updated.Status)
	assert.False(t, updated.CancelAtPeriodEnd, "the local flag must not outlive the reversal")
}

// TestCancel_WithoutAStripeSubscriptionStaysLocal — a trial that never added
// a card has no Stripe subscription to cancel, and must not be blocked on one.
func TestCancel_WithoutAStripeSubscriptionStaysLocal(t *testing.T) {
	db := testdb.NewDB(t, "store_subscriptions", "stores")

	row := seedCancellableSub(t, db, subscription.StatusTrialing, nil)
	stripe := &recordingCanceller{periodEnd: time.Now().Add(24 * time.Hour).UTC()}

	svc := cancel.NewService(db, subscription.NewRepository(), nil, slog.Default()).WithStripe(stripe)
	out, err := svc.Cancel(context.Background(), cancel.Input{
		TenantID: row.TenantID,
		StoreID:  row.StoreID,
		Actor:    "user:" + uuid.NewString(),
	})
	require.NoError(t, err)

	assert.Empty(t, stripe.cancelled, "there is no Stripe subscription to cancel")
	assert.Equal(t, string(subscription.StatusCancelScheduled), out.Status)
}
