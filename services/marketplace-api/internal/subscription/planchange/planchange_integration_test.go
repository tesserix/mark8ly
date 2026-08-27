//go:build integration

package planchange_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	billingstripe "github.com/mark8ly/marketplace-api/internal/billing/stripe"
	"github.com/mark8ly/marketplace-api/internal/billing/trial"
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

// seedStores inserts n active stores for the given tenant into the stores
// table. Used by downgrade preflight tests to set up over/under quota scenarios.
func seedStores(t *testing.T, db *gorm.DB, tenantID uuid.UUID, count int) {
	t.Helper()
	for i := 0; i < count; i++ {
		testdb.SeedStore(t, db, tenantID, uuid.New())
	}
}

func TestExecute_Upgrade_StarterToStudio_CommitsImmediately(t *testing.T) {
	db := testdb.NewDB(t, "subscription_plan_change_audit", "store_subscriptions")
	ctx := context.Background()

	tenantID := uuid.New()
	storeID := uuid.New()
	subID := "sub_upgrade_test"

	require.NoError(t, db.Create(&subscription.StoreSubscription{
		TenantID:             tenantID,
		StoreID:              storeID,
		StripeCustomerID:     "cus_test",
		StripeSubscriptionID: &subID,
		Plan:                 subscription.PlanStarter,
		Status:               subscription.StatusActive,
		SubscriptionPeriod:   subscription.PeriodMonthly,
		PriceTier:            subscription.PriceTierDeveloped,
		BillingCurrency:      strPtr("USD"),
	}).Error)

	subRepo := subscription.NewRepository()

	o := planchange.NewOrchestrator(planchange.Deps{
		DB:               db,
		Stripe:           fakeStripeOK(),
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

	got, err := subRepo.GetByStoreID(ctx, db, tenantID, storeID)
	require.NoError(t, err)
	require.Equal(t, subscription.PlanStudio, got.Plan)

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

func TestExecute_Downgrade_StudioToStarter_ParksPending(t *testing.T) {
	db := testdb.NewDB(t, "subscription_plan_change_audit", "store_subscriptions", "stores")
	ctx := context.Background()

	tenantID := uuid.New()
	storeID := uuid.New()
	subID := "sub_downgrade_test"
	periodEnd := time.Now().Add(15 * 24 * time.Hour)

	require.NoError(t, db.Create(&subscription.StoreSubscription{
		TenantID:             tenantID,
		StoreID:              storeID,
		StripeCustomerID:     "cus_studio",
		StripeSubscriptionID: &subID,
		Plan:                 subscription.PlanStudio,
		Status:               subscription.StatusActive,
		SubscriptionPeriod:   subscription.PeriodMonthly,
		PriceTier:            subscription.PriceTierDeveloped,
		BillingCurrency:      strPtr("USD"),
		CurrentPeriodEnd:     &periodEnd,
	}).Error)

	// Seed 1 store — within the Starter limit of 2.
	seedStores(t, db, tenantID, 1)

	subRepo := subscription.NewRepository()
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
		Actor:        "user:test",
		Reason:       "merchant_requested",
	})

	require.NoError(t, err)
	require.Equal(t, planchange.ResultDowngradeScheduled, out.Result)
	require.False(t, out.StripeUpdated)
	require.True(t, out.EffectiveAt.After(time.Now()), "effective_at should be in the future")

	got, err := subRepo.GetByStoreID(ctx, db, tenantID, storeID)
	require.NoError(t, err)
	require.NotNil(t, got.PendingDowngradePlan)
	require.Equal(t, subscription.PlanStarter, *got.PendingDowngradePlan)

	var count int64
	require.NoError(t, db.Table("subscription_plan_change_audit").
		Where("store_id = ? AND action = ?", storeID, "downgrade_scheduled").
		Count(&count).Error)
	require.Equal(t, int64(1), count)
}

func TestExecute_Downgrade_StudioToStarter_OverQuota_Rejected(t *testing.T) {
	db := testdb.NewDB(t, "subscription_plan_change_audit", "store_subscriptions", "stores")
	ctx := context.Background()

	tenantID := uuid.New()
	storeID := uuid.New()
	subID := "sub_overquota_test"
	periodEnd := time.Now().Add(15 * 24 * time.Hour)

	require.NoError(t, db.Create(&subscription.StoreSubscription{
		TenantID:             tenantID,
		StoreID:              storeID,
		StripeCustomerID:     "cus_overquota",
		StripeSubscriptionID: &subID,
		Plan:                 subscription.PlanStudio,
		Status:               subscription.StatusActive,
		SubscriptionPeriod:   subscription.PeriodMonthly,
		PriceTier:            subscription.PriceTierDeveloped,
		BillingCurrency:      strPtr("USD"),
		CurrentPeriodEnd:     &periodEnd,
	}).Error)

	// Seed 3 stores — exceeds the Starter limit of 2.
	seedStores(t, db, tenantID, 3)

	o := planchange.NewOrchestrator(planchange.Deps{
		DB:               db,
		Stripe:           fakeStripeOK(),
		SubscriptionRepo: subscription.NewRepository(),
		Clock:            fixedClock{t: time.Now()},
	})

	_, err := o.Execute(ctx, planchange.Input{
		TenantID:     tenantID,
		StoreID:      storeID,
		TargetPlan:   subscription.PlanStarter,
		TargetPeriod: subscription.PeriodMonthly,
		Actor:        "user:test",
		Reason:       "merchant_requested",
	})

	require.ErrorIs(t, err, planchange.ErrStoreCountOverQuota)

	var count int64
	require.NoError(t, db.Table("subscription_plan_change_audit").
		Where("store_id = ? AND action = ?", storeID, "downgrade_blocked_over_quota").
		Count(&count).Error)
	require.Equal(t, int64(1), count)
}

// TestExecute_InitialSubscription_TrialEndSentToStripeIsEffectiveEnd proves
// the trial_end wired into the initial Stripe CreateSubscription call is the
// EFFECTIVE end (trial.EndsAt), not created_at + 90d. This is the call site
// that closes #326 (a hardcoded 90*24h that never referenced trial.TrialDays)
// and it sends real money-affecting data to Stripe, so the assertion checks
// the exact integer rather than merely that CreateSubscription was called.
func TestExecute_InitialSubscription_TrialEndSentToStripeIsEffectiveEnd(t *testing.T) {
	db := testdb.NewDB(t, "subscription_plan_change_audit", "store_subscriptions", "stores")
	ctx := context.Background()

	tenantID := uuid.New()
	storeID := uuid.New()

	// store_subscriptions.store_id has a FK to stores — seed the parent row
	// first (mirrors seedStores below / trial's seedExpiringStore).
	require.NoError(t, db.Exec(
		`INSERT INTO stores (id, tenant_id, slug, name, country_code, currency_code, timezone, status, synced_at, storefront_customer_portal_secret)
		 VALUES (?, ?, ?, 'Test Store', 'US', 'USD', 'UTC', 'active', now(), encode(gen_random_bytes(32), 'hex'))`,
		storeID, tenantID, "test-store-"+uuid.NewString()[:8],
	).Error)

	created := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	extended := time.Date(2026, 12, 25, 0, 0, 0, 0, time.UTC)
	require.NotEqual(t, created.Add(90*24*time.Hour).Unix(), extended.Unix(),
		"fixture must distinguish the extended value from the derived one")

	require.NoError(t, db.Create(&subscription.StoreSubscription{
		TenantID:           tenantID,
		StoreID:            storeID,
		StripeCustomerID:   "cus_initial_sub",
		Plan:               subscription.PlanTrial,
		Status:             subscription.StatusSignup,
		SubscriptionPeriod: subscription.PeriodMonthly,
		PriceTier:          subscription.PriceTierDeveloped,
		BillingCurrency:    strPtr("USD"),
		CreatedAt:          created,
		TrialEndsAt:        &extended,
	}).Error)

	fake := fakeStripeOK()

	o := planchange.NewOrchestrator(planchange.Deps{
		DB:               db,
		Stripe:           fake,
		SubscriptionRepo: subscription.NewRepository(),
		Clock:            fixedClock{t: time.Now()},
	})

	out, err := o.Execute(ctx, planchange.Input{
		TenantID:          tenantID,
		StoreID:           storeID,
		TargetPlan:        subscription.PlanStarter,
		TargetPeriod:      subscription.PeriodMonthly,
		RequestedCurrency: "USD",
		Actor:             "user:" + uuid.NewString(),
		Reason:            "initial_selection",
	})

	require.NoError(t, err)
	require.Equal(t, planchange.ResultUpgradeCommitted, out.Result)

	got, err := subscription.NewRepository().GetByStoreID(ctx, db, tenantID, storeID)
	require.NoError(t, err)

	require.Equal(t, extended.UTC().Unix(), fake.lastTrialEnd,
		"trial_end sent to Stripe must be the effective end (trial.EndsAt), not created_at + 90d")
	require.Equal(t, trial.EndsAt(*got).Unix(), fake.lastTrialEnd,
		"trial_end sent to Stripe must go through the same accessor as the merchant-facing display")
}

func strPtr(s string) *string { return &s }
