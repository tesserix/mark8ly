//go:build integration

package webhook_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/webhook"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

// TestPrune_RemovesRowsPastRetentionAndKeepsRecentOnes is the delivery-table
// counterpart to the webhook_events prune in internal/webhookprune: 30 days,
// no plan gate. It ages exactly one of two delivered rows past the window
// and asserts Prune removes only that row.
//
// db is scoped with testdb.NewDB(t, "webhook_deliveries", "webhook_subscriptions",
// "outbox_events") rather than the zero-arg form — a bare Count() here would
// otherwise be polluted by delivery rows FanOut-ed by sibling tests in this
// package (the exact issue hit in Task 4).
func TestPrune_RemovesRowsPastRetentionAndKeepsRecentOnes(t *testing.T) {
	db := testdb.NewDB(t, "webhook_deliveries", "webhook_subscriptions", "outbox_events")
	subs := webhook.NewSubscriptionRepo(db)
	deliveries := webhook.NewDeliveryRepo(db)
	ctx := context.Background()
	sub := newSub(t, subs, uuid.New(), []string{"order.placed"})

	_, err := deliveries.FanOut(ctx, []webhook.Delivery{
		{SubscriptionID: sub.ID, OutboxEventID: uuid.New(), EventType: "order.placed", AggregateID: uuid.New(), Status: webhook.StatusDelivered},
		{SubscriptionID: sub.ID, OutboxEventID: uuid.New(), EventType: "order.placed", AggregateID: uuid.New(), Status: webhook.StatusDelivered},
	})
	require.NoError(t, err)

	// Age exactly one row past the window.
	require.NoError(t, db.Exec(`
		UPDATE webhook_deliveries SET created_at = now() - interval '31 days'
		 WHERE id = (SELECT id FROM webhook_deliveries LIMIT 1)`).Error)

	n, err := deliveries.Prune(ctx, webhook.RetentionWindow)
	require.NoError(t, err)
	require.EqualValues(t, 1, n)

	var remaining int64
	require.NoError(t, db.Model(&webhook.Delivery{}).Count(&remaining).Error)
	require.EqualValues(t, 1, remaining, "a delivery inside the window must survive")
}
