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
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

// TestExpiryCron_TransitionsDay90StoresWithoutCard seeds a trialing store
// whose created_at is 91 days ago (past the 90-day cutoff) and verifies Run
// transitions it to "expired".
func TestExpiryCron_TransitionsDay90StoresWithoutCard(t *testing.T) {
	db := testdb.NewDB(t, "store_subscriptions", "stores")
	now := time.Date(2026, 7, 1, 0, 15, 0, 0, time.UTC)
	// 91 days ago — clearly past the cutoff.
	createdAt := now.AddDate(0, 0, -(trial.TrialDays + 1))

	row := seedStoreAndSubscription(t, db, subscription.StoreSubscription{
		StripeCustomerID:   "cus_expiry_91",
		Status:             subscription.StatusTrialing,
		Plan:               subscription.PlanTrial,
		SubscriptionPeriod: subscription.PeriodMonthly,
		PriceTier:          subscription.PriceTierDeveloped,
		CreatedAt:          createdAt,
	})

	cron := trial.NewExpiryCron(db, nil, nil, fixedClock(now))
	require.NoError(t, cron.Run(context.Background()))

	var updated subscription.StoreSubscription
	require.NoError(t, db.Where("id = ?", row.ID).First(&updated).Error)
	assert.Equal(t, subscription.StatusExpired, updated.Status,
		"store past day-90 with no card must be transitioned to expired")
}

// TestExpiryCron_SkipsStoresWithStripeSubscription verifies that a trialing
// store that already has a Stripe subscription ID (card was added) is not
// expired by the cron — it will convert to active when Stripe fires invoice.paid.
func TestExpiryCron_SkipsStoresWithStripeSubscription(t *testing.T) {
	db := testdb.NewDB(t, "store_subscriptions", "stores")
	now := time.Date(2026, 7, 2, 0, 15, 0, 0, time.UTC)
	createdAt := now.AddDate(0, 0, -(trial.TrialDays + 1))

	subID := "sub_has_card"
	row := seedStoreAndSubscription(t, db, subscription.StoreSubscription{
		StripeCustomerID:     "cus_expiry_carded",
		StripeSubscriptionID: &subID,
		Status:               subscription.StatusTrialing,
		Plan:                 subscription.PlanTrial,
		SubscriptionPeriod:   subscription.PeriodMonthly,
		PriceTier:            subscription.PriceTierDeveloped,
		CreatedAt:            createdAt,
	})

	cron := trial.NewExpiryCron(db, nil, nil, fixedClock(now))
	require.NoError(t, cron.Run(context.Background()))

	var updated subscription.StoreSubscription
	require.NoError(t, db.Where("id = ?", row.ID).First(&updated).Error)
	assert.Equal(t, subscription.StatusTrialing, updated.Status,
		"store with stripe_subscription_id must not be expired by cron")
}

// TestExpiryCron_Idempotent_SecondRunNoop verifies that running the expiry cron
// twice for the same store produces no error and does not change the status a
// second time (the state machine guards the trialing → expired transition).
func TestExpiryCron_Idempotent_SecondRunNoop(t *testing.T) {
	db := testdb.NewDB(t, "store_subscriptions", "stores")
	now := time.Date(2026, 7, 3, 0, 15, 0, 0, time.UTC)
	createdAt := now.AddDate(0, 0, -(trial.TrialDays + 1))

	row := seedStoreAndSubscription(t, db, subscription.StoreSubscription{
		StripeCustomerID:   "cus_expiry_idem",
		Status:             subscription.StatusTrialing,
		Plan:               subscription.PlanTrial,
		SubscriptionPeriod: subscription.PeriodMonthly,
		PriceTier:          subscription.PriceTierDeveloped,
		CreatedAt:          createdAt,
	})

	cron := trial.NewExpiryCron(db, nil, nil, fixedClock(now))

	// First run — transitions to expired.
	require.NoError(t, cron.Run(context.Background()))

	var afterFirst subscription.StoreSubscription
	require.NoError(t, db.Where("id = ?", row.ID).First(&afterFirst).Error)
	assert.Equal(t, subscription.StatusExpired, afterFirst.Status)

	// Second run — store is now expired, not trialing; query selects nothing.
	require.NoError(t, cron.Run(context.Background()),
		"second run must not return an error")

	var afterSecond subscription.StoreSubscription
	require.NoError(t, db.Where("id = ?", row.ID).First(&afterSecond).Error)
	assert.Equal(t, subscription.StatusExpired, afterSecond.Status,
		"second run must leave status as expired")
}
