//go:build integration

package webhookevents_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/webhookevents"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

func TestInsertIfNew_IdempotentByEventID(t *testing.T) {
	db := testdb.NewDB(t, "stripe_webhook_events")
	repo := webhookevents.NewRepository()

	ok, err := repo.InsertIfNew(context.Background(), db, webhookevents.StripeWebhookEvent{
		EventID:   "evt_1",
		EventType: "customer.subscription.updated",
		Payload:   []byte(`{}`),
	})
	require.NoError(t, err)
	require.True(t, ok)

	ok, err = repo.InsertIfNew(context.Background(), db, webhookevents.StripeWebhookEvent{
		EventID:   "evt_1",
		EventType: "customer.subscription.updated",
		Payload:   []byte(`{}`),
	})
	require.NoError(t, err)
	require.False(t, ok, "duplicate insert must return false")
}

// TestGetUnprocessed_Filters pins what recovery may see.
//
// This test used to require that e2 — unprocessed, but already attributed to
// a store — was EXCLUDED, which is the defect written down as an assertion.
// The handler attributes an event before it dispatches, so every ordinary
// dispatch failure looked like e2 and was skipped by recovery for good,
// while Stripe had been answered 200 and would not resend. Unprocessed is
// the only thing that decides eligibility now; only a processed event, or
// one a human has been asked to look at, is out.
func TestGetUnprocessed_Filters(t *testing.T) {
	db := testdb.NewDB(t, "stripe_webhook_events")

	require.NoError(t, db.Exec(`INSERT INTO stripe_webhook_events (event_id, event_type, payload) VALUES
        ('e1','t','{}'),
        ('e2','t','{}'),
        ('e3','t','{}'),
        ('e4','t','{}')`).Error)
	storeID := uuid.New().String()
	require.NoError(t, db.Exec(`UPDATE stripe_webhook_events SET store_id = ?::uuid WHERE event_id = 'e2'`, storeID).Error)
	require.NoError(t, db.Exec(`UPDATE stripe_webhook_events SET processed_at = now() WHERE event_id = 'e3'`).Error)
	require.NoError(t, db.Exec(`UPDATE stripe_webhook_events SET manual_review_required = true WHERE event_id = 'e4'`).Error)

	repo := webhookevents.NewRepository()
	pending, err := repo.GetUnprocessed(context.Background(), db, 10)
	require.NoError(t, err)

	got := make([]string, 0, len(pending))
	for _, e := range pending {
		got = append(got, e.EventID)
	}
	require.ElementsMatch(t, []string{"e1", "e2"}, got,
		"an unprocessed event is eligible whether or not it knows its store")
}

// TestClearManualReview_PutsTheEventBackInTheQueue — the flag stops churn,
// but nothing cleared it, so it was a one-way door out of every recovery
// path. cmd/webhook-replay is the way back in.
func TestClearManualReview_PutsTheEventBackInTheQueue(t *testing.T) {
	db := testdb.NewDB(t, "stripe_webhook_events")
	repo := webhookevents.NewRepository()
	ctx := context.Background()

	_, err := repo.InsertIfNew(ctx, db, webhookevents.StripeWebhookEvent{
		EventID:   "evt_stuck",
		EventType: "invoice.paid",
		Payload:   []byte(`{}`),
	})
	require.NoError(t, err)
	_, err = repo.IncrementRetry(ctx, db, "evt_stuck", "handler blew up")
	require.NoError(t, err)
	require.NoError(t, repo.FlagManualReview(ctx, db, "evt_stuck", "retry cap exceeded"))

	stuck, err := repo.ListManualReview(ctx, db, 10)
	require.NoError(t, err)
	require.Len(t, stuck, 1)
	require.Equal(t, "evt_stuck", stuck[0].EventID)

	require.NoError(t, repo.ClearManualReview(ctx, db, "evt_stuck"))

	pending, err := repo.GetUnprocessed(ctx, db, 10)
	require.NoError(t, err)
	require.Len(t, pending, 1)
	require.Equal(t, "evt_stuck", pending[0].EventID)
	require.Zero(t, pending[0].RetryCount, "the retry budget is restored, or it stops again immediately")

	stuck, err = repo.ListManualReview(ctx, db, 10)
	require.NoError(t, err)
	require.Empty(t, stuck)
}

// TestGet_ReturnsTheStoredEvent — the handler reads the existing row on a
// redelivery to decide whether it is a duplicate of work already done.
func TestGet_ReturnsTheStoredEvent(t *testing.T) {
	db := testdb.NewDB(t, "stripe_webhook_events")
	repo := webhookevents.NewRepository()
	ctx := context.Background()

	_, err := repo.InsertIfNew(ctx, db, webhookevents.StripeWebhookEvent{
		EventID:   "evt_get",
		EventType: "invoice.paid",
		Payload:   []byte(`{}`),
	})
	require.NoError(t, err)

	got, err := repo.Get(ctx, db, "evt_get")
	require.NoError(t, err)
	require.Equal(t, "invoice.paid", got.EventType)
	require.Nil(t, got.ProcessedAt)

	require.NoError(t, repo.MarkProcessed(ctx, db, "evt_get"))
	got, err = repo.Get(ctx, db, "evt_get")
	require.NoError(t, err)
	require.NotNil(t, got.ProcessedAt)
}

func TestIncrementRetry_BumpsCountAndRecordsError(t *testing.T) {
	db := testdb.NewDB(t, "stripe_webhook_events")
	repo := webhookevents.NewRepository()

	_, err := repo.InsertIfNew(context.Background(), db, webhookevents.StripeWebhookEvent{
		EventID:   "evt_retry",
		EventType: "t",
		Payload:   []byte(`{}`),
	})
	require.NoError(t, err)

	n, err := repo.IncrementRetry(context.Background(), db, "evt_retry", "first fail")
	require.NoError(t, err)
	require.Equal(t, 1, n)

	n, err = repo.IncrementRetry(context.Background(), db, "evt_retry", "second fail")
	require.NoError(t, err)
	require.Equal(t, 2, n)
}

func TestFlagManualReview_RemovesFromTheRecoveryQueue(t *testing.T) {
	db := testdb.NewDB(t, "stripe_webhook_events")
	repo := webhookevents.NewRepository()

	_, err := repo.InsertIfNew(context.Background(), db, webhookevents.StripeWebhookEvent{
		EventID:   "evt_manual",
		EventType: "t",
		Payload:   []byte(`{}`),
	})
	require.NoError(t, err)

	require.NoError(t, repo.FlagManualReview(context.Background(), db, "evt_manual", "no customer match after 6 retries"))

	pending, err := repo.GetUnprocessed(context.Background(), db, 10)
	require.NoError(t, err)
	for _, o := range pending {
		require.NotEqual(t, "evt_manual", o.EventID)
	}
}
