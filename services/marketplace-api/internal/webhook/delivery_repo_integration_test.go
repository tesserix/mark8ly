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
