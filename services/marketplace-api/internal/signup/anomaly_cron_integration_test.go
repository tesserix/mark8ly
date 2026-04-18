//go:build integration

package signup_test

import (
	"context"
	"fmt"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/signup"
)

// newTestCounter creates an unregistered prometheus counter safe for test use.
func newTestCounter() prometheus.Counter {
	return prometheus.NewCounter(prometheus.CounterOpts{Name: "test_counter"})
}

// testCounterValue reads the current float64 value of a prometheus.Counter.
func testCounterValue(c prometheus.Counter) float64 {
	var m dto.Metric
	_ = c.Write(&m)
	return m.GetCounter().GetValue()
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

// fakeSlack records calls to Send.
type fakeSlack struct {
	calls atomic.Int32
}

func (f *fakeSlack) Send(_ context.Context, _ string) error {
	f.calls.Add(1)
	return nil
}

// seedSignups inserts n store_subscriptions rows with created_at = ts and
// registers cleanup to delete them. Returns a unique marker stripe_customer_id
// prefix for isolation.
func seedSignups(t *testing.T, db *gorm.DB, n int, ts time.Time, prefix string) {
	t.Helper()
	for i := 0; i < n; i++ {
		err := db.Exec(`
			INSERT INTO store_subscriptions
				(id, tenant_id, store_id, stripe_customer_id, plan, status, subscription_period, price_tier, created_at, updated_at)
			VALUES
				(gen_random_uuid(), gen_random_uuid(), gen_random_uuid(), ?, 'trial', 'trialing', 'monthly', 'developed', ?, now())`,
			fmt.Sprintf("%s_%d", prefix, i), ts,
		).Error
		require.NoError(t, err)
	}
	t.Cleanup(func() {
		db.Exec("DELETE FROM store_subscriptions WHERE stripe_customer_id LIKE ?", prefix+"%")
		db.Exec("DELETE FROM signup_anomaly_log WHERE alert_date >= ?", ts.Format("2006-01-02"))
	})
}

// fixedClock returns a clock func that always returns t.
func fixedClock(t time.Time) func() time.Time { return func() time.Time { return t } }

// TestAnomalyCron_QuietUnder50_NoSlack_NoCounter seeds 49 rows created
// yesterday and asserts: no Slack send, no log row, counter stays at 0.
func TestAnomalyCron_QuietUnder50_NoSlack_NoCounter(t *testing.T) {
	db := openIntegrationDB(t)
	now := time.Date(2026, 8, 1, 5, 0, 0, 0, time.UTC)
	yesterday := now.AddDate(0, 0, -1)

	slack := &fakeSlack{}
	counter := newTestCounter()
	seedSignups(t, db, 49, yesterday, "cus_quiet49")

	cron := signup.NewAnomalyCron(db, slack, counter, nil, fixedClock(now))
	require.NoError(t, cron.Run(context.Background()))

	assert.Equal(t, int32(0), slack.calls.Load(), "slack must not be called under threshold")
	assert.Equal(t, float64(0), testCounterValue(counter), "counter must not increment")

	var logCount int64
	require.NoError(t, db.Raw(
		"SELECT COUNT(*) FROM signup_anomaly_log WHERE signup_date = ?",
		yesterday.Format("2006-01-02"),
	).Scan(&logCount).Error)
	assert.Equal(t, int64(0), logCount, "no anomaly log row must be inserted")
}

// TestAnomalyCron_AlertsOver50 seeds 75 rows yesterday, runs the cron, and
// asserts exactly one Slack send, counter incremented once, and a log row present.
func TestAnomalyCron_AlertsOver50(t *testing.T) {
	db := openIntegrationDB(t)
	now := time.Date(2026, 8, 2, 5, 0, 0, 0, time.UTC)
	yesterday := now.AddDate(0, 0, -1)

	slack := &fakeSlack{}
	counter := newTestCounter()
	seedSignups(t, db, 75, yesterday, "cus_alert75")

	cron := signup.NewAnomalyCron(db, slack, counter, nil, fixedClock(now))
	require.NoError(t, cron.Run(context.Background()))

	assert.Equal(t, int32(1), slack.calls.Load(), "slack must be called exactly once")
	assert.Equal(t, float64(1), testCounterValue(counter), "counter must increment once")

	var logCount int64
	require.NoError(t, db.Raw(
		"SELECT COUNT(*) FROM signup_anomaly_log WHERE signup_date = ?",
		yesterday.Format("2006-01-02"),
	).Scan(&logCount).Error)
	assert.Equal(t, int64(1), logCount, "exactly one anomaly log row must be inserted")
}

// TestAnomalyCron_IdempotentSameDay_SecondRunNoOp seeds 75 rows, runs the cron
// twice with the same clock, and asserts only one Slack send total.
func TestAnomalyCron_IdempotentSameDay_SecondRunNoOp(t *testing.T) {
	db := openIntegrationDB(t)
	now := time.Date(2026, 8, 3, 5, 0, 0, 0, time.UTC)
	yesterday := now.AddDate(0, 0, -1)

	slack := &fakeSlack{}
	counter := newTestCounter()
	seedSignups(t, db, 75, yesterday, "cus_idem75")

	cron := signup.NewAnomalyCron(db, slack, counter, nil, fixedClock(now))

	require.NoError(t, cron.Run(context.Background()), "first run must succeed")
	require.NoError(t, cron.Run(context.Background()), "second run must succeed")

	assert.Equal(t, int32(1), slack.calls.Load(), "slack must be called only once total")
	assert.Equal(t, float64(1), testCounterValue(counter), "counter must increment only once")
}
