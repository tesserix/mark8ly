//go:build integration

package planchange_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	billingstripe "github.com/mark8ly/marketplace-api/internal/billing/stripe"
	"github.com/mark8ly/marketplace-api/internal/subscription"
	"github.com/mark8ly/marketplace-api/internal/subscription/planchange"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

// fakeStripeOK returns a successful stub Stripe response.
func fakeStripeOK() *fakeStripe {
	return &fakeStripe{
		priceID: "price_test_studio_monthly",
		updateSub: &billingstripe.Subscription{
			ID:     "sub_test_integration",
			Status: "active",
		},
	}
}

func TestExecute_Upgrade_StarterToStudio_CommitsImmediately(t *testing.T) {
	db := testdb.NewDB(t, "subscription_plan_change_audit", "store_subscriptions")
	ctx := context.Background()

	tenantID := uuid.New()
	storeID := uuid.New()
	subID := "sub_upgrade_test"

	require.NoError(t, db.Create(&subscription.StoreSubscription{
		TenantID:           tenantID,
		StoreID:            storeID,
		StripeCustomerID:   "cus_test",
		StripeSubscriptionID: &subID,
		Plan:               subscription.PlanStarter,
		Status:             subscription.StatusActive,
		SubscriptionPeriod: subscription.PeriodMonthly,
		PriceTier:          subscription.PriceTierDeveloped,
		BillingCurrency:    strPtr("USD"),
	}).Error)

	subRepo := subscription.NewRepository()
	stripe := fakeStripeOK()

	o := planchange.NewOrchestrator(planchange.Deps{
		DB:               db,
		Stripe:           stripe,
		SubscriptionRepo: subRepo,
		Clock:            fixedClock{t: time.Now()},
	})

	out, err := o.Execute(ctx, planchange.Input{
		TenantID:          tenantID,
		StoreID:           storeID,
		TargetPlan:        subscription.PlanStudio,
		TargetPeriod:      subscription.PeriodMonthly,
		RequestedCurrency: "USD",
		Actor:             "user:" + uuid.NewString(),
		Reason:            "integration_test",
	})

	require.NoError(t, err)
	require.Equal(t, planchange.ResultUpgradeCommitted, out.Result)
	require.True(t, out.StripeUpdated)

	// Verify the subscription row was updated.
	got, err := subRepo.GetByStoreID(ctx, db, tenantID, storeID)
	require.NoError(t, err)
	require.Equal(t, subscription.PlanStudio, got.Plan)

	// Verify audit row was written.
	var count int64
	require.NoError(t, db.Table("subscription_plan_change_audit").
		Where("store_id = ? AND action = ?", storeID, "upgrade_committed").
		Count(&count).Error)
	require.Equal(t, int64(1), count)
}

func TestExecute_Upgrade_Rejected_WhenStatusReadOnly(t *testing.T) {
	db := testdb.NewDB(t, "store_subscriptions", "subscription_plan_change_audit")
	ctx := context.Background()

	tenantID := uuid.New()
	storeID := uuid.New()

	require.NoError(t, db.Create(&subscription.StoreSubscription{
		TenantID:           tenantID,
		StoreID:            storeID,
		StripeCustomerID:   "cus_expired",
		Plan:               subscription.PlanStarter,
		Status:             subscription.StatusExpired,
		SubscriptionPeriod: subscription.PeriodMonthly,
		PriceTier:          subscription.PriceTierDeveloped,
	}).Error)

	o := planchange.NewOrchestrator(planchange.Deps{
		DB:               db,
		Stripe:           fakeStripeOK(),
		SubscriptionRepo: subscription.NewRepository(),
		Clock:            fixedClock{t: time.Now()},
	})

	_, err := o.Execute(ctx, planchange.Input{
		TenantID:     tenantID,
		StoreID:      storeID,
		TargetPlan:   subscription.PlanStudio,
		TargetPeriod: subscription.PeriodMonthly,
		Actor:        "user:test",
	})

	require.ErrorIs(t, err, planchange.ErrSubscriptionReadOnly)
}

func TestExecute_RejectsCurrencyChange(t *testing.T) {
	db := testdb.NewDB(t, "store_subscriptions", "subscription_plan_change_audit")
	ctx := context.Background()

	tenantID := uuid.New()
	storeID := uuid.New()
	subID := "sub_currency_test"

	require.NoError(t, db.Create(&subscription.StoreSubscription{
		TenantID:             tenantID,
		StoreID:              storeID,
		StripeCustomerID:     "cus_eur",
		StripeSubscriptionID: &subID,
		Plan:                 subscription.PlanStarter,
		Status:               subscription.StatusActive,
		SubscriptionPeriod:   subscription.PeriodMonthly,
		PriceTier:            subscription.PriceTierDeveloped,
		BillingCurrency:      strPtr("EUR"),
	}).Error)

	o := planchange.NewOrchestrator(planchange.Deps{
		DB:               db,
		Stripe:           fakeStripeOK(),
		SubscriptionRepo: subscription.NewRepository(),
		Clock:            fixedClock{t: time.Now()},
	})

	_, err := o.Execute(ctx, planchange.Input{
		TenantID:          tenantID,
		StoreID:           storeID,
		TargetPlan:        subscription.PlanStudio,
		TargetPeriod:      subscription.PeriodMonthly,
		RequestedCurrency: "USD", // mismatch: stored as EUR
		Actor:             "user:test",
	})

	require.ErrorIs(t, err, planchange.ErrCurrencyLocked)
}

func strPtr(s string) *string { return &s }
