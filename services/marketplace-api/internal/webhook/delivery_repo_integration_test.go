//go:build integration

package webhook_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/webhook"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

// TestClaimDue_LeasesSoASecondClaimCannotSeeTheSameRow is the fix for the
// Critical review finding on this task: FOR UPDATE SKIP LOCKED's row lock
// is released the instant ClaimDue's own transaction commits — long before
// a real Send (up to RequestTimeout later) completes and RecordOutcome
// moves status off pending. Without a lease, a second worker calling
// ClaimDue in that window would see the row as due and unlocked, and claim
// (and therefore send) it again.
//
// This drives ClaimDue twice in a row with no RecordOutcome between the
// calls — standing in for two concurrent workers, the first of which is
// still mid-Send when the second polls. Before the lease fix, the second
// call returned the same row a second time; after it, the row's
// next_attempt_at has been pushed into the future by the first claim, so
// the second call sees nothing due.
func TestClaimDue_LeasesSoASecondClaimCannotSeeTheSameRow(t *testing.T) {
	db := testdb.NewDB(t, "webhook_deliveries", "webhook_subscriptions", "outbox_events")
	subs := webhook.NewSubscriptionRepo(db)
	deliveries := webhook.NewDeliveryRepo(db)
	ctx := context.Background()

	sub := newSub(t, subs, uuid.New(), []string{"order.placed"})
	_, err := deliveries.FanOut(ctx, []webhook.Delivery{{
		SubscriptionID: sub.ID, OutboxEventID: uuid.New(), EventType: "order.placed",
		AggregateID: uuid.New(), Status: webhook.StatusPending, NextAttemptAt: time.Now(),
	}})
	require.NoError(t, err)

	first, err := deliveries.ClaimDue(ctx, 10)
	require.NoError(t, err)
	require.Len(t, first, 1, "first claim must see the due delivery")

	second, err := deliveries.ClaimDue(ctx, 10)
	require.NoError(t, err)
	require.Empty(t, second, "second claim must NOT re-see a delivery the first claim already leased — that would be a duplicate send")
}

// TestClaimDue_PicksTheRowBackUpOnceTheLeaseExpires proves the other half
// of the contract: a claim that never reaches RecordOutcome (the worker
// died mid-send, say) is not lost forever — it becomes claimable again once
// the lease window passes, which is what makes this at-least-once rather
// than at-most-once.
func TestClaimDue_PicksTheRowBackUpOnceTheLeaseExpires(t *testing.T) {
	db := testdb.NewDB(t, "webhook_deliveries", "webhook_subscriptions", "outbox_events")
	subs := webhook.NewSubscriptionRepo(db)
	deliveries := webhook.NewDeliveryRepo(db)
	ctx := context.Background()

	sub := newSub(t, subs, uuid.New(), []string{"order.placed"})
	_, err := deliveries.FanOut(ctx, []webhook.Delivery{{
		SubscriptionID: sub.ID, OutboxEventID: uuid.New(), EventType: "order.placed",
		AggregateID: uuid.New(), Status: webhook.StatusPending, NextAttemptAt: time.Now(),
	}})
	require.NoError(t, err)

	first, err := deliveries.ClaimDue(ctx, 10)
	require.NoError(t, err)
	require.Len(t, first, 1)

	// Simulate the lease expiring without the worker ever calling
	// RecordOutcome, by rewinding next_attempt_at past "due" directly
	// rather than sleeping out LeaseWindow in a test.
	require.NoError(t, db.Exec(`UPDATE webhook_deliveries SET next_attempt_at = now() WHERE id = ?`, first[0].ID).Error)

	again, err := deliveries.ClaimDue(ctx, 10)
	require.NoError(t, err)
	require.Len(t, again, 1, "an expired lease must make the row claimable again")
}

// TestFanOut_InsertsABatchLargerThanThePostgresParameterLimit covers the
// HIGH found reviewing the batched fan-out. Batching moved FanOut from one
// statement per outbox row to ONE statement for the whole batch, so its row
// count became batch x matching_subscriptions. At 5 bound parameters per
// row, Postgres's 65535-parameter ceiling is 13107 rows — reachable with
// ~132 enabled subscriptions on one store for one event type at batch=100,
// and nothing caps subscription count.
//
// The failure mode is what makes this load-bearing rather than cosmetic: a
// FanOut error returns before advanceCursor, so the cursor never moves, the
// same batch is re-read every 5s, and NO tenant's webhooks dispatch at all.
// One merchant with too many subscriptions would take the subsystem down
// globally — the poison-pill shape internal/outbox documents removing in
// #374.
//
// Driving it through real subscriptions would need 13k of them; the
// parameter count is per-STATEMENT and does not care where the rows came
// from, so one subscription and distinct outbox_event_ids reproduces it
// exactly.
func TestFanOut_InsertsABatchLargerThanThePostgresParameterLimit(t *testing.T) {
	db := testdb.NewDB(t, "webhook_deliveries", "webhook_subscriptions", "outbox_events")
	subs := webhook.NewSubscriptionRepo(db)
	deliveries := webhook.NewDeliveryRepo(db)
	ctx := context.Background()

	sub := newSub(t, subs, uuid.New(), []string{"order.placed"})

	// 5 params per row, so 65535/5 = 13107 is the ceiling. Go just past it.
	const rows = 13200
	pending := make([]webhook.Delivery, 0, rows)
	for i := 0; i < rows; i++ {
		pending = append(pending, webhook.Delivery{
			SubscriptionID: sub.ID, OutboxEventID: uuid.New(), EventType: "order.placed",
			AggregateID: uuid.New(), Status: webhook.StatusPending,
		})
	}

	n, err := deliveries.FanOut(ctx, pending)
	require.NoError(t, err, "a batch past the parameter limit must not error — an error here stalls dispatch for every tenant, forever")
	require.Equal(t, rows, n, "every row must be inserted, and RowsAffected summed across chunks")

	var count int64
	require.NoError(t, db.Model(&webhook.Delivery{}).Count(&count).Error)
	require.EqualValues(t, rows, count)

	// Chunking must not break idempotency: re-running inserts nothing.
	again, err := deliveries.FanOut(ctx, pending)
	require.NoError(t, err)
	require.Equal(t, 0, again, "ON CONFLICT DO NOTHING must still hold across chunk boundaries")
}
