//go:build integration

package planchange_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	billingstripe "github.com/mark8ly/marketplace-api/internal/billing/stripe"
	"github.com/mark8ly/marketplace-api/internal/subscription"
	"github.com/mark8ly/marketplace-api/internal/subscription/planchange"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

// countingStripe records how many times the irreversible Stripe call was
// made, so a test can assert it was NOT made.
type countingStripe struct {
	updateCalls int
	updateErr   error
}

func (f *countingStripe) UpdateSubscription(_ context.Context, in billingstripe.UpdateSubscriptionParams) (*billingstripe.Subscription, error) {
	f.updateCalls++
	if f.updateErr != nil {
		return nil, f.updateErr
	}
	return &billingstripe.Subscription{ID: in.SubscriptionID, Status: "active"}, nil
}

func (f *countingStripe) CreateSubscription(_ context.Context, _ billingstripe.CreateSubscriptionInput) (*billingstripe.Subscription, error) {
	return nil, errors.New("not expected in these tests")
}

func (f *countingStripe) PriceIDFor(_ context.Context, _ subscription.SubscriptionPlan, _ subscription.SubscriptionPeriod, _ string, _ subscription.PriceTier) (string, error) {
	return "price_studio_monthly", nil
}

// erroringBudget stands in for campaignbudget.Service failing — one of the
// three fallible steps that used to sit AFTER the Stripe call.
type erroringBudget struct{ err error }

func (b erroringBudget) RecomputeLimitForPlan(_ context.Context, _ *gorm.DB, _ uuid.UUID, _ string) error {
	return b.err
}

// seedUpgradeableSubscription inserts an active starter/monthly subscription
// with a Stripe subscription id, ready to be upgraded to studio.
func seedUpgradeableSubscription(t *testing.T, db *gorm.DB) (uuid.UUID, uuid.UUID) {
	t.Helper()
	tenantID := uuid.New()
	storeID := uuid.New()
	testdb.SeedStore(t, db, tenantID, storeID)

	subID := "sub_stripe_window"
	require.NoError(t, db.Create(&subscription.StoreSubscription{
		TenantID:             tenantID,
		StoreID:              storeID,
		StripeCustomerID:     "cus_stripe_window",
		StripeSubscriptionID: &subID,
		Plan:                 subscription.PlanStarter,
		Status:               subscription.StatusActive,
		SubscriptionPeriod:   subscription.PeriodMonthly,
		PriceTier:            subscription.PriceTierDeveloped,
		BillingCurrency:      strPtr("USD"),
	}).Error)
	return tenantID, storeID
}

func upgradeInput(tenantID, storeID uuid.UUID) planchange.Input {
	return planchange.Input{
		TenantID:          tenantID,
		StoreID:           storeID,
		TargetPlan:        subscription.PlanStudio,
		TargetPeriod:      subscription.PeriodMonthly,
		RequestedCurrency: "USD",
		Actor:             "user:" + uuid.NewString(),
		Reason:            "stripe_window_test",
	}
}

func countAuditRows(t *testing.T, db *gorm.DB, storeID uuid.UUID) int64 {
	t.Helper()
	var n int64
	require.NoError(t, db.Table("subscription_plan_change_audit").
		Where("store_id = ?", storeID).Count(&n).Error)
	return n
}

func currentPlan(t *testing.T, db *gorm.DB, storeID uuid.UUID) string {
	t.Helper()
	var plan string
	require.NoError(t, db.Raw(
		`SELECT plan FROM store_subscriptions WHERE store_id = ?`, storeID,
	).Row().Scan(&plan))
	return plan
}

// TestUpgrade_BudgetRecomputeFailure_NeverReachesStripe is the #425 guard: a
// failing budget recompute used to run AFTER Stripe had already re-priced the
// subscription and issued a proration invoice, rolling the local transaction
// back and leaving the two systems disagreeing. It now runs before, so the
// same failure costs the merchant nothing.
func TestUpgrade_BudgetRecomputeFailure_NeverReachesStripe(t *testing.T) {
	db := testdb.NewDB(t, "subscription_plan_change_audit", "store_subscriptions")
	tenantID, storeID := seedUpgradeableSubscription(t, db)

	stripe := &countingStripe{}
	o := planchange.NewOrchestrator(planchange.Deps{
		DB:               db,
		Stripe:           stripe,
		SubscriptionRepo: subscription.NewRepository(),
		Clock:            fixedClock{t: time.Now()},
		BudgetRecomputer: erroringBudget{err: errors.New("budget row locked")},
	})

	_, err := o.Execute(context.Background(), upgradeInput(tenantID, storeID))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "recompute budget limit")

	assert.Equal(t, 0, stripe.updateCalls,
		"a failure that can be discovered locally must never bill the merchant")
	assert.Equal(t, int64(0), countAuditRows(t, db, storeID))
	assert.Equal(t, string(subscription.PlanStarter), currentPlan(t, db, storeID))
}

// TestUpgrade_StripeFailure_LeavesNoAuditRow pins the behaviour that makes the
// reordering safe: the audit row is now written BEFORE the Stripe call, but it
// is written through the same transaction, so a Stripe failure still erases
// it. No audit row is ever left recording an upgrade that did not happen.
func TestUpgrade_StripeFailure_LeavesNoAuditRow(t *testing.T) {
	db := testdb.NewDB(t, "subscription_plan_change_audit", "store_subscriptions")
	tenantID, storeID := seedUpgradeableSubscription(t, db)

	stripe := &countingStripe{updateErr: errors.New("stripe: card_declined")}
	o := planchange.NewOrchestrator(planchange.Deps{
		DB:               db,
		Stripe:           stripe,
		SubscriptionRepo: subscription.NewRepository(),
		Clock:            fixedClock{t: time.Now()},
	})

	_, err := o.Execute(context.Background(), upgradeInput(tenantID, storeID))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "stripe update subscription")

	assert.Equal(t, 1, stripe.updateCalls)
	assert.Equal(t, int64(0), countAuditRows(t, db, storeID),
		"an audit row for an upgrade Stripe refused must not persist")
	assert.Equal(t, string(subscription.PlanStarter), currentPlan(t, db, storeID))
}

// TestUpgrade_Success_WritesAuditRowWithStripeSubscriptionID confirms the
// audit row still carries the Stripe subscription id after the reorder — it
// is now read from the local row rather than from the Stripe response.
func TestUpgrade_Success_WritesAuditRowWithStripeSubscriptionID(t *testing.T) {
	db := testdb.NewDB(t, "subscription_plan_change_audit", "store_subscriptions")
	tenantID, storeID := seedUpgradeableSubscription(t, db)

	o := planchange.NewOrchestrator(planchange.Deps{
		DB:               db,
		Stripe:           &countingStripe{},
		SubscriptionRepo: subscription.NewRepository(),
		Clock:            fixedClock{t: time.Now()},
	})

	out, err := o.Execute(context.Background(), upgradeInput(tenantID, storeID))
	require.NoError(t, err)
	assert.Equal(t, planchange.ResultUpgradeCommitted, out.Result)

	var gotSubID, gotAction string
	require.NoError(t, db.Raw(
		`SELECT stripe_subscription_id, action FROM subscription_plan_change_audit WHERE store_id = ?`,
		storeID,
	).Row().Scan(&gotSubID, &gotAction))
	assert.Equal(t, "sub_stripe_window", gotSubID)
	assert.Equal(t, "upgrade_committed", gotAction)
	assert.Equal(t, string(subscription.PlanStudio), currentPlan(t, db, storeID))
}
