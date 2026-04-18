//go:build integration

package dispatch_test

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/billing/dispatch"
	"github.com/mark8ly/marketplace-api/internal/webhookevents"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

type fakePagerDuty struct {
	calls int32
	last  string
}

func (f *fakePagerDuty) Trigger(ctx context.Context, summary string) error {
	atomic.AddInt32(&f.calls, 1)
	f.last = summary
	return nil
}

func TestCron_PagerDutyOnUnresolved1h(t *testing.T) {
	db := testdb.NewDB(t, "stripe_webhook_events", "store_subscriptions")

	require.NoError(t, db.Exec(
		`INSERT INTO stripe_webhook_events (event_id, event_type, payload, received_at)
         VALUES ('evt_stale','checkout.session.completed','{"data":{"object":{"customer":"cus_missing"}}}'::jsonb, now() - interval '61 minutes')`,
	).Error)

	pd := &fakePagerDuty{}
	c := dispatch.NewCron(dispatch.CronConfig{
		DB: db, Repo: webhookevents.NewRepository(), Dispatcher: dispatch.New(),
		PagerDuty: pd, StaleThreshold: time.Hour,
	})
	require.NoError(t, c.RunOnce(context.Background()))

	require.EqualValues(t, 1, atomic.LoadInt32(&pd.calls))
	require.Contains(t, pd.last, "evt_stale")
}

func TestCron_FreshEventDoesNotPage(t *testing.T) {
	db := testdb.NewDB(t, "stripe_webhook_events", "store_subscriptions")

	require.NoError(t, db.Exec(
		`INSERT INTO stripe_webhook_events (event_id, event_type, payload)
         VALUES ('evt_fresh','checkout.session.completed','{"data":{"object":{"customer":"cus_missing"}}}'::jsonb)`,
	).Error)

	pd := &fakePagerDuty{}
	c := dispatch.NewCron(dispatch.CronConfig{
		DB: db, Repo: webhookevents.NewRepository(), Dispatcher: dispatch.New(),
		PagerDuty: pd, StaleThreshold: time.Hour,
	})
	require.NoError(t, c.RunOnce(context.Background()))

	require.EqualValues(t, 0, atomic.LoadInt32(&pd.calls))
}
