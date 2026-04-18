package webhookevents

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Repository wraps access to the stripe_webhook_events table.
//
// InsertIfNew is the idempotency gate: Stripe retries webhook POSTs, so
// the handler layer calls InsertIfNew first and only dispatches when this
// returns (true, nil) — meaning the row was actually inserted.
type Repository interface {
	// InsertIfNew attempts to insert the event. Returns (true, nil) when a
	// new row is created; (false, nil) when the event_id already exists
	// (ON CONFLICT DO NOTHING).
	InsertIfNew(ctx context.Context, db *gorm.DB, e StripeWebhookEvent) (bool, error)

	// GetUnprocessedOrphans returns up to `limit` rows with processed_at IS NULL,
	// store_id IS NULL, and manual_review_required = false. Ordered by received_at.
	GetUnprocessedOrphans(ctx context.Context, db *gorm.DB, limit int) ([]StripeWebhookEvent, error)

	// MarkProcessed stamps processed_at = now() for the event_id.
	MarkProcessed(ctx context.Context, db *gorm.DB, eventID string) error

	// SetStoreID backfills (store_id, tenant_id) on a previously-orphaned event.
	SetStoreID(ctx context.Context, db *gorm.DB, eventID string, storeID, tenantID uuid.UUID) error

	// IncrementRetry bumps retry_count by 1 and records processing_error.
	// Returns the new retry_count so the caller can decide when to flag manual review.
	IncrementRetry(ctx context.Context, db *gorm.DB, eventID string, errMsg string) (int, error)

	// FlagManualReview sets manual_review_required = true and records the reason
	// in processing_error. Used after OrphanRetryMaxCount exceeded.
	FlagManualReview(ctx context.Context, db *gorm.DB, eventID string, reason string) error
}

type repoImpl struct{}

func NewRepository() Repository { return &repoImpl{} }

func (r *repoImpl) InsertIfNew(ctx context.Context, db *gorm.DB, e StripeWebhookEvent) (bool, error) {
	res := db.WithContext(ctx).Exec(
		`INSERT INTO stripe_webhook_events (event_id, event_type, store_id, tenant_id, payload)
         VALUES (?, ?, ?, ?, ?::jsonb)
         ON CONFLICT (event_id) DO NOTHING`,
		e.EventID, e.EventType, e.StoreID, e.TenantID, string(e.Payload),
	)
	if res.Error != nil {
		return false, fmt.Errorf("webhookevents: InsertIfNew: %w", res.Error)
	}
	return res.RowsAffected > 0, nil
}

func (r *repoImpl) GetUnprocessedOrphans(ctx context.Context, db *gorm.DB, limit int) ([]StripeWebhookEvent, error) {
	var rows []StripeWebhookEvent
	err := db.WithContext(ctx).
		Where("processed_at IS NULL AND store_id IS NULL AND manual_review_required = false").
		Order("received_at ASC").
		Limit(limit).
		Find(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("webhookevents: GetUnprocessedOrphans: %w", err)
	}
	return rows, nil
}

func (r *repoImpl) MarkProcessed(ctx context.Context, db *gorm.DB, eventID string) error {
	res := db.WithContext(ctx).
		Model(&StripeWebhookEvent{}).
		Where("event_id = ?", eventID).
		Update("processed_at", time.Now())
	if res.Error != nil {
		return fmt.Errorf("webhookevents: MarkProcessed: %w", res.Error)
	}
	return nil
}

func (r *repoImpl) SetStoreID(ctx context.Context, db *gorm.DB, eventID string, storeID, tenantID uuid.UUID) error {
	res := db.WithContext(ctx).
		Model(&StripeWebhookEvent{}).
		Where("event_id = ?", eventID).
		Updates(map[string]any{
			"store_id":  storeID,
			"tenant_id": tenantID,
		})
	if res.Error != nil {
		return fmt.Errorf("webhookevents: SetStoreID: %w", res.Error)
	}
	return nil
}

func (r *repoImpl) IncrementRetry(ctx context.Context, db *gorm.DB, eventID string, errMsg string) (int, error) {
	var row StripeWebhookEvent
	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses().Where("event_id = ?", eventID).First(&row).Error; err != nil {
			return err
		}
		row.RetryCount++
		row.ProcessingError = &errMsg
		return tx.Save(&row).Error
	})
	if err != nil {
		return 0, fmt.Errorf("webhookevents: IncrementRetry: %w", err)
	}
	return row.RetryCount, nil
}

func (r *repoImpl) FlagManualReview(ctx context.Context, db *gorm.DB, eventID, reason string) error {
	res := db.WithContext(ctx).
		Model(&StripeWebhookEvent{}).
		Where("event_id = ?", eventID).
		Updates(map[string]any{
			"manual_review_required": true,
			"processing_error":       reason,
		})
	if res.Error != nil {
		return fmt.Errorf("webhookevents: FlagManualReview: %w", res.Error)
	}
	return nil
}
