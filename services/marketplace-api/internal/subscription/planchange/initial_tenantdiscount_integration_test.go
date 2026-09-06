//go:build integration

package planchange_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	billingstripe "github.com/mark8ly/marketplace-api/internal/billing/stripe"
	"github.com/mark8ly/marketplace-api/internal/billing/tenantdiscount"
	"github.com/mark8ly/marketplace-api/internal/subscription"
	"github.com/mark8ly/marketplace-api/internal/subscription/planchange"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

// mark8ly#660 T6. executeInitialSubscription is the SECOND place a Stripe
// subscription is created for a store that had none — the lazily bootstrapped
// row, stripe_customer_id set and stripe_subscription_id still NULL, which is
// precisely the store the fan-out reported `pending`.

// stubDiscounter stands in for tenantdiscount.Service at the port planchange
// declares.
type stubDiscounter struct {
	calls   []tenantdiscount.NewSubscriptionInput
	outcome tenantdiscount.Outcome
	err     error
}

func (s *stubDiscounter) ApplyToNewSubscription(_ context.Context, in tenantdiscount.NewSubscriptionInput) (tenantdiscount.Outcome, error) {
	s.calls = append(s.calls, in)
	return s.outcome, s.err
}

// newInitialSubscriptionOrchestrator seeds the store and the
// store_subscriptions row executeInitialSubscription is written for — a
// customer but no Stripe subscription yet — and returns an orchestrator over
// them.
func newInitialSubscriptionOrchestrator(t *testing.T, disc planchange.TenantDiscountApplier) (*planchange.Orchestrator, uuid.UUID, uuid.UUID) {
	t.Helper()

	db := testdb.NewDB(t, "subscription_plan_change_audit", "store_subscriptions", "stores")
	tenantID, storeID := uuid.New(), uuid.New()
	testdb.SeedStore(t, db, tenantID, storeID)

	require.NoError(t, db.Create(&subscription.StoreSubscription{
		TenantID:           tenantID,
		StoreID:            storeID,
		StripeCustomerID:   "cus_test",
		Plan:               subscription.PlanTrial,
		Status:             subscription.StatusSignup,
		SubscriptionPeriod: subscription.PeriodMonthly,
		PriceTier:          subscription.PriceTierDeveloped,
		BillingCurrency:    strPtr("USD"),
	}).Error)

	fs := &fakeStripe{
		priceID:   "price_test_starter_monthly",
		updateSub: &billingstripe.Subscription{ID: "sub_initial_test", Status: "trialing"},
	}
	o := planchange.NewOrchestrator(planchange.Deps{
		DB:               db,
		Stripe:           fs,
		SubscriptionRepo: subscription.NewRepository(),
		Clock:            fixedClock{t: time.Now()},
		TenantDiscount:   disc,
	})
	return o, tenantID, storeID
}

func initialSubscriptionInput(tenantID, storeID uuid.UUID) planchange.Input {
	return planchange.Input{
		TenantID:          tenantID,
		StoreID:           storeID,
		TargetPlan:        subscription.PlanStarter,
		TargetPeriod:      subscription.PeriodMonthly,
		RequestedCurrency: "USD",
		Actor:             "user:" + uuid.NewString(),
		Reason:            "integration_test",
	}
}

func TestExecute_InitialSubscriptionOffersItToTheTenantsStandingOverride(t *testing.T) {
	disc := &stubDiscounter{outcome: tenantdiscount.OutcomeApplied}
	o, tenantID, storeID := newInitialSubscriptionOrchestrator(t, disc)

	out, err := o.Execute(context.Background(), initialSubscriptionInput(tenantID, storeID))
	require.NoError(t, err)
	require.Equal(t, planchange.ResultUpgradeCommitted, out.Result)

	require.Len(t, disc.calls, 1)
	require.Equal(t, tenantdiscount.NewSubscriptionInput{
		TenantID: tenantID, StoreID: storeID, StripeSubscriptionID: "sub_initial_test",
	}, disc.calls[0])
}

// THE LOAD-BEARING ONE, mirroring trial's. The discount must never be able to
// fail a plan change.
func TestExecute_ADiscountFailureDoesNotBlockTheInitialSubscription(t *testing.T) {
	disc := &stubDiscounter{err: errors.New("stripe is down")}
	o, tenantID, storeID := newInitialSubscriptionOrchestrator(t, disc)

	out, err := o.Execute(context.Background(), initialSubscriptionInput(tenantID, storeID))
	require.NoError(t, err, "a discount failure must never fail the plan change")
	require.Equal(t, planchange.ResultUpgradeCommitted, out.Result)
}

// Without a discounter wired — the no-Stripe-billing configuration — Execute
// behaves exactly as it did before T6.
func TestExecute_InitialSubscriptionWithNoDiscounterIsUnchanged(t *testing.T) {
	o, tenantID, storeID := newInitialSubscriptionOrchestrator(t, nil)

	out, err := o.Execute(context.Background(), initialSubscriptionInput(tenantID, storeID))
	require.NoError(t, err)
	require.Equal(t, planchange.ResultUpgradeCommitted, out.Result)
}
