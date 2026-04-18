//go:build integration

package trial_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/billing/trial"
	"github.com/mark8ly/marketplace-api/internal/subscription"
)

// fixedClock returns a clock func that always returns t.
func fixedClock(t time.Time) func() time.Time { return func() time.Time { return t } }

// TestBannerCron_SetsDay60State_SkipsOtherDays seeds three rows at days
// 59, 60, and 61 relative to "now", runs the cron, and asserts only the
// day-60 row advances to "day_60".
func TestBannerCron_SetsDay60State_SkipsOtherDays(t *testing.T) {
	db := openIntegrationDB(t)
	now := time.Date(2026, 3, 1, 9, 0, 0, 0, time.UTC)

	day60 := now.AddDate(0, 0, -60)
	day59 := now.AddDate(0, 0, -59)
	day61 := now.AddDate(0, 0, -61)

	rowAt60 := seedSubscription(t, db, subscription.StoreSubscription{
		StripeCustomerID:   "cus_banner_60",
		Status:             subscription.StatusTrialing,
		Plan:               subscription.PlanTrial,
		SubscriptionPeriod: subscription.PeriodMonthly,
		PriceTier:          subscription.PriceTierDeveloped,
		CreatedAt:          day60,
	})
	rowAt59 := seedSubscription(t, db, subscription.StoreSubscription{
		StripeCustomerID:   "cus_banner_59",
		Status:             subscription.StatusTrialing,
		Plan:               subscription.PlanTrial,
		SubscriptionPeriod: subscription.PeriodMonthly,
		PriceTier:          subscription.PriceTierDeveloped,
		CreatedAt:          day59,
	})
	rowAt61 := seedSubscription(t, db, subscription.StoreSubscription{
		StripeCustomerID:   "cus_banner_61",
		Status:             subscription.StatusTrialing,
		Plan:               subscription.PlanTrial,
		SubscriptionPeriod: subscription.PeriodMonthly,
		PriceTier:          subscription.PriceTierDeveloped,
		CreatedAt:          day61,
	})

	cron := trial.NewBannerCron(db, nil, fixedClock(now))
	require.NoError(t, cron.Run(context.Background()))

	// day-60 row should have banner state "day_60".
	var state60 *string
	require.NoError(t, db.Raw(
		"SELECT trial_banner_state FROM store_subscriptions WHERE id = ?", rowAt60.ID,
	).Scan(&state60).Error)
	require.NotNil(t, state60, "day-60 row should have trial_banner_state set")
	assert.Equal(t, "day_60", *state60)

	// day-59 row: no banner state (not yet at threshold).
	var state59 *string
	require.NoError(t, db.Raw(
		"SELECT trial_banner_state FROM store_subscriptions WHERE id = ?", rowAt59.ID,
	).Scan(&state59).Error)
	assert.Nil(t, state59, "day-59 row must not have trial_banner_state set")

	// day-61 row: no day-60 match but may hit day-61 if it exists; since we
	// only define 60/75/85 we expect no state change for day 61.
	var state61 *string
	require.NoError(t, db.Raw(
		"SELECT trial_banner_state FROM store_subscriptions WHERE id = ?", rowAt61.ID,
	).Scan(&state61).Error)
	assert.Nil(t, state61, "day-61 row must not match any banner threshold")
}

// TestBannerCron_Idempotent_SameDay runs the cron twice at the same simulated
// day and asserts the second run is a no-op (state stays "day_60", not promoted).
func TestBannerCron_Idempotent_SameDay(t *testing.T) {
	db := openIntegrationDB(t)
	now := time.Date(2026, 4, 1, 9, 0, 0, 0, time.UTC)
	createdAt := now.AddDate(0, 0, -60)

	row := seedSubscription(t, db, subscription.StoreSubscription{
		StripeCustomerID:   "cus_idem_60",
		Status:             subscription.StatusTrialing,
		Plan:               subscription.PlanTrial,
		SubscriptionPeriod: subscription.PeriodMonthly,
		PriceTier:          subscription.PriceTierDeveloped,
		CreatedAt:          createdAt,
	})

	cron := trial.NewBannerCron(db, nil, fixedClock(now))

	// First run.
	require.NoError(t, cron.Run(context.Background()))
	// Second run — same clock, same day.
	require.NoError(t, cron.Run(context.Background()))

	var state *string
	require.NoError(t, db.Raw(
		"SELECT trial_banner_state FROM store_subscriptions WHERE id = ?", row.ID,
	).Scan(&state).Error)
	require.NotNil(t, state)
	assert.Equal(t, "day_60", *state, "second run must not change the banner state")
}

// TestBannerCron_SkipsStoresWithStripeSubscriptionID verifies that a trialing
// store that already has a Stripe subscription (card added late) is not touched
// by the banner cron.
func TestBannerCron_SkipsStoresWithStripeSubscriptionID(t *testing.T) {
	db := openIntegrationDB(t)
	now := time.Date(2026, 5, 1, 9, 0, 0, 0, time.UTC)
	createdAt := now.AddDate(0, 0, -60)

	subID := "sub_already_carded"
	row := seedSubscription(t, db, subscription.StoreSubscription{
		StripeCustomerID:     "cus_carded",
		StripeSubscriptionID: &subID,
		Status:               subscription.StatusTrialing,
		Plan:                 subscription.PlanTrial,
		SubscriptionPeriod:   subscription.PeriodMonthly,
		PriceTier:            subscription.PriceTierDeveloped,
		CreatedAt:            createdAt,
	})

	cron := trial.NewBannerCron(db, nil, fixedClock(now))
	require.NoError(t, cron.Run(context.Background()))

	var state *string
	require.NoError(t, db.Raw(
		"SELECT trial_banner_state FROM store_subscriptions WHERE id = ?", row.ID,
	).Scan(&state).Error)
	assert.Nil(t, state, "store with stripe_subscription_id must not get banner state")
}

// TestBannerCron_AdvancesStateAcrossThresholds verifies that a store at day 75
// gets state "day_75" directly, skipping "day_60" entirely, because each query
// selects only stores whose created_at matches the specific day window.
func TestBannerCron_AdvancesStateAcrossThresholds(t *testing.T) {
	db := openIntegrationDB(t)
	now := time.Date(2026, 6, 1, 9, 0, 0, 0, time.UTC)
	createdAt := now.AddDate(0, 0, -75)

	row := seedSubscription(t, db, subscription.StoreSubscription{
		StripeCustomerID:   "cus_day75",
		Status:             subscription.StatusTrialing,
		Plan:               subscription.PlanTrial,
		SubscriptionPeriod: subscription.PeriodMonthly,
		PriceTier:          subscription.PriceTierDeveloped,
		CreatedAt:          createdAt,
	})

	cron := trial.NewBannerCron(db, nil, fixedClock(now))
	require.NoError(t, cron.Run(context.Background()))

	var state *string
	require.NoError(t, db.Raw(
		"SELECT trial_banner_state FROM store_subscriptions WHERE id = ?", row.ID,
	).Scan(&state).Error)
	require.NotNil(t, state)
	assert.Equal(t, "day_75", *state, "day-75 store must advance straight to day_75 state")
}
