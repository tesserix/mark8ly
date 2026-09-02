//go:build integration

package webhook_test

import (
	"context"
	"encoding/json"
	"log/slog"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/outbox"
	"github.com/mark8ly/marketplace-api/internal/webhook"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
	"gorm.io/gorm"
)

func enqueueOutbox(t *testing.T, db *gorm.DB, tenant uuid.UUID, eventType string) uuid.UUID {
	t.Helper()
	id := uuid.New()
	payload, _ := json.Marshal(map[string]any{"store_id": uuid.New().String()})
	require.NoError(t, db.Exec(`
		INSERT INTO outbox_events (id, tenant_id, aggregate, aggregate_id, event_type, payload)
		VALUES (?, ?, 'order', ?, ?, ?)`,
		id, tenant, uuid.New(), eventType, payload).Error)
	return id
}

// resetDispatchCursor rewinds the dispatcher's cursor to the epoch so each
// test is independent of execution order.
//
// webhook_dispatch_cursor is deliberately NOT in any testdb.NewDB cleanup
// list: migration 000126 seeds it as a singleton row
// (INSERT ... ON CONFLICT DO NOTHING), and TRUNCATE would delete that row.
// With no row left, Tick's `UPDATE webhook_dispatch_cursor ... WHERE id`
// would match zero rows, so the cursor would silently never advance — a
// dispatcher test that passes while dispatching nothing. Resetting the
// existing row's value, instead of truncating the table, keeps the row in
// place.
func resetDispatchCursor(t *testing.T, db *gorm.DB) {
	t.Helper()
	require.NoError(t, db.Exec(
		`UPDATE webhook_dispatch_cursor SET last_event_created = 'epoch', last_event_id = NULL`).Error)
}

func TestDispatcher_CreatesOneDeliveryPerMatchingSubscription(t *testing.T) {
	db := testdb.NewDB(t, "webhook_deliveries", "webhook_subscriptions", "outbox_events")
	resetDispatchCursor(t, db)
	subs := webhook.NewSubscriptionRepo(db)
	deliveries := webhook.NewDeliveryRepo(db)
	d := webhook.NewDispatcher(db, subs, deliveries, slog.Default(), 100)
	ctx := context.Background()
	tenant := uuid.New()

	newSub(t, subs, tenant, []string{"order.placed"})
	newSub(t, subs, tenant, []string{"order.placed", "order.refunded"})
	newSub(t, subs, tenant, []string{"product.created"}) // must not match
	enqueueOutbox(t, db, tenant, "order.placed")

	n, err := d.Tick(ctx)
	require.NoError(t, err)
	require.Equal(t, 2, n, "one delivery per subscription that selected the type")
}

// The property that replaces the exactly-once guarantee we gave up by NOT
// coupling to the outbox publisher's transaction.
func TestDispatcher_IsIdempotentAcrossRuns(t *testing.T) {
	db := testdb.NewDB(t, "webhook_deliveries", "webhook_subscriptions", "outbox_events")
	resetDispatchCursor(t, db)
	subs := webhook.NewSubscriptionRepo(db)
	deliveries := webhook.NewDeliveryRepo(db)
	ctx := context.Background()
	tenant := uuid.New()
	newSub(t, subs, tenant, []string{"order.placed"})
	enqueueOutbox(t, db, tenant, "order.placed")

	first := webhook.NewDispatcher(db, subs, deliveries, slog.Default(), 100)
	n1, err := first.Tick(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, n1)

	// A dispatcher starting from a fresh cursor re-reads the same rows —
	// the unique index must stop it double-delivering.
	resetDispatchCursor(t, db)
	second := webhook.NewDispatcher(db, subs, deliveries, slog.Default(), 100)
	n2, err := second.Tick(ctx)
	require.NoError(t, err)
	require.Equal(t, 0, n2, "re-dispatching the same outbox rows must create nothing")

	var count int64
	require.NoError(t, db.Model(&webhook.Delivery{}).Count(&count).Error)
	require.EqualValues(t, 1, count)
}

// The failure this whole architecture exists to prevent.
func TestDispatcher_DoesNotTouchOutboxPublishedState(t *testing.T) {
	db := testdb.NewDB(t, "webhook_deliveries", "webhook_subscriptions", "outbox_events")
	resetDispatchCursor(t, db)
	subs := webhook.NewSubscriptionRepo(db)
	d := webhook.NewDispatcher(db, subs, webhook.NewDeliveryRepo(db), slog.Default(), 100)
	tenant := uuid.New()
	newSub(t, subs, tenant, []string{"order.placed"})
	id := enqueueOutbox(t, db, tenant, "order.placed")

	_, err := d.Tick(context.Background())
	require.NoError(t, err)

	var row outbox.OutboxEvent
	require.NoError(t, db.First(&row, "id = ?", id).Error)
	require.Nil(t, row.PublishedAt, "webhook dispatch must not mark outbox rows published")
}
