package planchange_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	billingstripe "github.com/mark8ly/marketplace-api/internal/billing/stripe"
	"github.com/mark8ly/marketplace-api/internal/subscription"
	"github.com/mark8ly/marketplace-api/internal/subscription/planchange"
)

// ---------------------------------------------------------------------------
// Additional fakes needed by cron tests
// ---------------------------------------------------------------------------

// cronFakeSubRepo extends fakeSubRepo with controllable FindPendingDowngradesReady
// and CommitDowngrade/ClearPendingDowngrade call recording.
type cronFakeSubRepo struct {
	// GetByStoreID return value — the "freshly re-read" subscription.
	sub    *subscription.StoreSubscription
	getErr error

	// FindPendingDowngradesReady return values.
	pendingRows    []subscription.StoreSubscription
	pendingRowsErr error

	// Call records.
	commitDowngradeCalled   bool
	commitDowngradePlan     subscription.SubscriptionPlan
	commitDowngradePeriod   subscription.SubscriptionPeriod
	clearPendingCalled      bool
}

func (r *cronFakeSubRepo) GetByStoreID(_ context.Context, _ *gorm.DB, _, _ uuid.UUID) (*subscription.StoreSubscription, error) {
	return r.sub, r.getErr
}
func (r *cronFakeSubRepo) Create(_ context.Context, _ *gorm.DB, _ *subscription.StoreSubscription) error {
	return nil
}
func (r *cronFakeSubRepo) Update(_ context.Context, _ *gorm.DB, _ *subscription.StoreSubscription) error {
	return nil
}
func (r *cronFakeSubRepo) GetByStripeCustomerID(_ context.Context, _ *gorm.DB, _ string) (*subscription.StoreSubscription, error) {
	return nil, nil
}
func (r *cronFakeSubRepo) SetPendingDowngrade(_ context.Context, _ *gorm.DB, _, _ uuid.UUID, _ subscription.SubscriptionPlan, _ subscription.SubscriptionPeriod, _ time.Time, _ string) error {
	return nil
}
func (r *cronFakeSubRepo) ClearPendingDowngrade(_ context.Context, _ *gorm.DB, _, _ uuid.UUID) error {
	r.clearPendingCalled = true
	return nil
}
func (r *cronFakeSubRepo) CommitDowngrade(_ context.Context, _ *gorm.DB, _, _ uuid.UUID, plan subscription.SubscriptionPlan, period subscription.SubscriptionPeriod) error {
	r.commitDowngradeCalled = true
	r.commitDowngradePlan = plan
	r.commitDowngradePeriod = period
	return nil
}
func (r *cronFakeSubRepo) CommitUpgrade(_ context.Context, _ *gorm.DB, _, _ uuid.UUID, _ subscription.SubscriptionPlan, _ subscription.SubscriptionPeriod, _ string) error {
	return nil
}
func (r *cronFakeSubRepo) FindPendingDowngradesReady(_ context.Context, _ *gorm.DB, _ time.Time) ([]subscription.StoreSubscription, error) {
	return r.pendingRows, r.pendingRowsErr
}
func (r *cronFakeSubRepo) CountStoresForPlanSlot(_ context.Context, _ *gorm.DB, _ uuid.UUID) (int, error) {
	return 0, nil
}

// recordingStripe wraps fakeStripe and records UpdateSubscription call params.
type recordingStripe struct {
	fakeStripe
	updateCalled bool
	lastParams   billingstripe.UpdateSubscriptionParams
}

func (r *recordingStripe) UpdateSubscription(_ context.Context, p billingstripe.UpdateSubscriptionParams) (*billingstripe.Subscription, error) {
	r.updateCalled = true
	r.lastParams = p
	return r.updateSub, r.updateErr
}

// fakeNotifier records NotifyDowngradeBlocked invocations.
type fakeNotifier struct {
	called       int
	lastReason   string
	lastDetails  map[string]any
}

func (n *fakeNotifier) NotifyDowngradeBlocked(_ context.Context, _, _ uuid.UUID, reason string, details map[string]any) error {
	n.called++
	n.lastReason = reason
	n.lastDetails = details
	return nil
}

// testClock is a simple settable Clock for cron tests.
type testClock struct{ NowValue time.Time }

func (tc testClock) Now() time.Time { return tc.NowValue }

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// pendingDowngradeSub builds a StoreSubscription with a pending downgrade
// whose effective_at is at or before `effectiveAt`.
func pendingDowngradeSub(
	from subscription.SubscriptionPlan,
	to subscription.SubscriptionPlan,
	period subscription.SubscriptionPeriod,
	effectiveAt time.Time,
) *subscription.StoreSubscription {
	currency := "USD"
	subID := "sub_test_123"
	return &subscription.StoreSubscription{
		ID:                          uuid.New(),
		TenantID:                    uuid.New(),
		StoreID:                     uuid.New(),
		Plan:                        from,
		Status:                      subscription.StatusActive,
		SubscriptionPeriod:          period,
		BillingCurrency:             &currency,
		PriceTier:                   subscription.PriceTierDeveloped,
		StripeSubscriptionID:        &subID,
		PendingDowngradePlan:        &to,
		PendingDowngradePeriod:      &period,
		PendingDowngradeEffectiveAt: &effectiveAt,
	}
}

// newCronWithDeps builds a DowngradeRecheckCron wired to the supplied fakes.
// DB is nil — processOneInLock is called directly in unit tests.
func newCronWithDeps(
	subRepo subscription.Repository,
	storeRepo *fakeStoreRepo,
	stripe planchange.StripeClient,
	notifier planchange.DowngradeBlockNotifier,
	clk planchange.Clock,
) *planchange.DowngradeRecheckCron {
	return planchange.NewDowngradeRecheckCron(planchange.CronDeps{
		DB:               nil,
		SubscriptionRepo: subRepo,
		StoreRepo:        storeRepo,
		Stripe:           stripe,
		Notifier:         notifier,
		Clock:            clk,
	})
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

// TestDowngradeRecheckCron_NoReadyRows_NoOp asserts that RunOnce is a no-op
// when FindPendingDowngradesReady returns an empty slice.
func TestDowngradeRecheckCron_NoReadyRows_NoOp(t *testing.T) {
	subRepo := &cronFakeSubRepo{pendingRows: nil}
	stripe := &recordingStripe{}
	notifier := &fakeNotifier{}
	clk := testClock{NowValue: time.Now()}

	cron := newCronWithDeps(subRepo, &fakeStoreRepo{count: 0}, stripe, notifier, clk)

	err := cron.RunOnce(context.Background())

	require.NoError(t, err)
	assert.False(t, stripe.updateCalled, "Stripe.UpdateSubscription must not be called when no rows are ready")
	assert.False(t, subRepo.commitDowngradeCalled)
	assert.Equal(t, 0, notifier.called)
}

// TestDowngradeRecheckCron_CommitsWhenEligible_UpdatesStripeAndPersists asserts
// the happy-path: one ready row with store count under cap → Stripe updated,
// CommitDowngrade called, idempotency key has the expected prefix.
func TestDowngradeRecheckCron_CommitsWhenEligible_UpdatesStripeAndPersists(t *testing.T) {
	now := time.Date(2026, 4, 19, 12, 0, 0, 0, time.UTC)
	effectiveAt := now.Add(-time.Minute) // already past

	sub := pendingDowngradeSub(
		subscription.PlanStudio,
		subscription.PlanStarter,
		subscription.PeriodMonthly,
		effectiveAt,
	)

	subRepo := &cronFakeSubRepo{
		sub:         sub,
		pendingRows: []subscription.StoreSubscription{*sub},
	}
	stripe := &recordingStripe{fakeStripe: fakeStripe{priceID: "price_starter_monthly"}}
	notifier := &fakeNotifier{}
	storeRepo := &fakeStoreRepo{count: 1} // under Starter cap of 2
	clk := testClock{NowValue: now}

	cron := newCronWithDeps(subRepo, storeRepo, stripe, notifier, clk)

	// Call processOneInLock directly (nil tx — fakes ignore the db arg).
	err := cron.ProcessOneInLock(context.Background(), nil, sub)

	require.NoError(t, err)

	// Stripe must have been called with ProrationNone and the cron idempotency-key prefix.
	require.True(t, stripe.updateCalled, "Stripe.UpdateSubscription must be called")
	assert.Equal(t, billingstripe.ProrationNone, stripe.lastParams.ProrationBehavior)
	assert.Contains(t, stripe.lastParams.IdempotencyKey, "plan-change-cron:")

	// CommitDowngrade must be called with the target plan and period.
	assert.True(t, subRepo.commitDowngradeCalled)
	assert.Equal(t, subscription.PlanStarter, subRepo.commitDowngradePlan)
	assert.Equal(t, subscription.PeriodMonthly, subRepo.commitDowngradePeriod)

	// Notifier must NOT be called on the happy path.
	assert.Equal(t, 0, notifier.called)
}

// TestDowngradeRecheckCron_BlocksWhenOverQuota_StaysActive_FiresNotifier asserts
// that when the re-check store count exceeds the target plan cap, the cron:
//   - does NOT call Stripe.UpdateSubscription
//   - calls ClearPendingDowngrade
//   - fires the notifier once with reason="store_count_over_quota"
func TestDowngradeRecheckCron_BlocksWhenOverQuota_StaysActive_FiresNotifier(t *testing.T) {
	now := time.Date(2026, 4, 19, 12, 0, 0, 0, time.UTC)
	effectiveAt := now.Add(-time.Minute)

	sub := pendingDowngradeSub(
		subscription.PlanStudio,
		subscription.PlanStarter, // Starter cap = 2
		subscription.PeriodMonthly,
		effectiveAt,
	)

	subRepo := &cronFakeSubRepo{
		sub:         sub,
		pendingRows: []subscription.StoreSubscription{*sub},
	}
	stripe := &recordingStripe{fakeStripe: fakeStripe{priceID: "price_starter_monthly"}}
	notifier := &fakeNotifier{}
	storeRepo := &fakeStoreRepo{count: 5} // exceeds Starter cap of 2
	clk := testClock{NowValue: now}

	cron := newCronWithDeps(subRepo, storeRepo, stripe, notifier, clk)

	err := cron.ProcessOneInLock(context.Background(), nil, sub)

	require.NoError(t, err)

	// Stripe must NOT be called when blocked.
	assert.False(t, stripe.updateCalled, "Stripe.UpdateSubscription must NOT be called when over quota")

	// ClearPendingDowngrade must be called.
	assert.True(t, subRepo.clearPendingCalled, "ClearPendingDowngrade must be called when blocked")

	// CommitDowngrade must NOT be called.
	assert.False(t, subRepo.commitDowngradeCalled)

	// Notifier must be called once with the over-quota reason.
	require.Equal(t, 1, notifier.called)
	assert.Equal(t, "store_count_over_quota", notifier.lastReason)
}

// TestDowngradeRecheckCron_NotReadyYet_Skips asserts that a row whose
// PendingDowngradeEffectiveAt is in the future is skipped gracefully.
func TestDowngradeRecheckCron_NotReadyYet_Skips(t *testing.T) {
	now := time.Date(2026, 4, 19, 12, 0, 0, 0, time.UTC)
	futureEffectiveAt := now.Add(24 * time.Hour) // still in the future

	sub := pendingDowngradeSub(
		subscription.PlanStudio,
		subscription.PlanStarter,
		subscription.PeriodMonthly,
		futureEffectiveAt,
	)

	subRepo := &cronFakeSubRepo{sub: sub}
	stripe := &recordingStripe{}
	notifier := &fakeNotifier{}
	storeRepo := &fakeStoreRepo{count: 1}
	clk := testClock{NowValue: now}

	cron := newCronWithDeps(subRepo, storeRepo, stripe, notifier, clk)

	err := cron.ProcessOneInLock(context.Background(), nil, sub)

	require.NoError(t, err)
	assert.False(t, stripe.updateCalled)
	assert.False(t, subRepo.commitDowngradeCalled)
	assert.False(t, subRepo.clearPendingCalled)
	assert.Equal(t, 0, notifier.called)
}

// TestDowngradeRecheckCron_NilPending_Skips asserts that a row with no
// PendingDowngradePlan (already committed or cleared by another worker) is
// skipped without error.
func TestDowngradeRecheckCron_NilPending_Skips(t *testing.T) {
	now := time.Date(2026, 4, 19, 12, 0, 0, 0, time.UTC)

	// Subscription with no pending downgrade — simulates another worker
	// clearing the row between FindPendingDowngradesReady and processOneInLock.
	currency := "USD"
	sub := &subscription.StoreSubscription{
		ID:                   uuid.New(),
		TenantID:             uuid.New(),
		StoreID:              uuid.New(),
		Plan:                 subscription.PlanStudio,
		Status:               subscription.StatusActive,
		SubscriptionPeriod:   subscription.PeriodMonthly,
		BillingCurrency:      &currency,
		PriceTier:            subscription.PriceTierDeveloped,
		PendingDowngradePlan: nil, // nil — nothing to commit
	}

	subRepo := &cronFakeSubRepo{sub: sub}
	stripe := &recordingStripe{}
	notifier := &fakeNotifier{}
	clk := testClock{NowValue: now}

	cron := newCronWithDeps(subRepo, &fakeStoreRepo{}, stripe, notifier, clk)

	err := cron.ProcessOneInLock(context.Background(), nil, sub)

	require.NoError(t, err)
	assert.False(t, stripe.updateCalled)
	assert.False(t, subRepo.commitDowngradeCalled)
}
