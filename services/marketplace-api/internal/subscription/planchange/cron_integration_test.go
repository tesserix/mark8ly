//go:build integration

package planchange_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	billingstripe "github.com/mark8ly/marketplace-api/internal/billing/stripe"
	"github.com/mark8ly/marketplace-api/internal/stores"
	"github.com/mark8ly/marketplace-api/internal/subscription"
	"github.com/mark8ly/marketplace-api/internal/subscription/planchange"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

// TestDowngradeRecheckCron_CommitsWhenEligible_Integration seeds a subscription
// with a ready pending downgrade, runs RunOnce against a real DB, and asserts
// that the subscription row is updated and an audit row is written.
func TestDowngradeRecheckCron_CommitsWhenEligible_Integration(t *testing.T) {
	db := testdb.NewDB(t, "subscription_plan_change_audit", "store_subscriptions", "stores")
	ctx := context.Background()

	tenantID := uuid.New()
	storeID := uuid.New()
	subID := "sub_cron_integration_test"

	// Place the effective_at in the past so the cron picks it up immediately.
	effectiveAt := time.Now().Add(-2 * time.Hour)
	targetPlan := subscription.PlanStarter
	targetPeriod := subscription.PeriodMonthly

	testdb.SeedStore(t, db, tenantID, storeID)

	require.NoError(t, db.Create(&subscription.StoreSubscription{
		TenantID:                    tenantID,
		StoreID:                     storeID,
		StripeCustomerID:            "cus_cron_test",
		StripeSubscriptionID:        &subID,
		Plan:                        subscription.PlanStudio,
		Status:                      subscription.StatusActive,
		SubscriptionPeriod:          subscription.PeriodMonthly,
		PriceTier:                   subscription.PriceTierDeveloped,
		BillingCurrency:             strPtr("USD"),
		PendingDowngradePlan:        &targetPlan,
		PendingDowngradePeriod:      &targetPeriod,
		PendingDowngradeEffectiveAt: &effectiveAt,
	}).Error)

	// Seed 1 store — within the Starter cap of 2.
	seedStores(t, db, tenantID, 1)

	subRepo := subscription.NewRepository()
	storeRepo := stores.NewRepository(db)

	cron := planchange.NewDowngradeRecheckCron(planchange.CronDeps{
		DB:               db,
		SubscriptionRepo: subRepo,
		StoreRepo:        storeRepo,
		Stripe: &fakeStripe{
			priceID: "price_starter_monthly_test",
			updateSub: &billingstripe.Subscription{
				ID:     subID,
				Status: "active",
			},
		},
		Clock: fixedClock{t: time.Now()},
	})

	require.NoError(t, cron.RunOnce(ctx))

	// The subscription plan must have been updated to Starter.
	got, err := subRepo.GetByStoreID(ctx, db, tenantID, storeID)
	require.NoError(t, err)
	require.Equal(t, subscription.PlanStarter, got.Plan, "plan must be updated to Starter")
	require.Nil(t, got.PendingDowngradePlan, "pending_downgrade_plan must be cleared")
	require.Nil(t, got.PendingDowngradeEffectiveAt, "pending_downgrade_effective_at must be cleared")

	// An audit row with action=downgrade_committed must exist.
	var auditCount int64
	require.NoError(t, db.Table("subscription_plan_change_audit").
		Where("store_id = ? AND action = ?", storeID, "downgrade_committed").
		Count(&auditCount).Error)
	require.Equal(t, int64(1), auditCount, "exactly one downgrade_committed audit row expected")
}
