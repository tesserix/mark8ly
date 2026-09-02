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

// Tick reads one batch of outbox events past the cursor, fans them out, and
// advances the cursor. Returns how many delivery rows were created.
func (d *Dispatcher) Tick(ctx context.Context) (int, error) {
	var cursor time.Time
	if err := d.db.WithContext(ctx).
		Raw(`SELECT last_event_created FROM webhook_dispatch_cursor WHERE id`).
		Scan(&cursor).Error; err != nil {
		return 0, fmt.Errorf("webhook: read cursor: %w", err)
	}

	var rows []outboxRow
	if err := d.db.WithContext(ctx).Raw(`
		SELECT id, tenant_id, aggregate_id, event_type, created_at
		  FROM outbox_events
		 WHERE created_at > ?
		 ORDER BY created_at ASC
		 LIMIT ?`, cursor, d.batch).Scan(&rows).Error; err != nil {
		return 0, fmt.Errorf("webhook: read outbox: %w", err)
	}
	if len(rows) == 0 {
		return 0, nil
	}

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

	last := rows[len(rows)-1]
	if err := d.db.WithContext(ctx).Exec(`
		UPDATE webhook_dispatch_cursor
		   SET last_event_created = ?, last_event_id = ?
		 WHERE id`, last.CreatedAt, last.ID).Error; err != nil {
		return created, fmt.Errorf("webhook: advance cursor: %w", err)
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
