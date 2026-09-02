package webhook

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Dispatcher turns outbox_events into webhook_deliveries.
//
// It keeps its OWN cursor (webhook_dispatch_cursor) rather than reading the
// outbox publisher's watermark, so the two consumers advance independently.
// A stalled webhook dispatch cannot hold back outbox publishing, and vice
// versa. It never writes to outbox_events.
//
// Each Tick runs two passes over outbox_events — a prompt forward pass and
// a trailing sweep of the settled region. See dispatchCursor.

// DispatchLookback is how far behind "now" a region of outbox_events is
// considered SETTLED — old enough that every transaction which could have
// written a row there has certainly committed.
//
// It exists because created_at ordering is NOT commit ordering.
// OutboxEvent.CreatedAt is stamped at INSERT, but the row stays invisible
// until the business transaction commits — and the enqueue is not the last
// statement before commit (internal/order/service.go enqueues, then does
// more work inside the same transaction). So a row stamped at t=100.000 and
// committed at t=100.400 becomes visible AFTER one stamped t=100.100 and
// committed at t=100.150. A single cursor advancing to the newest row it
// has seen steps over the first and never selects it again: no error, no
// retry, no dead-letter, just silent permanent delivery loss — the worst
// failure this system can produce. Replica clock skew produces the same
// shape on its own, since created_at comes from the POD clock and five KEDA
// replicas each tick every 5s.
//
// The window must exceed the longest plausible business transaction plus
// that skew. Five minutes is several orders of magnitude above both: the
// service's HTTP handlers are bounded well under a minute, and GKE nodes
// run chronyd with skew in the low milliseconds. Over-sizing it only delays
// the safety net, and costs nothing else — the sweep's re-reads are free
// because fan-out is idempotent via ON CONFLICT DO NOTHING on
// (outbox_event_id, subscription_id).
//
// Deliberately NOT solved by writing to outbox_events: this dispatcher
// never does, and that separation from outbox.Publisher is a design
// decision, not an accident.
const DispatchLookback = 5 * time.Minute

type Dispatcher struct {
	db         *gorm.DB
	subs       *SubscriptionRepo
	deliveries *DeliveryRepo
	logger     *slog.Logger
	batch      int
	lookback   time.Duration
}

func NewDispatcher(db *gorm.DB, subs *SubscriptionRepo, deliveries *DeliveryRepo, logger *slog.Logger, batch int) *Dispatcher {
	if batch <= 0 {
		batch = 100
	}
	return &Dispatcher{
		db: db, subs: subs, deliveries: deliveries,
		logger: logger, batch: batch, lookback: DispatchLookback,
	}
}

// WithLookback overrides DispatchLookback for this dispatcher.
//
// It exists for tests: the sweep's whole contract is "a row becomes
// reachable once its region has settled", and proving that at the shipped
// five minutes would mean a five-minute test. Ageing the timestamps
// instead cannot substitute — a row inserted into an ALREADY-settled region
// is not a late commit, it is something the design says cannot happen, and
// a test built on it asserts the wrong guarantee. Production uses the
// constant.
func (d *Dispatcher) WithLookback(v time.Duration) *Dispatcher {
	d.lookback = v
	return d
}

type outboxRow struct {
	ID          uuid.UUID
	TenantID    uuid.UUID
	StoreID     string // payload->>'store_id', parsed in Go — see fanOutRows
	AggregateID uuid.UUID
	EventType   string
	CreatedAt   time.Time
}

// matchKey is one distinct fan-out target set within a batch. The
// dispatcher resolves subscriptions once per key rather than once per row:
// a 100-row batch from one store collapses to a handful of keys, where the
// per-row form ran a match + an insert for every row — ~1000 statements a
// tick with five KEDA replicas scanning the same backlog, against a
// 5-connection pool on a shared db-f1-micro.
type matchKey struct {
	tenantID  uuid.UUID
	storeID   uuid.UUID
	eventType string
}

// dispatchCursor mirrors webhook_dispatch_cursor, which holds TWO
// watermarks over the same table.
//
// LastEvent* is the prompt forward cursor: it advances to the newest row
// each tick actually read, so a freshly committed event is dispatched
// within one tick. On its own it is lossy, for the reason on
// DispatchLookback.
//
// Swept* is the safety net. It trails through the settled region
// (created_at <= now() - DispatchLookback), where visibility is no longer
// in question, so every row is guaranteed to pass under it exactly once
// however its commit interleaved. Rows the forward pass already handled
// simply re-insert as no-ops.
//
// Both are compared as (created_at, id) pairs. Ties on created_at are
// routine in a transactional outbox — several events written in one
// transaction share Postgres's now(), which is transaction-start time — and
// a timestamp-only comparison would hide a row from the next read.
//
// Two independently advancing watermarks, each with its own LIMIT, is also
// what keeps either pass from starving: a single cursor pinned inside the
// lookback window would re-serve the window's oldest rows every tick, so
// the tail of any group larger than one batch would wait out the whole
// window before being delivered.
type dispatchCursor struct {
	LastEventCreated time.Time
	LastEventID      *uuid.UUID
	SweptCreated     time.Time
	SweptID          *uuid.UUID
}

// readCursor loads both watermarks. The id columns are NULL only in the
// seeded initial state (000126 inserts the singleton row with no id, 000127
// copies it into swept_id); every comparison runs
// (created_at, id) > (?, ?), so nil is coalesced to the zero UUID rather
// than left to compare against NULL, which would make the predicate match
// nothing.
func (d *Dispatcher) readCursor(ctx context.Context) (dispatchCursor, error) {
	var cursor dispatchCursor
	if err := d.db.WithContext(ctx).
		Raw(`SELECT last_event_created, last_event_id, swept_created, swept_id
		       FROM webhook_dispatch_cursor WHERE id`).
		Scan(&cursor).Error; err != nil {
		return dispatchCursor{}, fmt.Errorf("webhook: read cursor: %w", err)
	}
	zero := uuid.UUID{}
	if cursor.LastEventID == nil {
		cursor.LastEventID = &zero
	}
	if cursor.SweptID == nil {
		cursor.SweptID = &zero
	}
	return cursor, nil
}

// advanceCursor moves one watermark to the last row of the batch just
// fanned out. GREATEST guards both columns together so the pair stays
// consistent when multiple dispatcher replicas run this loop concurrently —
// an unconditional SET would let a slower replica, racing a faster one,
// walk a watermark backward. Idempotent fan-out makes that safe rather than
// lossy, but it is wasted and confusing work, so we guard it.
//
// createdCol/idCol are column names chosen by this file, never by a caller.
func (d *Dispatcher) advanceCursor(ctx context.Context, createdCol, idCol string, last outboxRow) error {
	err := d.db.WithContext(ctx).Exec(fmt.Sprintf(`
		UPDATE webhook_dispatch_cursor
		   SET %[1]s = GREATEST(%[1]s, ?),
		       %[2]s = CASE
		           WHEN ? > %[1]s THEN ?
		           WHEN ? = %[1]s THEN GREATEST(%[2]s, ?)
		           ELSE %[2]s
		       END
		 WHERE id`, createdCol, idCol),
		last.CreatedAt, last.CreatedAt, last.ID, last.CreatedAt, last.ID).Error
	if err != nil {
		return fmt.Errorf("webhook: advance cursor: %w", err)
	}
	return nil
}

// intervalArg renders a Duration for a Postgres ::interval cast. Kept in
// milliseconds so sub-second windows survive the round trip.
func intervalArg(d time.Duration) string {
	return fmt.Sprintf("%d milliseconds", d.Milliseconds())
}

// Tick runs both passes once and returns how many delivery rows were
// created. See dispatchCursor for why there are two.
func (d *Dispatcher) Tick(ctx context.Context) (int, error) {
	cursor, err := d.readCursor(ctx)
	if err != nil {
		return 0, err
	}

	created, err := d.forwardPass(ctx, cursor)
	if err != nil {
		return created, err
	}
	swept, err := d.sweepPass(ctx, cursor)
	return created + swept, err
}

// forwardPass dispatches everything newer than the forward cursor. This is
// the path that delivers promptly.
func (d *Dispatcher) forwardPass(ctx context.Context, cursor dispatchCursor) (int, error) {
	rows, err := d.read(ctx, `(created_at, id) > (?, ?)`,
		cursor.LastEventCreated, *cursor.LastEventID)
	if err != nil {
		return 0, err
	}
	if len(rows) == 0 {
		return 0, nil
	}
	created, err := d.fanOutRows(ctx, rows)
	if err != nil {
		return created, err
	}
	return created, d.advanceCursor(ctx, "last_event_created", "last_event_id", rows[len(rows)-1])
}

// sweepPass re-walks the settled region, catching anything the forward pass
// stepped over because it committed out of created_at order. now() is SQL's:
// a pod-clock boundary here would reintroduce exactly the skew assumption
// DispatchLookback exists to absorb.
func (d *Dispatcher) sweepPass(ctx context.Context, cursor dispatchCursor) (int, error) {
	rows, err := d.read(ctx, `(created_at, id) > (?, ?) AND created_at <= now() - ?::interval`,
		cursor.SweptCreated, *cursor.SweptID, intervalArg(d.lookback))
	if err != nil {
		return 0, err
	}
	if len(rows) == 0 {
		return 0, nil
	}
	created, err := d.fanOutRows(ctx, rows)
	if err != nil {
		return created, err
	}
	return created, d.advanceCursor(ctx, "swept_created", "swept_id", rows[len(rows)-1])
}

// read loads one batch of outbox rows matching where, oldest first.
func (d *Dispatcher) read(ctx context.Context, where string, args ...any) ([]outboxRow, error) {
	var rows []outboxRow
	args = append(args, d.batch)
	err := d.db.WithContext(ctx).Raw(`
		SELECT id, tenant_id, payload->>'store_id' AS store_id,
		       aggregate_id, event_type, created_at
		  FROM outbox_events
		 WHERE `+where+`
		 ORDER BY created_at ASC, id ASC
		 LIMIT ?`, args...).Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("webhook: read outbox: %w", err)
	}
	return rows, nil
}

// fanOutRows creates delivery rows for every subscription matching each
// outbox row, returning how many were actually created.
//
// Subscriptions are resolved once per distinct (tenant, store, event type)
// across the whole batch and inserted in ONE FanOut, rather than a
// match+insert per row. Per-row semantics are unchanged; the statement
// count is not — see matchKey.
func (d *Dispatcher) fanOutRows(ctx context.Context, rows []outboxRow) (int, error) {
	matches := make(map[matchKey][]Subscription)
	pending := make([]Delivery, 0, len(rows))

	for _, row := range rows {
		// Every outbox producer writes a top-level store_id (the outbox
		// publisher already treats a missing one as the terminal
		// ReasonPayloadMissingStoreID), so this is a producer bug if it
		// ever fires. Skipping is the only safe response: dispatching on an
		// unknown store would fan the event out to the wrong merchant's
		// endpoint, which is the bug this scoping exists to prevent.
		storeID, err := uuid.Parse(row.StoreID)
		if err != nil {
			if d.logger != nil {
				d.logger.Error("webhook: outbox row has no usable store_id; not dispatching",
					"outbox_event_id", row.ID.String(), "event_type", row.EventType)
			}
			continue
		}

		key := matchKey{tenantID: row.TenantID, storeID: storeID, eventType: row.EventType}
		subs, ok := matches[key]
		if !ok {
			subs, err = d.subs.MatchingEvent(ctx, key.tenantID, key.storeID, key.eventType)
			if err != nil {
				return 0, err
			}
			matches[key] = subs
		}

		for _, s := range subs {
			pending = append(pending, Delivery{
				SubscriptionID: s.ID,
				OutboxEventID:  row.ID,
				EventType:      row.EventType,
				AggregateID:    row.AggregateID,
				Status:         StatusPending,
			})
		}
	}

	return d.deliveries.FanOut(ctx, pending)
}

// Start runs Tick on an interval until ctx is cancelled. Mirrors
// outbox.Publisher.Start so the two loops behave the same way on shutdown.
func (d *Dispatcher) Start(ctx context.Context, interval time.Duration) <-chan struct{} {
	done := make(chan struct{})
	go func() {
		defer close(done)
		t := time.NewTicker(interval)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				if _, err := d.Tick(ctx); err != nil && d.logger != nil {
					d.logger.Error("webhook dispatcher tick failed", "err", err)
				}
			}
		}
	}()
	return done
}
