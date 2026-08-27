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
	"github.com/mark8ly/marketplace-api/internal/stores"
	"github.com/mark8ly/marketplace-api/internal/subscription"
	"github.com/mark8ly/marketplace-api/internal/subscription/planchange"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

// TestCriterion39_DowngradeBlockedOverQuota_AtPeriodEnd is the end-to-end smoke
// test for spec criterion #39: a downgrade is scheduled while the merchant is
// under quota, the merchant then adds stores during the grace window bringing
// the count over the target plan's cap, and when the cron fires at period-end
// it blocks the downgrade, keeps the merchant on the higher plan, writes a
// downgrade_blocked_over_quota audit row, and fires the notifier.
func TestCriterion39_DowngradeBlockedOverQuota_AtPeriodEnd(t *testing.T) {
	db := testdb.NewDB(t, "subscription_plan_change_audit", "store_subscriptions", "stores")
	ctx := context.Background()

	tenantID := uuid.New()
	storeID := uuid.New()
	subID := "sub_criterion39_test"

	// Period ends in the future so the downgrade scheduling step succeeds.
	periodEnd := time.Now().Add(15 * 24 * time.Hour)
	targetPlan := subscription.PlanStarter
	targetPeriod := subscription.PeriodMonthly

	// Seed 1 store — under Starter cap of 2, so scheduling succeeds.
	seedStores(t, db, tenantID, 1)
	testdb.SeedStore(t, db, tenantID, storeID)

	require.NoError(t, db.Create(&subscription.StoreSubscription{
		TenantID:             tenantID,
		StoreID:              storeID,
		StripeCustomerID:     "cus_criterion39",
		StripeSubscriptionID: &subID,
		Plan:                 subscription.PlanStudio,
		Status:               subscription.StatusActive,
		SubscriptionPeriod:   subscription.PeriodMonthly,
		PriceTier:            subscription.PriceTierDeveloped,
		BillingCurrency:      strPtr("USD"),
		CurrentPeriodEnd:     &periodEnd,
	}).Error)

	subRepo := subscription.NewRepository()

	// Step 1: schedule the downgrade while under quota.
	o := planchange.NewOrchestrator(planchange.Deps{
		DB:               db,
		Stripe:           fakeStripeOK(),
		SubscriptionRepo: subRepo,
		Clock:            fixedClock{t: time.Now()},
	})

	out, err := o.Execute(ctx, planchange.Input{
		TenantID:     tenantID,
		StoreID:      storeID,
		TargetPlan:   subscription.PlanStarter,
		TargetPeriod: subscription.PeriodMonthly,
		Actor:        "user:merchant",
		Reason:       "criterion_39_test",
	})
	require.NoError(t, err)
	require.Equal(t, planchange.ResultDowngradeScheduled, out.Result)

	// Step 2: merchant adds 3 more stores during the grace window, now over quota.
	seedStores(t, db, tenantID, 3)

	// Step 3: cron fires — effective_at is in the past (backdated).
	pastEffective := time.Now().Add(-time.Hour)
	require.NoError(t, db.Model(&subscription.StoreSubscription{}).
		Where("store_id = ?", storeID).
		Updates(map[string]any{
			"pending_downgrade_plan":         targetPlan,
			"pending_downgrade_period":       targetPeriod,
			"pending_downgrade_effective_at": pastEffective,
		}).Error)

	notifier := &fakeNotifier{}
	rs := &recordingStripe{
		fakeStripe: fakeStripe{
			priceID: "price_starter_monthly_criterion39",
			updateSub: &billingstripe.Subscription{
				ID:     subID,
				Status: "active",
			},
		},
	}

	cron := planchange.NewDowngradeRecheckCron(planchange.CronDeps{
		DB:               db,
		SubscriptionRepo: subRepo,
		StoreRepo:        stores.NewRepository(db),
		Stripe:           rs,
		Notifier:         notifier,
		Clock:            fixedClock{t: time.Now()},
	})

	require.NoError(t, cron.RunOnce(ctx))

	// Assertions: plan must remain Studio, no Stripe call, notifier fired once.
	got, err := subRepo.GetByStoreID(ctx, db, tenantID, storeID)
	require.NoError(t, err)

	assert.Equal(t, subscription.PlanStudio, got.Plan, "plan must remain Studio after block")
	assert.Equal(t, subscription.StatusActive, got.Status)
	assert.Nil(t, got.PendingDowngradePlan, "pending downgrade must be cleared after block")

	assert.False(t, rs.updateCalled, "Stripe.UpdateSubscription must NOT be called when blocked")

	require.Equal(t, 1, notifier.called, "notifier must be called exactly once")
	assert.Equal(t, "store_count_over_quota", notifier.lastReason)

	var count int64
	require.NoError(t, db.Table("subscription_plan_change_audit").
		Where("store_id = ? AND action = ?", storeID, "downgrade_blocked_over_quota").
		Count(&count).Error)
	assert.Equal(t, int64(1), count, "exactly one downgrade_blocked_over_quota audit row expected")
}
