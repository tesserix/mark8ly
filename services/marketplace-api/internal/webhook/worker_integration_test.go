//go:build integration

package webhook_test

import (
	"context"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/webhook"
	"github.com/mark8ly/marketplace-api/internal/webhook/ssrfguard"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

func TestWorker_DeliversAndMarksDelivered(t *testing.T) {
	var hits int32
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.WriteHeader(200)
	}))
	defer srv.Close()

	db := testdb.NewDB(t, "webhook_deliveries", "webhook_subscriptions", "outbox_events")
	subs := webhook.NewSubscriptionRepo(db)
	deliveries := webhook.NewDeliveryRepo(db)
	guard := ssrfguard.New(func(string) ([]net.IP, error) {
		return []net.IP{net.ParseIP("93.184.216.34")}, nil
	})
	w := webhook.NewWorker(deliveries, subs, webhook.NewSender(guard, srv.Client()), slog.Default(), 4, nil)
	ctx := context.Background()

	sub := newSub(t, subs, uuid.New(), []string{"order.placed"})
	require.NoError(t, db.Exec(`UPDATE webhook_subscriptions SET url = ? WHERE id = ?`, srv.URL, sub.ID).Error)
	_, err := deliveries.FanOut(ctx, []webhook.Delivery{{
		SubscriptionID: sub.ID, OutboxEventID: uuid.New(), EventType: "order.placed",
		AggregateID: uuid.New(), Status: webhook.StatusPending, NextAttemptAt: time.Now(),
	}})
	require.NoError(t, err)

	n, err := w.Tick(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, n)
	require.EqualValues(t, 1, atomic.LoadInt32(&hits))

	var got webhook.Delivery
	require.NoError(t, db.First(&got).Error)
	require.Equal(t, webhook.StatusDelivered, got.Status)
	require.NotNil(t, got.DeliveredAt)
}

func TestWorker_RetriesWithBackoffThenDeadLetters(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
	}))
	defer srv.Close()

	db := testdb.NewDB(t, "webhook_deliveries", "webhook_subscriptions", "outbox_events")
	subs := webhook.NewSubscriptionRepo(db)
	deliveries := webhook.NewDeliveryRepo(db)
	guard := ssrfguard.New(func(string) ([]net.IP, error) {
		return []net.IP{net.ParseIP("93.184.216.34")}, nil
	})
	w := webhook.NewWorker(deliveries, subs, webhook.NewSender(guard, srv.Client()), slog.Default(), 4, nil)
	ctx := context.Background()

	sub := newSub(t, subs, uuid.New(), []string{"order.placed"})
	require.NoError(t, db.Exec(`UPDATE webhook_subscriptions SET url = ? WHERE id = ?`, srv.URL, sub.ID).Error)
	_, err := deliveries.FanOut(ctx, []webhook.Delivery{{
		SubscriptionID: sub.ID, OutboxEventID: uuid.New(), EventType: "order.placed",
		AggregateID: uuid.New(), Status: webhook.StatusPending, NextAttemptAt: time.Now(),
	}})
	require.NoError(t, err)

	for i := 0; i < webhook.MaxAttempts; i++ {
		_, err := w.Tick(ctx)
		require.NoError(t, err)
		// Make the next attempt due immediately rather than sleeping out the backoff.
		require.NoError(t, db.Exec(`UPDATE webhook_deliveries SET next_attempt_at = now()`).Error)
	}

	var got webhook.Delivery
	require.NoError(t, db.First(&got).Error)
	require.Equal(t, webhook.StatusFailed, got.Status)
	require.Equal(t, webhook.MaxAttempts, got.Attempts)
	require.NotNil(t, got.LastStatusCode)
	require.Equal(t, 500, *got.LastStatusCode)
}

// TestWorker_DisablesSubscriptionExactlyOnceAndNotifies exercises the
// notify-once contract RecordFailure was built for in Task 2: the worker
// must call the notify callback on the tick that flips the subscription
// disabled, and never again for further failures against the same dead
// endpoint.
func TestWorker_DisablesSubscriptionExactlyOnceAndNotifies(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
	}))
	defer srv.Close()

	db := testdb.NewDB(t, "webhook_deliveries", "webhook_subscriptions", "outbox_events")
	subs := webhook.NewSubscriptionRepo(db)
	deliveries := webhook.NewDeliveryRepo(db)
	guard := ssrfguard.New(func(string) ([]net.IP, error) {
		return []net.IP{net.ParseIP("93.184.216.34")}, nil
	})

	var notifyCount int32
	w := webhook.NewWorker(deliveries, subs, webhook.NewSender(guard, srv.Client()), slog.Default(), 4,
		func(webhook.Subscription) { atomic.AddInt32(&notifyCount, 1) })
	ctx := context.Background()

	sub := newSub(t, subs, uuid.New(), []string{"order.placed"})
	require.NoError(t, db.Exec(`UPDATE webhook_subscriptions SET url = ? WHERE id = ?`, srv.URL, sub.ID).Error)

	// RecordFailure only increments once per DEAD-LETTERED delivery (see
	// worker.go's disableIfExhausted), not once per failed HTTP attempt —
	// so tripping FailureThreshold takes that many separate deliveries
	// exhausting their retries, not that many attempts.
	for i := 0; i < webhook.FailureThreshold; i++ {
		_, err := deliveries.FanOut(ctx, []webhook.Delivery{{
			SubscriptionID: sub.ID, OutboxEventID: uuid.New(), EventType: "order.placed",
			AggregateID: uuid.New(), Status: webhook.StatusPending, NextAttemptAt: time.Now(),
		}})
		require.NoError(t, err)
		for a := 0; a < webhook.MaxAttempts; a++ {
			_, err := w.Tick(ctx)
			require.NoError(t, err)
			require.NoError(t, db.Exec(`UPDATE webhook_deliveries SET next_attempt_at = now() WHERE status = ?`, webhook.StatusPending).Error)
		}
	}

	require.EqualValues(t, 1, atomic.LoadInt32(&notifyCount), "notify must fire exactly once")

	var got webhook.Subscription
	require.NoError(t, db.First(&got, "id = ?", sub.ID).Error)
	require.False(t, got.Enabled)
}
