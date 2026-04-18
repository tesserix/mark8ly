//go:build integration

package trial_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/billing/trial"
	"github.com/mark8ly/marketplace-api/internal/subscription"
)

// newActivationCounter creates an unregistered prometheus counter safe for test use.
func newActivationCounter() prometheus.Counter {
	return prometheus.NewCounter(prometheus.CounterOpts{Name: "test_activation_counter"})
}

// activationCounterValue reads the current float64 value of a prometheus.Counter.
func activationCounterValue(c prometheus.Counter) float64 {
	var m dto.Metric
	_ = c.Write(&m)
	return m.GetCounter().GetValue()
}

// TestActivationCron_IncrementsForStoreWithProductAtDay30 seeds a trialing store
// at exactly day 30 with one product and asserts the counter increments and the
// row is stamped.
func TestActivationCron_IncrementsForStoreWithProductAtDay30(t *testing.T) {
	db := openIntegrationDB(t)
	now := time.Date(2026, 9, 1, 0, 30, 0, 0, time.UTC)
	day30 := now.AddDate(0, 0, -30)

	row := seedSubscription(t, db, subscription.StoreSubscription{
		StripeCustomerID:   "cus_act_day30",
		Status:             subscription.StatusTrialing,
		Plan:               subscription.PlanTrial,
		SubscriptionPeriod: subscription.PeriodMonthly,
		PriceTier:          subscription.PriceTierDeveloped,
		CreatedAt:          day30,
	})

	productID := uuid.New()
	require.NoError(t, db.Exec(`
		INSERT INTO products (id, tenant_id, store_id, handle, title, status, tags, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, '{}', now(), now())`,
		productID, row.TenantID, row.StoreID, "act-product-day30", "Test Product", "active",
	).Error)
	t.Cleanup(func() { db.Exec("DELETE FROM products WHERE id = ?", productID) })

	counter := newActivationCounter()
	cron := trial.NewActivationCron(db, counter, nil, fixedClock(now))
	require.NoError(t, cron.Run(context.Background()))

	assert.Equal(t, float64(1), activationCounterValue(counter), "counter must increment once")

	var markedAt *time.Time
	require.NoError(t, db.Raw(
		"SELECT trial_activation_marked_at FROM store_subscriptions WHERE id = ?", row.ID,
	).Scan(&markedAt).Error)
	assert.NotNil(t, markedAt, "trial_activation_marked_at must be stamped")
}

// TestActivationCron_SkipsStoreWithoutProducts seeds a day-30 trialing store with
// no products and asserts the counter stays 0 and no mark is set.
func TestActivationCron_SkipsStoreWithoutProducts(t *testing.T) {
	db := openIntegrationDB(t)
	now := time.Date(2026, 9, 2, 0, 30, 0, 0, time.UTC)
	day30 := now.AddDate(0, 0, -30)

	row := seedSubscription(t, db, subscription.StoreSubscription{
		StripeCustomerID:   "cus_act_noproduct",
		Status:             subscription.StatusTrialing,
		Plan:               subscription.PlanTrial,
		SubscriptionPeriod: subscription.PeriodMonthly,
		PriceTier:          subscription.PriceTierDeveloped,
		CreatedAt:          day30,
	})

	counter := newActivationCounter()
	cron := trial.NewActivationCron(db, counter, nil, fixedClock(now))
	require.NoError(t, cron.Run(context.Background()))

	assert.Equal(t, float64(0), activationCounterValue(counter), "counter must not increment for store with no products")

	var markedAt *time.Time
	require.NoError(t, db.Raw(
		"SELECT trial_activation_marked_at FROM store_subscriptions WHERE id = ?", row.ID,
	).Scan(&markedAt).Error)
	assert.Nil(t, markedAt, "trial_activation_marked_at must not be set for store with no products")
}

// TestActivationCron_SkipsAlreadyMarkedStore seeds a day-30 trialing store that
// already has trial_activation_marked_at stamped and asserts the counter stays 0.
func TestActivationCron_SkipsAlreadyMarkedStore(t *testing.T) {
	db := openIntegrationDB(t)
	now := time.Date(2026, 9, 3, 0, 30, 0, 0, time.UTC)
	day30 := now.AddDate(0, 0, -30)
	alreadyMarked := now.Add(-time.Hour)

	row := seedSubscription(t, db, subscription.StoreSubscription{
		StripeCustomerID:        "cus_act_marked",
		Status:                  subscription.StatusTrialing,
		Plan:                    subscription.PlanTrial,
		SubscriptionPeriod:      subscription.PeriodMonthly,
		PriceTier:               subscription.PriceTierDeveloped,
		CreatedAt:               day30,
		TrialActivationMarkedAt: &alreadyMarked,
	})

	productID := uuid.New()
	require.NoError(t, db.Exec(`
		INSERT INTO products (id, tenant_id, store_id, handle, title, status, tags, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, '{}', now(), now())`,
		productID, row.TenantID, row.StoreID, "act-product-marked", "Test Product", "active",
	).Error)
	t.Cleanup(func() { db.Exec("DELETE FROM products WHERE id = ?", productID) })

	counter := newActivationCounter()
	cron := trial.NewActivationCron(db, counter, nil, fixedClock(now))
	require.NoError(t, cron.Run(context.Background()))

	assert.Equal(t, float64(0), activationCounterValue(counter), "counter must not increment for already-marked store")
}

// TestActivationCron_SkipsNonTrialingStore seeds a day-30 store in "active"
// status with a product and asserts the counter stays 0.
func TestActivationCron_SkipsNonTrialingStore(t *testing.T) {
	db := openIntegrationDB(t)
	now := time.Date(2026, 9, 4, 0, 30, 0, 0, time.UTC)
	day30 := now.AddDate(0, 0, -30)

	row := seedSubscription(t, db, subscription.StoreSubscription{
		StripeCustomerID:   "cus_act_active",
		Status:             subscription.StatusActive,
		Plan:               subscription.PlanStarter,
		SubscriptionPeriod: subscription.PeriodMonthly,
		PriceTier:          subscription.PriceTierDeveloped,
		CreatedAt:          day30,
	})

	productID := uuid.New()
	require.NoError(t, db.Exec(`
		INSERT INTO products (id, tenant_id, store_id, handle, title, status, tags, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, '{}', now(), now())`,
		productID, row.TenantID, row.StoreID, "act-product-active", "Test Product", "active",
	).Error)
	t.Cleanup(func() { db.Exec("DELETE FROM products WHERE id = ?", productID) })

	counter := newActivationCounter()
	cron := trial.NewActivationCron(db, counter, nil, fixedClock(now))
	require.NoError(t, cron.Run(context.Background()))

	assert.Equal(t, float64(0), activationCounterValue(counter), "counter must not increment for non-trialing store")
}

// TestActivationCron_Idempotent_SecondRunNoop runs the cron twice for the same
// qualifying store and asserts the counter increments exactly once total.
func TestActivationCron_Idempotent_SecondRunNoop(t *testing.T) {
	db := openIntegrationDB(t)
	now := time.Date(2026, 9, 5, 0, 30, 0, 0, time.UTC)
	day30 := now.AddDate(0, 0, -30)

	row := seedSubscription(t, db, subscription.StoreSubscription{
		StripeCustomerID:   "cus_act_idem",
		Status:             subscription.StatusTrialing,
		Plan:               subscription.PlanTrial,
		SubscriptionPeriod: subscription.PeriodMonthly,
		PriceTier:          subscription.PriceTierDeveloped,
		CreatedAt:          day30,
	})

	productID := uuid.New()
	require.NoError(t, db.Exec(`
		INSERT INTO products (id, tenant_id, store_id, handle, title, status, tags, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, '{}', now(), now())`,
		productID, row.TenantID, row.StoreID, "act-product-idem", "Test Product", "active",
	).Error)
	t.Cleanup(func() { db.Exec("DELETE FROM products WHERE id = ?", productID) })

	counter := newActivationCounter()
	cron := trial.NewActivationCron(db, counter, nil, fixedClock(now))

	require.NoError(t, cron.Run(context.Background()), "first run must succeed")
	require.NoError(t, cron.Run(context.Background()), "second run must succeed")

	assert.Equal(t, float64(1), activationCounterValue(counter),
		"counter must increment exactly once across two runs")
}
