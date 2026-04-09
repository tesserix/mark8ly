package outbox

import (
	"context"
	"fmt"

	"gorm.io/gorm"
)

// Repository is the data-access interface for outbox_events.
type Repository interface {
	EnqueueInTx(ctx context.Context, tx *gorm.DB, evt *OutboxEvent) error
	// ProcessBatch opens its own transaction, locks up to `limit` unpublished
	// rows via FOR UPDATE SKIP LOCKED, and calls fn with the rows and the
	// same tx. If fn returns nil the tx commits (the caller is expected to
	// have called MarkPublishedInTx inside fn); if fn returns an error the
	// tx rolls back and the rows become visible to the next poll. Returns
	// the number of rows the callback saw.
	ProcessBatch(ctx context.Context, limit int,
		fn func(tx *gorm.DB, rows []OutboxEvent) error) (int, error)
	MarkPublishedInTx(tx *gorm.DB, ids []string) error
}

type gormRepository struct {
	db *gorm.DB
}

func NewRepository(db *gorm.DB) Repository { return &gormRepository{db: db} }

func (r *gormRepository) EnqueueInTx(ctx context.Context, tx *gorm.DB, evt *OutboxEvent) error {
	if err := tx.WithContext(ctx).Create(evt).Error; err != nil {
		return fmt.Errorf("outbox: enqueue: %w", err)
	}
	return nil
}

func (r *gormRepository) ProcessBatch(ctx context.Context, limit int,
	fn func(tx *gorm.DB, rows []OutboxEvent) error) (int, error) {
	var seen int
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var rows []OutboxEvent
		if err := tx.Raw(`
			SELECT id, tenant_id, aggregate, aggregate_id, event_type,
			       payload, created_at, published_at, error
			FROM outbox_events
			WHERE published_at IS NULL
			ORDER BY tenant_id, created_at
			LIMIT ?
			FOR UPDATE SKIP LOCKED`, limit).Scan(&rows).Error; err != nil {
			return fmt.Errorf("outbox: poll: %w", err)
		}
		seen = len(rows)
		if seen == 0 {
			return nil
		}
		return fn(tx, rows)
	})
	return seen, err
}

func (r *gormRepository) MarkPublishedInTx(tx *gorm.DB, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	if err := tx.Exec(`UPDATE outbox_events SET published_at = now() WHERE id IN ?`,
		ids).Error; err != nil {
		return fmt.Errorf("outbox: mark published: %w", err)
	}
	return nil
}
