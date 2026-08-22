//go:build integration

package planchange_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	billingstripe "github.com/mark8ly/marketplace-api/internal/billing/stripe"
	"github.com/mark8ly/marketplace-api/internal/subscription"
	"github.com/mark8ly/marketplace-api/internal/subscription/planchange"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

// TestPeriodSwitch_MonthlyToAnnual_Pro_CommitsImmediately verifies that a
// Monthly→Annual period switch on the Pro plan:
//   - returns ResultPeriodSwitchCommitted
//   - resolves the correct annual price ID and passes it to Stripe
//   - persists SubscriptionPeriod = PeriodAnnual in the DB
func TestPeriodSwitch_MonthlyToAnnual_Pro_CommitsImmediately(t *testing.T) {
	db := testdb.NewDB(t, "subscription_plan_change_audit", "store_subscriptions")
	ctx := context.Background()

	tenantID := uuid.New()
	storeID := uuid.New()
	subID := "sub_period_switch_pro"

	require.NoError(t, db.Create(&subscription.StoreSubscription{
		TenantID:             tenantID,
		StoreID:              storeID,
		StripeCustomerID:     "cus_pro_monthly",
		StripeSubscriptionID: &subID,
		Plan:                 subscription.PlanPro,
		Status:               subscription.StatusActive,
		SubscriptionPeriod:   subscription.PeriodMonthly,
		PriceTier:            subscription.PriceTierDeveloped,
		BillingCurrency:      strPtr("USD"),
	}).Error)

	// Wire a recordingStripe so we can assert which price ID was sent.
	// PriceIDFor returns priceID unconditionally for any (plan, period, currency, tier).
	const expectedPriceID = "price_pro_annual_usd"
	rs := &recordingStripe{
		fakeStripe: fakeStripe{
			priceID: expectedPriceID,
			updateSub: &billingstripe.Subscription{
				ID:     subID,
				Status: "active",
			},
		},
	}

	subRepo := subscription.NewRepository()
	o := planchange.NewOrchestrator(planchange.Deps{
		DB:               db,
		Stripe:           rs,
		SubscriptionRepo: subRepo,
		Clock:            fixedClock{t: time.Now()},
	})

	out, err := o.Execute(ctx, planchange.Input{
		TenantID:          tenantID,
		StoreID:           storeID,
		TargetPlan:        subscription.PlanPro, // same plan — period switch only
		TargetPeriod:      subscription.PeriodAnnual,
		RequestedCurrency: "USD",
		Actor:             "user:merchant",
		Reason:            "period_switch_test",
	})

	require.NoError(t, err)
	assert.Equal(t, planchange.ResultPeriodSwitchCommitted, out.Result)
	assert.True(t, out.StripeUpdated)

	// The annual price ID must have been passed to Stripe.
	require.True(t, rs.updateCalled, "Stripe.UpdateSubscription must be called for a period switch")
	assert.Equal(t, expectedPriceID, rs.lastParams.PriceID,
		"Stripe must receive the annual price ID resolved by PriceIDFor")
	assert.Equal(t, billingstripe.ProrationAlwaysInvoice, rs.lastParams.ProrationBehavior,
		"period upgrades must use ProrationAlwaysInvoice")

	// The DB row must reflect the new period.
	got, err := subRepo.GetByStoreID(ctx, db, tenantID, storeID)
	require.NoError(t, err)
	assert.Equal(t, subscription.PeriodAnnual, got.SubscriptionPeriod,
		"SubscriptionPeriod must be updated to Annual in the DB")
	assert.Equal(t, subscription.PlanPro, got.Plan, "plan must remain Pro after period switch")

	// Exactly one period_switch_committed audit row must exist.
	var count int64
	require.NoError(t, db.Table("subscription_plan_change_audit").
		Where("store_id = ? AND action = ?", storeID, "period_switch_committed").
		Count(&count).Error)
	assert.Equal(t, int64(1), count, "exactly one period_switch_committed audit row expected")
}
