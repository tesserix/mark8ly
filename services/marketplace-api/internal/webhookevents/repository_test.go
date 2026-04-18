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

func TestGetUnprocessedOrphans_Filters(t *testing.T) {
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
	orphans, err := repo.GetUnprocessedOrphans(context.Background(), db, 10)
	require.NoError(t, err)
	require.Len(t, orphans, 1)
	require.Equal(t, "e1", orphans[0].EventID)
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

func TestFlagManualReview_RemovesFromOrphanQueue(t *testing.T) {
	db := testdb.NewDB(t, "stripe_webhook_events")
	repo := webhookevents.NewRepository()

	_, err := repo.InsertIfNew(context.Background(), db, webhookevents.StripeWebhookEvent{
		EventID:   "evt_manual",
		EventType: "t",
		Payload:   []byte(`{}`),
	})
	require.NoError(t, err)

	require.NoError(t, repo.FlagManualReview(context.Background(), db, "evt_manual", "no customer match after 6 retries"))

	orphans, err := repo.GetUnprocessedOrphans(context.Background(), db, 10)
	require.NoError(t, err)
	for _, o := range orphans {
		require.NotEqual(t, "evt_manual", o.EventID)
	}
}
