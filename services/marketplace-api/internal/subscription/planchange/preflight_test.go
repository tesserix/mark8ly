package planchange_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	billingstripe "github.com/mark8ly/marketplace-api/internal/billing/stripe"
	"github.com/mark8ly/marketplace-api/internal/plangate"
	"github.com/mark8ly/marketplace-api/internal/stores"
	"github.com/mark8ly/marketplace-api/internal/subscription"
	"github.com/mark8ly/marketplace-api/internal/subscription/planchange"
)

// ---------------------------------------------------------------------------
// Fakes
// ---------------------------------------------------------------------------

// fakeStoreRepo implements stores.Repository for preflight unit tests.
// Only CountActiveOrSoftDeletedRestorable, ListActiveOrSoftDeletedRestorable,
// and InFlightOrderCount return meaningful values; all other methods are stubs.
type fakeStoreRepo struct {
	count      int
	storeRows  []stores.Store
	orderCount int
}

func (r *fakeStoreRepo) GetByIDForTenant(_ context.Context, _, _ string) (*stores.Store, error) {
	return nil, nil
}
func (r *fakeStoreRepo) GetBySlug(_ context.Context, _ string) (*stores.Store, error) {
	return nil, nil
}
func (r *fakeStoreRepo) ListForTenant(_ context.Context, _ string) ([]stores.Store, error) {
	return nil, nil
}
func (r *fakeStoreRepo) Upsert(_ context.Context, _ *stores.Store) error { return nil }
func (r *fakeStoreRepo) GetProductsWatermark(_ context.Context, _ string) (time.Time, error) {
	return time.Time{}, nil
}
func (r *fakeStoreRepo) CountActiveOrSoftDeletedRestorable(_ context.Context, _ uuid.UUID) (int, error) {
	return r.count, nil
}
func (r *fakeStoreRepo) CountActiveOrSoftDeletedRestorableTx(_ context.Context, _ *gorm.DB, _ uuid.UUID) (int, error) {
	return r.count, nil
}
func (r *fakeStoreRepo) ListActiveOrSoftDeletedRestorable(_ context.Context, _ uuid.UUID) ([]stores.Store, error) {
	return r.storeRows, nil
}
func (r *fakeStoreRepo) InFlightOrderCount(_ context.Context, _ uuid.UUID) (int, error) {
	return r.orderCount, nil
}

// activeSubscription builds a minimal StoreSubscription in Active status on
// the given plan/period with a billing currency and period-end 30 days out.
func activeSubscription(plan subscription.SubscriptionPlan, period subscription.SubscriptionPeriod, currency string) *subscription.StoreSubscription {
	periodEnd := time.Now().Add(30 * 24 * time.Hour)
	return &subscription.StoreSubscription{
		ID:                 uuid.New(),
		TenantID:           uuid.New(),
		StoreID:            uuid.New(),
		Plan:               plan,
		Status:             subscription.StatusActive,
		SubscriptionPeriod: period,
		BillingCurrency:    &currency,
		CurrentPeriodEnd:   &periodEnd,
		PriceTier:          subscription.PriceTierDeveloped,
	}
}

// newOrchestrator constructs a planchange.Orchestrator wired to the supplied
// fakes. DB is nil — Preflight does not open a transaction.
func newOrchestrator(sub *subscription.StoreSubscription, sr *fakeStoreRepo) *planchange.Orchestrator {
	return planchange.NewOrchestrator(planchange.Deps{
		DB:               nil, // Preflight reads via GetByStoreID(ctx, DB, …) — fakeSubRepo ignores it
		Stripe:           &fakeStripe{priceID: "price_test"},
		SubscriptionRepo: &fakeSubRepo{sub: sub},
		StoreRepo:        sr,
		Clock:            fixedClock{t: time.Now()},
	})
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

func TestPreflight_Upgrade_AllowImmediate(t *testing.T) {
	sub := activeSubscription(subscription.PlanStarter, subscription.PeriodMonthly, "USD")
	o := newOrchestrator(sub, &fakeStoreRepo{count: 1})

	report, err := o.Preflight(context.Background(), planchange.Input{
		TenantID:          sub.TenantID,
		StoreID:           sub.StoreID,
		TargetPlan:        subscription.PlanStudio,
		TargetPeriod:      subscription.PeriodMonthly,
		RequestedCurrency: "USD",
	})

	require.NoError(t, err)
	require.Equal(t, planchange.DecisionAllowUpgradeNow, report.Decision)
	// Starter=2 stores, Studio=5 stores → delta +3
	require.Equal(t, 3, report.CurrentPlanDiff.StoresDelta)
	require.False(t, report.EffectiveAt.IsZero())
}

func TestPreflight_PeriodSwitch(t *testing.T) {
	sub := activeSubscription(subscription.PlanStarter, subscription.PeriodMonthly, "USD")
	o := newOrchestrator(sub, &fakeStoreRepo{count: 1})

	report, err := o.Preflight(context.Background(), planchange.Input{
		TenantID:     sub.TenantID,
		StoreID:      sub.StoreID,
		TargetPlan:   subscription.PlanStarter, // same plan
		TargetPeriod: subscription.PeriodAnnual, // period switch
	})

	require.NoError(t, err)
	require.Equal(t, planchange.DecisionAllowPeriodSwitch, report.Decision)
	require.False(t, report.EffectiveAt.IsZero())
}

func TestPreflight_NoChange(t *testing.T) {
	sub := activeSubscription(subscription.PlanStarter, subscription.PeriodMonthly, "USD")
	o := newOrchestrator(sub, &fakeStoreRepo{count: 1})

	report, err := o.Preflight(context.Background(), planchange.Input{
		TenantID:     sub.TenantID,
		StoreID:      sub.StoreID,
		TargetPlan:   subscription.PlanStarter,  // identical
		TargetPeriod: subscription.PeriodMonthly, // identical
	})

	require.NoError(t, err)
	require.Equal(t, planchange.DecisionNoOp, report.Decision)
}

func TestPreflight_ReadOnly_Blocks(t *testing.T) {
	periodEnd := time.Now().Add(30 * 24 * time.Hour)
	sub := &subscription.StoreSubscription{
		ID:                 uuid.New(),
		TenantID:           uuid.New(),
		StoreID:            uuid.New(),
		Plan:               subscription.PlanStarter,
		Status:             subscription.StatusExpired, // read-only
		SubscriptionPeriod: subscription.PeriodMonthly,
		CurrentPeriodEnd:   &periodEnd,
	}
	o := newOrchestrator(sub, &fakeStoreRepo{count: 1})

	report, err := o.Preflight(context.Background(), planchange.Input{
		TenantID:     sub.TenantID,
		StoreID:      sub.StoreID,
		TargetPlan:   subscription.PlanStudio,
		TargetPeriod: subscription.PeriodMonthly,
	})

	require.NoError(t, err)
	require.Equal(t, planchange.DecisionBlockReadOnly, report.Decision)
}

func TestPreflight_CurrencyMismatch_Blocks(t *testing.T) {
	sub := activeSubscription(subscription.PlanStarter, subscription.PeriodMonthly, "USD")
	o := newOrchestrator(sub, &fakeStoreRepo{count: 1})

	report, err := o.Preflight(context.Background(), planchange.Input{
		TenantID:          sub.TenantID,
		StoreID:           sub.StoreID,
		TargetPlan:        subscription.PlanStudio,
		TargetPeriod:      subscription.PeriodMonthly,
		RequestedCurrency: "INR", // mismatch: sub has USD
	})

	require.NoError(t, err)
	require.Equal(t, planchange.DecisionBlockCurrency, report.Decision)
}

func TestPreflight_Downgrade_OverQuota_Blocks(t *testing.T) {
	sub := activeSubscription(subscription.PlanStudio, subscription.PeriodMonthly, "USD")

	// Build 5 store rows to surface in the report.
	storeRows := make([]stores.Store, 5)
	for i := range storeRows {
		storeRows[i] = stores.Store{
			ID:       uuid.NewString(),
			TenantID: sub.TenantID.String(),
			Slug:     "store-" + uuid.NewString()[:8],
			Name:     "Store",
			Status:   stores.StatusActive,
		}
	}
	sr := &fakeStoreRepo{count: 5, storeRows: storeRows, orderCount: 0}
	o := newOrchestrator(sub, sr)

	report, err := o.Preflight(context.Background(), planchange.Input{
		TenantID:     sub.TenantID,
		StoreID:      sub.StoreID,
		TargetPlan:   subscription.PlanStarter, // limit = 2
		TargetPeriod: subscription.PeriodMonthly,
	})

	require.NoError(t, err)
	require.Equal(t, planchange.DecisionBlockStoreCount, report.Decision)
	require.Equal(t, 5, report.StoreCount)
	require.Equal(t, plangate.Limit(subscription.PlanStarter, plangate.FeatureStores), report.TargetStoreLimit) // 2
	require.Len(t, report.Stores, 5)
}

func TestPreflight_Downgrade_UnderQuota_Allows(t *testing.T) {
	sub := activeSubscription(subscription.PlanStudio, subscription.PeriodMonthly, "USD")
	sr := &fakeStoreRepo{count: 2} // within Starter limit of 2
	o := newOrchestrator(sub, sr)

	report, err := o.Preflight(context.Background(), planchange.Input{
		TenantID:     sub.TenantID,
		StoreID:      sub.StoreID,
		TargetPlan:   subscription.PlanStarter,
		TargetPeriod: subscription.PeriodMonthly,
	})

	require.NoError(t, err)
	require.Equal(t, planchange.DecisionAllowDowngradeAtPeriodEnd, report.Decision)
	require.False(t, report.EffectiveAt.IsZero())
	// EffectiveAt should be the period-end (30 days out), not now.
	require.True(t, report.EffectiveAt.After(time.Now()))
}

// TestPreflight_PlanDiff_Pro_Unlimited verifies that transitioning to a plan
// where ImagesPerProduct is Unlimited produces a delta of plangate.Unlimited
// (-1), not a raw numeric difference (which would be meaningless).
func TestPreflight_PlanDiff_Pro_Unlimited(t *testing.T) {
	sub := activeSubscription(subscription.PlanStarter, subscription.PeriodMonthly, "USD")
	o := newOrchestrator(sub, &fakeStoreRepo{count: 1})

	report, err := o.Preflight(context.Background(), planchange.Input{
		TenantID:     sub.TenantID,
		StoreID:      sub.StoreID,
		TargetPlan:   subscription.PlanPro,
		TargetPeriod: subscription.PeriodMonthly,
	})

	require.NoError(t, err)
	require.Equal(t, planchange.DecisionAllowUpgradeNow, report.Decision)

	// Pro has Unlimited images — delta must be plangate.Unlimited (-1), not
	// a raw "Unlimited - 25" nonsense value.
	require.Equal(t, plangate.Unlimited, report.CurrentPlanDiff.ImagesPerProductDelta,
		"expected Unlimited sentinel (-1) for images delta when target plan is Pro")

	// Stores: Starter=2, Pro=10 → delta +8.
	require.Equal(t, 8, report.CurrentPlanDiff.StoresDelta)
}

// TestPreflight_Fakes_ImplementInterfaces is a compile-time check that the
// fakes defined in this file fully satisfy the required interfaces. This test
// always passes at runtime; it fails at compile time if an interface method is
// missing from a fake.
func TestPreflight_Fakes_ImplementInterfaces(t *testing.T) {
	var _ stores.Repository = (*fakeStoreRepo)(nil)
	var _ subscription.Repository = (*fakeSubRepo)(nil)
	var _ planchange.StripeClient = (*fakeStripe)(nil)
}

// ---------------------------------------------------------------------------
// Helpers reused from planchange_test.go (same package _test, same binary)
// fakeStripe, fakeSubRepo, fixedClock are already declared there.
// ---------------------------------------------------------------------------

// Ensure the unused billing import compiles — fakeStripe references it.
var _ billingstripe.UpdateSubscriptionParams
