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

	// GetUnprocessed returns up to `limit` rows with processed_at IS NULL and
	// manual_review_required = false, ordered by received_at.
	//
	// It deliberately does NOT filter on store_id IS NULL, which is what its
	// predecessor (GetUnprocessedOrphans) did. The handler resolves the store
	// and calls SetStoreID BEFORE dispatching, so an event that failed in its
	// handler — the ordinary failure, not the orphan race — came back with
	// store_id populated and was excluded from recovery forever. Stripe had
	// already been told 200, so nothing else was going to retry it either.
	GetUnprocessed(ctx context.Context, db *gorm.DB, limit int) ([]StripeWebhookEvent, error)

	// Get returns one event by id. Used by the handler to decide whether a
	// duplicate delivery is a genuine duplicate or a redelivery of something
	// that never processed.
	Get(ctx context.Context, db *gorm.DB, eventID string) (*StripeWebhookEvent, error)

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

	// ClearManualReview clears the flag and resets retry_count so the recovery
	// loop picks the event up again.
	//
	// The flag is how an event stops churning, but nothing cleared it, so it
	// was a one-way door: every flagged event was excluded from recovery and
	// from the stale alert, permanently and silently. An operator who has
	// fixed the underlying cause needs a way back in — see
	// cmd/webhook-replay.
	ClearManualReview(ctx context.Context, db *gorm.DB, eventID string) error

	// ListManualReview returns up to `limit` events awaiting manual review,
	// oldest first, so an operator can see what is stuck before replaying it.
	ListManualReview(ctx context.Context, db *gorm.DB, limit int) ([]StripeWebhookEvent, error)

	// Acknowledge closes an event out permanently: processed_at is stamped,
	// the manual-review flag cleared, and `reason` recorded in
	// processing_error.
	//
	// For events that will NEVER resolve, which replaying cannot help. On
	// 2026-09-23 all 31 flagged events were of that kind: none carried the
	// mark8ly_store_id metadata CreateSubscription stamps, so none was for a
	// subscription this service created, and no store_subscriptions row held
	// a Stripe customer id for them to match. Replaying would have failed
	// them six more times and flagged them again.
	//
	// Leaving them flagged is not free either: the next genuinely stuck event
	// would be buried among known-dead ones, which is how a queue stops being
	// read.
	Acknowledge(ctx context.Context, db *gorm.DB, eventID, reason string) error
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

func (r *repoImpl) GetUnprocessed(ctx context.Context, db *gorm.DB, limit int) ([]StripeWebhookEvent, error) {
	var rows []StripeWebhookEvent
	err := db.WithContext(ctx).
		Where("processed_at IS NULL AND manual_review_required = false").
		Order("received_at ASC").
		Limit(limit).
		Find(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("webhookevents: GetUnprocessed: %w", err)
	}
	return rows, nil
}

func (r *repoImpl) Get(ctx context.Context, db *gorm.DB, eventID string) (*StripeWebhookEvent, error) {
	var row StripeWebhookEvent
	err := db.WithContext(ctx).
		Where("event_id = ?", eventID).
		First(&row).Error
	if err != nil {
		return nil, fmt.Errorf("webhookevents: Get: %w", err)
	}
	return &row, nil
}

func (r *repoImpl) ClearManualReview(ctx context.Context, db *gorm.DB, eventID string) error {
	res := db.WithContext(ctx).
		Model(&StripeWebhookEvent{}).
		Where("event_id = ?", eventID).
		Updates(map[string]any{
			"manual_review_required": false,
			"retry_count":            0,
		})
	if res.Error != nil {
		return fmt.Errorf("webhookevents: ClearManualReview: %w", res.Error)
	}
	return nil
}

func (r *repoImpl) Acknowledge(ctx context.Context, db *gorm.DB, eventID, reason string) error {
	res := db.WithContext(ctx).
		Model(&StripeWebhookEvent{}).
		Where("event_id = ?", eventID).
		Updates(map[string]any{
			"processed_at":           time.Now(),
			"manual_review_required": false,
			"processing_error":       reason,
		})
	if res.Error != nil {
		return fmt.Errorf("webhookevents: Acknowledge: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return fmt.Errorf("webhookevents: Acknowledge: no event %s", eventID)
	}
	return nil
}

func (r *repoImpl) ListManualReview(ctx context.Context, db *gorm.DB, limit int) ([]StripeWebhookEvent, error) {
	var rows []StripeWebhookEvent
	err := db.WithContext(ctx).
		Where("manual_review_required = true AND processed_at IS NULL").
		Order("received_at ASC").
		Limit(limit).
		Find(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("webhookevents: ListManualReview: %w", err)
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
