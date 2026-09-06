//go:build integration

package signup_test

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/signup"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
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

// fakeSlack records calls to Send.
type fakeSlack struct {
	calls atomic.Int32
}

func (f *fakeSlack) Send(_ context.Context, _ string) error {
	f.calls.Add(1)
	return nil
}

// seedSignups inserts n store_subscriptions rows with created_at = ts, each
// with its own stores parent row.
//
// The parent is not optional and cannot be shared. store_subscriptions.store_id
// has referenced stores(id) since migration 000015, so a synthetic
// gen_random_uuid() store_id is rejected by store_subscriptions_store_id_fkey;
// and UNIQUE (store_id) on the same table means n subscriptions need n stores.
//
// prefix distinguishes one test's rows from another's in stripe_customer_id.
// No cleanup is registered here: the caller's testdb.NewDB truncates
// store_subscriptions, stores and signup_anomaly_log on test completion, and
// testdb.SeedStore registers its own per-store row and sequence cleanup.
func seedSignups(t *testing.T, db *gorm.DB, n int, ts time.Time, prefix string) {
	t.Helper()
	for i := 0; i < n; i++ {
		tenantID, storeID := uuid.New(), uuid.New()
		testdb.SeedStore(t, db, tenantID, storeID)

		err := db.Exec(`
			INSERT INTO store_subscriptions
				(id, tenant_id, store_id, stripe_customer_id, plan, status, subscription_period, price_tier, created_at, updated_at)
			VALUES
				(gen_random_uuid(), ?, ?, ?, 'trial', 'trialing', 'monthly', 'developed', ?, now())`,
			tenantID, storeID, fmt.Sprintf("%s_%d", prefix, i), ts,
		).Error
		require.NoError(t, err)
	}
}

// fixedClock returns a clock func that always returns t.
func fixedClock(t time.Time) func() time.Time { return func() time.Time { return t } }

// TestAnomalyCron_QuietUnder50_NoSlack_NoCounter seeds 49 rows created
// yesterday and asserts: no Slack send, no log row, counter stays at 0.
func TestAnomalyCron_QuietUnder50_NoSlack_NoCounter(t *testing.T) {
	db := testdb.NewDB(t, "store_subscriptions", "stores", "signup_anomaly_log")
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
	db := testdb.NewDB(t, "store_subscriptions", "stores", "signup_anomaly_log")
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
	db := testdb.NewDB(t, "store_subscriptions", "stores", "signup_anomaly_log")
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
