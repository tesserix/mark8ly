package webhook

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type SubscriptionRepo struct{ db *gorm.DB }

func NewSubscriptionRepo(db *gorm.DB) *SubscriptionRepo { return &SubscriptionRepo{db: db} }

func (r *SubscriptionRepo) Create(ctx context.Context, s *Subscription) error {
	if err := r.db.WithContext(ctx).Create(s).Error; err != nil {
		return fmt.Errorf("webhook: create subscription: %w", err)
	}
	return nil
}

func (r *SubscriptionRepo) ListForStore(ctx context.Context, tenantID, storeID uuid.UUID) ([]Subscription, error) {
	var out []Subscription
	err := r.db.WithContext(ctx).
		Where("tenant_id = ? AND store_id = ?", tenantID, storeID).
		Order("created_at DESC").Find(&out).Error
	if err != nil {
		return nil, fmt.Errorf("webhook: list subscriptions: %w", err)
	}
	return out, nil
}

// MatchingEvent returns the ENABLED subscriptions for tenantID that selected
// eventType. `event_types @> ARRAY[?]` uses the array containment operator so
// the match happens in Postgres rather than by loading every subscription.
func (r *SubscriptionRepo) MatchingEvent(ctx context.Context, tenantID uuid.UUID, eventType string) ([]Subscription, error) {
	var out []Subscription
	err := r.db.WithContext(ctx).
		Where("tenant_id = ? AND enabled AND event_types @> ARRAY[?]::text[]", tenantID, eventType).
		Find(&out).Error
	if err != nil {
		return nil, fmt.Errorf("webhook: match subscriptions: %w", err)
	}
	return out, nil
}

// RecordFailure increments the consecutive-failure counter and disables the
// subscription once it reaches threshold, reporting whether it did.
//
// The increment and the disable happen in ONE statement so two delivery
// workers failing concurrently cannot interleave a read-modify-write and
// lose a count.
func (r *SubscriptionRepo) RecordFailure(ctx context.Context, id uuid.UUID, threshold int) (bool, error) {
	var enabled bool
	err := r.db.WithContext(ctx).Raw(`
		UPDATE webhook_subscriptions
		   SET consecutive_failures = consecutive_failures + 1,
		       enabled = CASE WHEN consecutive_failures + 1 >= ? THEN false ELSE enabled END,
		       disabled_reason = CASE WHEN consecutive_failures + 1 >= ?
		            THEN 'Disabled automatically after ' || (consecutive_failures + 1) ||
		                 ' consecutive delivery failures. Fix the endpoint and re-enable.'
		            ELSE disabled_reason END,
		       disabled_at = CASE WHEN consecutive_failures + 1 >= ? THEN now() ELSE disabled_at END,
		       updated_at = now()
		 WHERE id = ?
		RETURNING enabled`, threshold, threshold, threshold, id).Scan(&enabled).Error
	if err != nil {
		return false, fmt.Errorf("webhook: record failure: %w", err)
	}
	return !enabled, nil
}

// RecordSuccess clears the counter. Without this an endpoint that fails
// occasionally over weeks would eventually be disabled despite working.
func (r *SubscriptionRepo) RecordSuccess(ctx context.Context, id uuid.UUID) error {
	err := r.db.WithContext(ctx).Exec(`
		UPDATE webhook_subscriptions
		   SET consecutive_failures = 0, updated_at = now()
		 WHERE id = ? AND consecutive_failures <> 0`, id).Error
	if err != nil {
		return fmt.Errorf("webhook: record success: %w", err)
	}
	return nil
}
