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
type Dispatcher struct {
	db         *gorm.DB
	subs       *SubscriptionRepo
	deliveries *DeliveryRepo
	logger     *slog.Logger
	batch      int
}

func NewDispatcher(db *gorm.DB, subs *SubscriptionRepo, deliveries *DeliveryRepo, logger *slog.Logger, batch int) *Dispatcher {
	if batch <= 0 {
		batch = 100
	}
	return &Dispatcher{db: db, subs: subs, deliveries: deliveries, logger: logger, batch: batch}
}

type outboxRow struct {
	ID          uuid.UUID
	TenantID    uuid.UUID
	AggregateID uuid.UUID
	EventType   string
	CreatedAt   time.Time
}

// dispatchCursor mirrors webhook_dispatch_cursor. LastEventID is compared
// alongside LastEventCreated so ties on created_at (routine in a
// transactional outbox: several events written in one transaction share
// Postgres's `now()`, which is transaction-start time) never hide a row from
// the next read.
type dispatchCursor struct {
	LastEventCreated time.Time
	LastEventID      *uuid.UUID
}

// readCursor loads the dispatcher's cursor. LastEventID is NULL only in the
// seeded initial state (migration 000126 inserts the singleton row with no
// id); every other case runs (created_at, id) > (?, ?), so nil is coalesced
// to the zero UUID rather than left to compare against NULL, which would
// make the predicate match nothing.
func (d *Dispatcher) readCursor(ctx context.Context) (dispatchCursor, error) {
	var cursor dispatchCursor
	if err := d.db.WithContext(ctx).
		Raw(`SELECT last_event_created, last_event_id FROM webhook_dispatch_cursor WHERE id`).
		Scan(&cursor).Error; err != nil {
		return dispatchCursor{}, fmt.Errorf("webhook: read cursor: %w", err)
	}
	if cursor.LastEventID == nil {
		zero := uuid.UUID{}
		cursor.LastEventID = &zero
	}
	return cursor, nil
}

// advanceCursor moves the cursor to the last row of the batch just fanned
// out. GREATEST guards both columns together so the pair stays consistent
// even when multiple dispatcher replicas run this loop concurrently (wired
// up in Task 6) — an unconditional SET would let a slower replica, racing a
// faster one, walk the cursor backward. Idempotent fan-out makes that safe
// rather than lossy, but it is wasted and confusing work, so we guard it.
func (d *Dispatcher) advanceCursor(ctx context.Context, last outboxRow) error {
	err := d.db.WithContext(ctx).Exec(`
		UPDATE webhook_dispatch_cursor
		   SET last_event_created = GREATEST(last_event_created, ?),
		       last_event_id = CASE
		           WHEN ? > last_event_created THEN ?
		           WHEN ? = last_event_created THEN GREATEST(last_event_id, ?)
		           ELSE last_event_id
		       END
		 WHERE id`, last.CreatedAt, last.CreatedAt, last.ID, last.CreatedAt, last.ID).Error
	if err != nil {
		return fmt.Errorf("webhook: advance cursor: %w", err)
	}
	return nil
}

// Tick reads one batch of outbox events past the cursor, fans them out, and
// advances the cursor. Returns how many delivery rows were created.
func (d *Dispatcher) Tick(ctx context.Context) (int, error) {
	cursor, err := d.readCursor(ctx)
	if err != nil {
		return 0, err
	}

	var rows []outboxRow
	if err := d.db.WithContext(ctx).Raw(`
		SELECT id, tenant_id, aggregate_id, event_type, created_at
		  FROM outbox_events
		 WHERE (created_at, id) > (?, ?)
		 ORDER BY created_at ASC, id ASC
		 LIMIT ?`, cursor.LastEventCreated, *cursor.LastEventID, d.batch).Scan(&rows).Error; err != nil {
		return 0, fmt.Errorf("webhook: read outbox: %w", err)
	}
	if len(rows) == 0 {
		return 0, nil
	}

	created, err := d.fanOutRows(ctx, rows)
	if err != nil {
		return created, err
	}

	if err := d.advanceCursor(ctx, rows[len(rows)-1]); err != nil {
		return created, err
	}
	return created, nil
}

// fanOutRows creates delivery rows for every subscription matching each
// outbox row, returning how many were actually created.
func (d *Dispatcher) fanOutRows(ctx context.Context, rows []outboxRow) (int, error) {
	created := 0
	for _, row := range rows {
		matches, err := d.subs.MatchingEvent(ctx, row.TenantID, row.EventType)
		if err != nil {
			return created, err
		}
		pending := make([]Delivery, 0, len(matches))
		for _, s := range matches {
			pending = append(pending, Delivery{
				SubscriptionID: s.ID,
				OutboxEventID:  row.ID,
				EventType:      row.EventType,
				AggregateID:    row.AggregateID,
				Status:         StatusPending,
				NextAttemptAt:  time.Now(),
			})
		}
		n, err := d.deliveries.FanOut(ctx, pending)
		if err != nil {
			return created, err
		}
		created += n
	}
	return created, nil
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
