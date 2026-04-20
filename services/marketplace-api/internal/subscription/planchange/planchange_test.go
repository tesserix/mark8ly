package planchange_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	billingstripe "github.com/mark8ly/marketplace-api/internal/billing/stripe"
	"github.com/mark8ly/marketplace-api/internal/subscription"
	"github.com/mark8ly/marketplace-api/internal/subscription/planchange"
)

// fakeStripe is a controllable StripeClient for unit tests.
type fakeStripe struct {
	priceIDErr error
	priceID    string
	updateErr  error
	updateSub  *billingstripe.Subscription
}

func (f *fakeStripe) UpdateSubscription(_ context.Context, _ billingstripe.UpdateSubscriptionParams) (*billingstripe.Subscription, error) {
	return f.updateSub, f.updateErr
}

func (f *fakeStripe) CreateSubscription(_ context.Context, _ billingstripe.CreateSubscriptionInput) (*billingstripe.Subscription, error) {
	return f.updateSub, f.updateErr
}

func (f *fakeStripe) PriceIDFor(_ context.Context, _ subscription.SubscriptionPlan, _ subscription.SubscriptionPeriod, _ string, _ subscription.PriceTier) (string, error) {
	return f.priceID, f.priceIDErr
}

// fakeSubRepo implements subscription.Repository with correct *gorm.DB signatures.
type fakeSubRepo struct {
	sub    *subscription.StoreSubscription
	getErr error
}

func (r *fakeSubRepo) GetByStoreID(_ context.Context, _ *gorm.DB, _, _ uuid.UUID) (*subscription.StoreSubscription, error) {
	return r.sub, r.getErr
}
func (r *fakeSubRepo) Create(_ context.Context, _ *gorm.DB, _ *subscription.StoreSubscription) error {
	return nil
}
func (r *fakeSubRepo) Update(_ context.Context, _ *gorm.DB, _ *subscription.StoreSubscription) error {
	return nil
}
func (r *fakeSubRepo) GetByStripeCustomerID(_ context.Context, _ *gorm.DB, _ string) (*subscription.StoreSubscription, error) {
	return nil, errors.New("not implemented")
}
func (r *fakeSubRepo) SetPendingDowngrade(_ context.Context, _ *gorm.DB, _, _ uuid.UUID, _ subscription.SubscriptionPlan, _ subscription.SubscriptionPeriod, _ time.Time, _ string) error {
	return nil
}
func (r *fakeSubRepo) ClearPendingDowngrade(_ context.Context, _ *gorm.DB, _, _ uuid.UUID) error {
	return nil
}
func (r *fakeSubRepo) CommitDowngrade(_ context.Context, _ *gorm.DB, _, _ uuid.UUID, _ subscription.SubscriptionPlan, _ subscription.SubscriptionPeriod) error {
	return nil
}
func (r *fakeSubRepo) CommitUpgrade(_ context.Context, _ *gorm.DB, _, _ uuid.UUID, _ subscription.SubscriptionPlan, _ subscription.SubscriptionPeriod, _ string) error {
	return nil
}
func (r *fakeSubRepo) FindPendingDowngradesReady(_ context.Context, _ *gorm.DB, _ time.Time) ([]subscription.StoreSubscription, error) {
	return nil, nil
}
func (r *fakeSubRepo) CountStoresForPlanSlot(_ context.Context, _ *gorm.DB, _ uuid.UUID) (int, error) {
	return 0, nil
}

// fixedClock returns a constant time for deterministic tests.
type fixedClock struct{ t time.Time }

func (c fixedClock) Now() time.Time { return c.t }

func TestExecute_InvalidTargetPlan(t *testing.T) {
	o := planchange.NewOrchestrator(planchange.Deps{
		// DB is nil — ErrInvalidTargetPlan is returned before any DB access.
		Stripe:           &fakeStripe{},
		SubscriptionRepo: &fakeSubRepo{},
		Clock:            fixedClock{t: time.Now()},
	})

	_, err := o.Execute(context.Background(), planchange.Input{
		TenantID:     uuid.New(),
		StoreID:      uuid.New(),
		TargetPlan:   subscription.PlanMarketplace, // not merchant-selectable
		TargetPeriod: subscription.PeriodMonthly,
		Actor:        "user:test",
		GinCtx:       &gin.Context{},
	})

	require.ErrorIs(t, err, planchange.ErrInvalidTargetPlan)
}

func TestExecute_TrialPlanIsInvalid(t *testing.T) {
	o := planchange.NewOrchestrator(planchange.Deps{
		Stripe:           &fakeStripe{},
		SubscriptionRepo: &fakeSubRepo{},
		Clock:            fixedClock{t: time.Now()},
	})

	_, err := o.Execute(context.Background(), planchange.Input{
		TenantID:     uuid.New(),
		StoreID:      uuid.New(),
		TargetPlan:   subscription.PlanTrial, // not merchant-selectable (checkout flow only)
		TargetPeriod: subscription.PeriodMonthly,
		Actor:        "user:test",
	})

	require.ErrorIs(t, err, planchange.ErrInvalidTargetPlan)
}
