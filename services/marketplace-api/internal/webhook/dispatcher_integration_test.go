//go:build integration

package webhook_test

import (
	"context"
	"encoding/json"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/outbox"
	"github.com/mark8ly/marketplace-api/internal/webhook"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
	"gorm.io/gorm"
)

func enqueueOutbox(t *testing.T, db *gorm.DB, tenant uuid.UUID, eventType string) uuid.UUID {
	t.Helper()
	return enqueueOutboxForStore(t, db, tenant, uuid.New(), eventType)
}

// enqueueOutboxForStore writes an outbox row whose payload carries an
// explicit store_id. Fan-out is scoped by (tenant_id, store_id), so a test
// that cares which subscriptions match has to control the store, not let
// the helper invent one.
func enqueueOutboxForStore(t *testing.T, db *gorm.DB, tenant, store uuid.UUID, eventType string) uuid.UUID {
	t.Helper()
	id := uuid.New()
	payload, _ := json.Marshal(map[string]any{"store_id": store.String()})
	require.NoError(t, db.Exec(`
		INSERT INTO outbox_events (id, tenant_id, aggregate, aggregate_id, event_type, payload)
		VALUES (?, ?, 'order', ?, ?, ?)`,
		id, tenant, uuid.New(), eventType, payload).Error)
	return id
}

// enqueueOutboxAt inserts an outbox row with an explicit created_at, for
// tests that need to control ordering directly rather than relying on
// now() — including two rows sharing the exact same timestamp, which is
// what a transactional outbox produces for events written in one
// transaction (Postgres's now() is transaction-start time).
func enqueueOutboxAt(t *testing.T, db *gorm.DB, tenant, store uuid.UUID, eventType string, createdAt time.Time) uuid.UUID {
	t.Helper()
	id := uuid.New()
	payload, _ := json.Marshal(map[string]any{"store_id": store.String()})
	require.NoError(t, db.Exec(`
		INSERT INTO outbox_events (id, tenant_id, aggregate, aggregate_id, event_type, payload, created_at)
		VALUES (?, ?, 'order', ?, ?, ?, ?)`,
		id, tenant, uuid.New(), eventType, payload, createdAt).Error)
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
	require.NoError(t, db.Exec(`
		UPDATE webhook_dispatch_cursor
		   SET last_event_created = 'epoch', last_event_id = NULL,
		       swept_created = 'epoch', swept_id = NULL`).Error)
}

func TestDispatcher_CreatesOneDeliveryPerMatchingSubscription(t *testing.T) {
	db := testdb.NewDB(t, "webhook_deliveries", "webhook_subscriptions", "outbox_events")
	resetDispatchCursor(t, db)
	subs := webhook.NewSubscriptionRepo(db)
	deliveries := webhook.NewDeliveryRepo(db)
	d := webhook.NewDispatcher(db, subs, deliveries, slog.Default(), 100)
	ctx := context.Background()
	tenant := uuid.New()

	store := uuid.New()
	newSubForStore(t, subs, tenant, store, []string{"order.placed"})
	newSubForStore(t, subs, tenant, store, []string{"order.placed", "order.refunded"})
	newSubForStore(t, subs, tenant, store, []string{"product.created"}) // must not match
	enqueueOutboxForStore(t, db, tenant, store, "order.placed")

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
	store := uuid.New()
	newSubForStore(t, subs, tenant, store, []string{"order.placed"})
	enqueueOutboxForStore(t, db, tenant, store, "order.placed")

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
	store := uuid.New()
	newSubForStore(t, subs, tenant, store, []string{"order.placed"})
	id := enqueueOutboxForStore(t, db, tenant, store, "order.placed")

	_, err := d.Tick(context.Background())
	require.NoError(t, err)

	var row outbox.OutboxEvent
	require.NoError(t, db.First(&row, "id = ?", id).Error)
	require.Nil(t, row.PublishedAt, "webhook dispatch must not mark outbox rows published")
}

// The bug this whole fix round exists to close: a timestamp-only cursor
// silently drops any row that lands on the far side of a batch boundary
// from another row sharing its exact created_at. A transactional outbox
// makes identical timestamps routine — Postgres's now() is transaction
// START time, so several events written in one transaction share it — so
// this is not an edge case.
func TestDispatcher_TieBreaksIdenticalCreatedAtByID(t *testing.T) {
	db := testdb.NewDB(t, "webhook_deliveries", "webhook_subscriptions", "outbox_events")
	resetDispatchCursor(t, db)
	subs := webhook.NewSubscriptionRepo(db)
	deliveries := webhook.NewDeliveryRepo(db)
	ctx := context.Background()
	tenant, store := uuid.New(), uuid.New()
	newSubForStore(t, subs, tenant, store, []string{"order.placed"})

	same := time.Now().UTC().Truncate(time.Microsecond)
	enqueueOutboxAt(t, db, tenant, store, "order.placed", same)
	enqueueOutboxAt(t, db, tenant, store, "order.placed", same)
	enqueueOutboxAt(t, db, tenant, store, "order.placed", same)

	// batch=2 forces the tie group to straddle a page boundary: the first
	// Tick can only take 2 of the 3 tied rows.
	d := webhook.NewDispatcher(db, subs, deliveries, slog.Default(), 2)

	n1, err := d.Tick(ctx)
	require.NoError(t, err)
	require.Equal(t, 2, n1, "first page takes 2 of the 3 tied rows")

	n2, err := d.Tick(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, n2, "the row left behind by the tie must still be dispatched on the next tick, not dropped forever")

	var count int64
	require.NoError(t, db.Model(&webhook.Delivery{}).Count(&count).Error)
	require.EqualValues(t, 3, count, "all 3 tied events must eventually produce a delivery")
}

// Renamed from TestDispatcher_CursorAdvanceNeverMovesBackward, which
// overstated what it proves. Tick's own WHERE clause means advanceCursor can
// never be handed a behind-the-watermark candidate single-threaded, so this
// does NOT exercise the GREATEST guard's real hazard — two dispatcher
// replicas racing, where a slower one's stale read would otherwise walk a
// watermark back. That race is not reproducible in a single-threaded test.
// What this DOES check is the observable contract at the boundary: a Tick
// running against a forward cursor that another writer has already pushed
// ahead leaves that advance intact.
func TestDispatcher_TickLeavesAnAlreadyAdvancedCursorAlone(t *testing.T) {
	db := testdb.NewDB(t, "webhook_deliveries", "webhook_subscriptions", "outbox_events")
	resetDispatchCursor(t, db)
	subs := webhook.NewSubscriptionRepo(db)
	deliveries := webhook.NewDeliveryRepo(db)
	d := webhook.NewDispatcher(db, subs, deliveries, slog.Default(), 100)
	ctx := context.Background()
	tenant, store := uuid.New(), uuid.New()
	newSubForStore(t, subs, tenant, store, []string{"order.placed"})

	base := time.Now().UTC().Truncate(time.Microsecond)
	enqueueOutboxAt(t, db, tenant, store, "order.placed", base)

	n, err := d.Tick(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, n)

	// Simulate a faster replica having pushed both watermarks ahead of
	// anything dispatched so far.
	future := base.Add(time.Hour)
	require.NoError(t, db.Exec(`
		UPDATE webhook_dispatch_cursor
		   SET last_event_created = ?, last_event_id = ?,
		       swept_created = ?, swept_id = ?`,
		future, uuid.New(), future, uuid.New()).Error)

	n2, err := d.Tick(ctx)
	require.NoError(t, err)
	require.Equal(t, 0, n2, "nothing sits ahead of the pushed-forward watermarks")

	var after struct {
		LastEventCreated time.Time
		SweptCreated     time.Time
	}
	require.NoError(t, db.Raw(
		`SELECT last_event_created, swept_created FROM webhook_dispatch_cursor WHERE id`).
		Scan(&after).Error)
	require.False(t, after.LastEventCreated.Before(future), "forward cursor must never move backward")
	require.False(t, after.SweptCreated.Before(future), "sweep watermark must never move backward")
}

// The Critical review finding on the branch: created_at ordering is NOT
// commit ordering. OutboxEvent.CreatedAt is stamped at INSERT, but the row
// is invisible until the business transaction commits — and enqueueOutbox
// is not the last statement before commit (see internal/order/service.go).
// So a row stamped EARLIER can become visible LATER than one stamped after
// it. A cursor that advanced strictly to the newest row it had seen would
// step over the late arrival and never select it again: no error, no retry,
// no dead-letter, just silent permanent delivery loss. Replica clock skew
// on `created_at` (it comes from the pod clock) produces the same shape.
//
// Two watermarks are what close it, and this test drives both. The forward
// cursor (last_event_*) advances freely to the newest row each tick reads,
// so a normal event is dispatched promptly — and so a late arrival IS
// stepped over, which is the first half of what is asserted below. The
// sweep watermark (swept_*) then trails through the region older than
// now() - DispatchLookback, where every transaction that could have written
// a row has certainly committed, so the row it missed is picked up exactly
// once. There is no OR term and no clamp on the forward cursor: each pass
// has its own LIMIT and advances independently, which is what keeps either
// from starving the other's backlog. Idempotent fan-out is what makes the
// overlap between the two passes free.
func TestDispatcher_DispatchesAnOutboxRowThatBecameVisibleBehindTheCursor(t *testing.T) {
	db := testdb.NewDB(t, "webhook_deliveries", "webhook_subscriptions", "outbox_events")
	resetDispatchCursor(t, db)
	subs := webhook.NewSubscriptionRepo(db)
	deliveries := webhook.NewDeliveryRepo(db)
	ctx := context.Background()
	tenant, store := uuid.New(), uuid.New()
	newSubForStore(t, subs, tenant, store, []string{"order.placed"})

	// A short lookback so the settled region catches up within the test
	// rather than five minutes from now. See Dispatcher.WithLookback.
	const lookback = 300 * time.Millisecond
	d := webhook.NewDispatcher(db, subs, deliveries, slog.Default(), 100).WithLookback(lookback)

	now := time.Now().UTC().Truncate(time.Microsecond)

	// The short transaction: stamped later, commits first, so the forward
	// pass sees only it and the cursor walks past.
	enqueueOutboxAt(t, db, tenant, store, "order.placed", now)
	n1, err := d.Tick(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, n1)

	// The long transaction finally commits. Its created_at is OLDER than
	// the row already dispatched, so the forward cursor is past it — this
	// is the row a single-cursor dispatcher loses silently and forever.
	late := enqueueOutboxAt(t, db, tenant, store, "order.placed", now.Add(-100*time.Millisecond))

	// Its region has not settled yet, so nothing happens on this tick.
	n2, err := d.Tick(ctx)
	require.NoError(t, err)
	require.Equal(t, 0, n2)

	// Once the lookback has passed, the sweep reaches it.
	time.Sleep(2 * lookback)
	n3, err := d.Tick(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, n3, "a row that committed behind the cursor must still be dispatched once its region settles")

	var count int64
	require.NoError(t, db.Model(&webhook.Delivery{}).
		Where("outbox_event_id = ?", late).Count(&count).Error)
	require.EqualValues(t, 1, count, "the late-committing event must have produced its delivery")

	// And the sweep must not manufacture a second delivery for the row the
	// forward pass already handled.
	var total int64
	require.NoError(t, db.Model(&webhook.Delivery{}).Count(&total).Error)
	require.EqualValues(t, 2, total)
}

// The second Critical finding: fan-out matched on tenant_id alone, so on a
// multi-store plan (plangate FeatureStores grants 2/5/10) a webhook
// registered on Store A received Store B's events.
func TestDispatcher_FansOutOnlyToSubscriptionsOnTheEventsStore(t *testing.T) {
	db := testdb.NewDB(t, "webhook_deliveries", "webhook_subscriptions", "outbox_events")
	resetDispatchCursor(t, db)
	subs := webhook.NewSubscriptionRepo(db)
	deliveries := webhook.NewDeliveryRepo(db)
	d := webhook.NewDispatcher(db, subs, deliveries, slog.Default(), 100)
	ctx := context.Background()
	tenant := uuid.New()
	storeA, storeB := uuid.New(), uuid.New()

	subA := newSubForStore(t, subs, tenant, storeA, []string{"order.placed"})
	newSubForStore(t, subs, tenant, storeB, []string{"order.placed"})

	enqueueOutboxForStore(t, db, tenant, storeA, "order.placed")

	n, err := d.Tick(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, n, "a store A event must reach store A's subscription only")

	var got []webhook.Delivery
	require.NoError(t, db.Find(&got).Error)
	require.Len(t, got, 1)
	require.Equal(t, subA.ID, got[0].SubscriptionID, "store B's subscription must not receive store A's event")
}
