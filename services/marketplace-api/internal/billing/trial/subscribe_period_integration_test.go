//go:build integration

package trial_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	billingstripe "github.com/mark8ly/marketplace-api/internal/billing/stripe"
	"github.com/mark8ly/marketplace-api/internal/billing/trial"
	"github.com/mark8ly/marketplace-api/internal/subscription"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

// The billing period Stripe returns at create time is persisted here, not
// only by whichever webhook happens to arrive later.
//
// While it was not, current_period_end stayed NULL on a real subscriber, and
// every consumer reads NULL as "already ended": lifecycle.FinalizeCron
// expires a cancel_scheduled row on its next tick, so a merchant who
// cancelled lost access immediately instead of at the end of the period they
// had paid for. See §1.5 of docs/GO-LIVE-PUNCHLIST.md.

func TestSubscribe_PersistsTheBillingPeriodFromStripe(t *testing.T) {
	db := testdb.NewDB(t, "store_subscriptions", "stores")

	row := seedStoreAndSubscription(t, db, subscription.StoreSubscription{
		StripeCustomerID:   "cus_period",
		Status:             subscription.StatusSignup,
		Plan:               subscription.PlanTrial,
		SubscriptionPeriod: subscription.PeriodMonthly,
		PriceTier:          subscription.PriceTierDeveloped,
	})

	periodStart := time.Date(2026, 9, 23, 0, 0, 0, 0, time.UTC)
	periodEnd := periodStart.AddDate(0, 0, 90)

	fake := &fakeStripe{
		priceIDResult: "price_x",
		createSubResult: &billingstripe.Subscription{
			ID:                 "sub_with_period",
			CurrentPeriodStart: periodStart.Unix(),
			CurrentPeriodEnd:   periodEnd.Unix(),
		},
	}
	s := trial.NewSubscriber(db, fake, nil)

	_, err := s.Subscribe(context.Background(), buildInput(row.TenantID, row.StoreID))
	require.NoError(t, err)

	var updated subscription.StoreSubscription
	require.NoError(t, db.Where("id = ?", row.ID).First(&updated).Error)

	require.NotNil(t, updated.CurrentPeriodEnd,
		"current_period_end must be set at subscribe time — NULL is read as 'already ended'")
	assert.Equal(t, periodEnd.Unix(), updated.CurrentPeriodEnd.UTC().Unix())

	require.NotNil(t, updated.CurrentPeriodStart)
	assert.Equal(t, periodStart.Unix(), updated.CurrentPeriodStart.UTC().Unix())
}

// A Stripe response with no period (nothing in the items array) must leave
// the columns alone rather than writing a zero time, which would date the
// period end to 1970 and expire the subscription on the next cron tick.
func TestSubscribe_LeavesThePeriodAloneWhenStripeReportsNone(t *testing.T) {
	db := testdb.NewDB(t, "store_subscriptions", "stores")

	row := seedStoreAndSubscription(t, db, subscription.StoreSubscription{
		StripeCustomerID:   "cus_noperiod",
		Status:             subscription.StatusSignup,
		Plan:               subscription.PlanTrial,
		SubscriptionPeriod: subscription.PeriodMonthly,
		PriceTier:          subscription.PriceTierDeveloped,
	})

	s := trial.NewSubscriber(db, defaultFakeStripe(), nil) // createSubResult carries no period
	_, err := s.Subscribe(context.Background(), buildInput(row.TenantID, row.StoreID))
	require.NoError(t, err)

	var updated subscription.StoreSubscription
	require.NoError(t, db.Where("id = ?", row.ID).First(&updated).Error)
	assert.Nil(t, updated.CurrentPeriodEnd, "no period from Stripe must stay NULL, never epoch zero")
	require.NotNil(t, updated.StripeSubscriptionID, "the subscription id is still persisted")
}
