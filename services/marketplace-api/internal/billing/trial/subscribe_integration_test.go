//go:build integration

package trial_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	billingstripe "github.com/mark8ly/marketplace-api/internal/billing/stripe"
	"github.com/mark8ly/marketplace-api/internal/billing/trial"
	"github.com/mark8ly/marketplace-api/internal/subscription"
)

// fakeStripe is a test double for trial.StripeAPI that records calls and
// returns deterministic responses without hitting the Stripe API.
type fakeStripe struct {
	priceIDResult   string
	priceIDErr      error
	createSubResult *billingstripe.Subscription
	createSubErr    error
	createSubCalls  int
	lastTrialEnd    int64
}

func (f *fakeStripe) CreateSubscription(_ context.Context, in billingstripe.CreateSubscriptionInput) (*billingstripe.Subscription, error) {
	f.createSubCalls++
	f.lastTrialEnd = in.TrialEnd
	if f.createSubErr != nil {
		return nil, f.createSubErr
	}
	return f.createSubResult, nil
}

func (f *fakeStripe) PriceIDFor(_ context.Context, _ subscription.SubscriptionPlan, _ subscription.SubscriptionPeriod, _ string, _ subscription.PriceTier) (string, error) {
	return f.priceIDResult, f.priceIDErr
}

// openIntegrationDB connects to the database specified by TEST_DB_DSN.
// Tests that call this are skipped when the env var is absent.
func openIntegrationDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn, ok := os.LookupEnv("TEST_DB_DSN")
	if !ok || dsn == "" {
		t.Skip("TEST_DB_DSN not set; skipping integration test")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	require.NoError(t, err, "open integration DB")
	t.Cleanup(func() { sqlDB, _ := db.DB(); _ = sqlDB.Close() })
	return db
}

// seedSubscription inserts a StoreSubscription row and registers cleanup.
func seedSubscription(t *testing.T, db *gorm.DB, row subscription.StoreSubscription) subscription.StoreSubscription {
	t.Helper()
	if row.ID == uuid.Nil {
		row.ID = uuid.New()
	}
	if row.TenantID == uuid.Nil {
		row.TenantID = uuid.New()
	}
	if row.StoreID == uuid.Nil {
		row.StoreID = uuid.New()
	}
	require.NoError(t, db.Create(&row).Error)
	t.Cleanup(func() {
		db.Unscoped().Delete(&subscription.StoreSubscription{}, "id = ?", row.ID)
	})
	return row
}

func defaultFakeStripe() *fakeStripe {
	return &fakeStripe{
		priceIDResult:   "price_starter_monthly_dev",
		createSubResult: &billingstripe.Subscription{ID: "sub_test_123"},
	}
}

func buildInput(tenantID, storeID uuid.UUID) trial.SubscribeInput {
	return trial.SubscribeInput{
		TenantID: tenantID,
		StoreID:  storeID,
		Plan:     subscription.PlanStarter,
		Period:   subscription.PeriodMonthly,
		Currency: "usd",
	}
}

// TestSubscribe_TrialEndIsSignupPlus90Days verifies that trial_end equals
// row.CreatedAt + 90d (the signup_date proxy).
func TestSubscribe_TrialEndIsSignupPlus90Days(t *testing.T) {
	db := openIntegrationDB(t)
	day0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	row := seedSubscription(t, db, subscription.StoreSubscription{
		StripeCustomerID:   "cus_abc",
		Status:             subscription.StatusSignup,
		Plan:               subscription.PlanTrial,
		SubscriptionPeriod: subscription.PeriodMonthly,
		PriceTier:          subscription.PriceTierDeveloped,
		CreatedAt:          day0,
	})

	fake := defaultFakeStripe()
	s := trial.NewSubscriber(db, fake, nil)

	res, err := s.Subscribe(context.Background(), buildInput(row.TenantID, row.StoreID))
	require.NoError(t, err)

	expected := day0.Add(trial.TrialDays * 24 * time.Hour).Unix()
	assert.Equal(t, expected, res.TrialEndUnix)
	assert.Equal(t, expected, fake.lastTrialEnd, "trial_end sent to Stripe must match computed value")
}

// TestSubscribe_PersistsStripeSubscriptionID verifies the DB row is updated
// with the Stripe subscription ID returned from the API.
func TestSubscribe_PersistsStripeSubscriptionID(t *testing.T) {
	db := openIntegrationDB(t)

	row := seedSubscription(t, db, subscription.StoreSubscription{
		StripeCustomerID:   "cus_abc",
		Status:             subscription.StatusSignup,
		Plan:               subscription.PlanTrial,
		SubscriptionPeriod: subscription.PeriodMonthly,
		PriceTier:          subscription.PriceTierDeveloped,
	})

	fake := &fakeStripe{
		priceIDResult:   "price_x",
		createSubResult: &billingstripe.Subscription{ID: "sub_persisted_abc"},
	}
	s := trial.NewSubscriber(db, fake, nil)

	res, err := s.Subscribe(context.Background(), buildInput(row.TenantID, row.StoreID))
	require.NoError(t, err)
	assert.Equal(t, "sub_persisted_abc", res.StripeSubscriptionID)

	var updated subscription.StoreSubscription
	require.NoError(t, db.Where("id = ?", row.ID).First(&updated).Error)
	require.NotNil(t, updated.StripeSubscriptionID)
	assert.Equal(t, "sub_persisted_abc", *updated.StripeSubscriptionID)
}

// TestSubscribe_NoImmediateStatusMutation verifies Subscribe does NOT flip
// subscription.status — the webhook owns that transition.
func TestSubscribe_NoImmediateStatusMutation(t *testing.T) {
	db := openIntegrationDB(t)

	row := seedSubscription(t, db, subscription.StoreSubscription{
		StripeCustomerID:   "cus_abc",
		Status:             subscription.StatusSignup,
		Plan:               subscription.PlanTrial,
		SubscriptionPeriod: subscription.PeriodMonthly,
		PriceTier:          subscription.PriceTierDeveloped,
	})

	s := trial.NewSubscriber(db, defaultFakeStripe(), nil)
	_, err := s.Subscribe(context.Background(), buildInput(row.TenantID, row.StoreID))
	require.NoError(t, err)

	var updated subscription.StoreSubscription
	require.NoError(t, db.Where("id = ?", row.ID).First(&updated).Error)
	assert.Equal(t, subscription.StatusSignup, updated.Status,
		"Subscribe must not mutate subscription.status — webhook owns that transition")
}

// TestSubscribe_BlockedWhenAlreadyActive verifies ErrSubscriptionAlreadyActive
// for stores whose status is already active.
func TestSubscribe_BlockedWhenAlreadyActive(t *testing.T) {
	db := openIntegrationDB(t)

	row := seedSubscription(t, db, subscription.StoreSubscription{
		StripeCustomerID:   "cus_abc",
		Status:             subscription.StatusActive,
		Plan:               subscription.PlanStarter,
		SubscriptionPeriod: subscription.PeriodMonthly,
		PriceTier:          subscription.PriceTierDeveloped,
	})

	s := trial.NewSubscriber(db, defaultFakeStripe(), nil)
	_, err := s.Subscribe(context.Background(), buildInput(row.TenantID, row.StoreID))
	assert.ErrorIs(t, err, trial.ErrSubscriptionAlreadyActive)
}

// TestSubscribe_MissingStripeCustomer verifies ErrMissingStripeCustomer when
// StripeCustomerID is empty.
func TestSubscribe_MissingStripeCustomer(t *testing.T) {
	db := openIntegrationDB(t)

	row := seedSubscription(t, db, subscription.StoreSubscription{
		StripeCustomerID:   "",
		Status:             subscription.StatusSignup,
		Plan:               subscription.PlanTrial,
		SubscriptionPeriod: subscription.PeriodMonthly,
		PriceTier:          subscription.PriceTierDeveloped,
	})

	s := trial.NewSubscriber(db, defaultFakeStripe(), nil)
	_, err := s.Subscribe(context.Background(), buildInput(row.TenantID, row.StoreID))
	assert.ErrorIs(t, err, trial.ErrMissingStripeCustomer)
}

// TestSubscribe_IdempotentOnReplay verifies that calling Subscribe twice with
// the same inputs (and a fake that returns the same sub ID on both calls)
// produces the same result. This mirrors Stripe's idempotency-key behaviour.
func TestSubscribe_IdempotentOnReplay(t *testing.T) {
	db := openIntegrationDB(t)

	row := seedSubscription(t, db, subscription.StoreSubscription{
		StripeCustomerID:   "cus_abc",
		Status:             subscription.StatusSignup,
		Plan:               subscription.PlanTrial,
		SubscriptionPeriod: subscription.PeriodMonthly,
		PriceTier:          subscription.PriceTierDeveloped,
	})

	fake := &fakeStripe{
		priceIDResult:   "price_x",
		createSubResult: &billingstripe.Subscription{ID: "sub_idempotent_xyz"},
	}
	s := trial.NewSubscriber(db, fake, nil)
	in := buildInput(row.TenantID, row.StoreID)

	res1, err := s.Subscribe(context.Background(), in)
	require.NoError(t, err)

	res2, err := s.Subscribe(context.Background(), in)
	require.NoError(t, err)

	assert.Equal(t, res1.StripeSubscriptionID, res2.StripeSubscriptionID,
		"replayed call must return the same subscription ID")
	assert.Equal(t, res1.TrialEndUnix, res2.TrialEndUnix)
}
