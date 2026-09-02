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
// subscription once it reaches threshold, reporting whether THIS call is the
// one that flipped it from enabled to disabled — not whether it is disabled
// now. A subscription already disabled by a prior call must report false, or
// Task 5's "notify the merchant once" logic would re-fire on every
// subsequent failure against a dead endpoint.
//
// The `old` CTE takes the row lock (FOR UPDATE) and `upd` reads
// consecutive_failures/enabled through that same locked row, so the whole
// read-increment-disable happens as one statement: two delivery workers
// failing concurrently cannot interleave a read-modify-write and lose a
// count.
//
// An id that matches no row leaves both CTEs empty, so the final SELECT
// returns zero rows; RowsAffected is then 0 and we report (false, nil)
// rather than misreporting a disable for a subscription that doesn't exist.
func (r *SubscriptionRepo) RecordFailure(ctx context.Context, id uuid.UUID, threshold int) (bool, error) {
	var result struct {
		WasEnabled bool
		IsEnabled  bool
	}
	tx := r.db.WithContext(ctx).Raw(`
		WITH old AS (
			SELECT enabled AS was_enabled, consecutive_failures
			  FROM webhook_subscriptions
			 WHERE id = ?
			   FOR UPDATE
		), upd AS (
			UPDATE webhook_subscriptions s
			   SET consecutive_failures = old.consecutive_failures + 1,
			       enabled = CASE WHEN old.consecutive_failures + 1 >= ? THEN false ELSE s.enabled END,
			       disabled_reason = CASE WHEN old.consecutive_failures + 1 >= ?
			            THEN 'Disabled automatically after ' || (old.consecutive_failures + 1) ||
			                 ' consecutive delivery failures. Fix the endpoint and re-enable.'
			            ELSE s.disabled_reason END,
			       disabled_at = CASE WHEN old.consecutive_failures + 1 >= ? THEN now() ELSE s.disabled_at END,
			       updated_at = now()
			  FROM old
			 WHERE s.id = ?
			RETURNING s.enabled AS is_enabled
		)
		SELECT old.was_enabled, upd.is_enabled FROM old, upd`,
		id, threshold, threshold, threshold, id).Scan(&result)
	if tx.Error != nil {
		return false, fmt.Errorf("webhook: record failure: %w", tx.Error)
	}
	if tx.RowsAffected == 0 {
		return false, nil
	}
	return result.WasEnabled && !result.IsEnabled, nil
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
