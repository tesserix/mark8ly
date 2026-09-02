package webhook

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type DeliveryRepo struct{ db *gorm.DB }

func NewDeliveryRepo(db *gorm.DB) *DeliveryRepo { return &DeliveryRepo{db: db} }

// FanOut inserts delivery rows, ignoring any that already exist.
//
// ON CONFLICT DO NOTHING against idx_webhook_deliveries_event_sub is what
// makes dispatch idempotent, and therefore what lets the dispatcher run
// OUTSIDE the outbox publisher's transaction without risking duplicate
// deliveries. Re-reading the same outbox rows is harmless.
func (r *DeliveryRepo) FanOut(ctx context.Context, rows []Delivery) (int, error) {
	if len(rows) == 0 {
		return 0, nil
	}
	res := r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "outbox_event_id"}, {Name: "subscription_id"}},
		DoNothing: true,
	}).Create(&rows)
	if res.Error != nil {
		return 0, fmt.Errorf("webhook: fan out deliveries: %w", res.Error)
	}
	return int(res.RowsAffected), nil
}

// ClaimDue claims up to limit pending, due deliveries by taking a short
// LEASE, not a lock.
//
// FOR UPDATE SKIP LOCKED only holds its row lock while THIS function's own
// transaction is open, and that transaction commits here, before the caller
// ever makes the outbound HTTP call. Postgres releases the lock at commit —
// well before RecordOutcome moves status off pending, up to RequestTimeout
// later. If ClaimDue simply returned the claimed rows at that point, a
// second worker calling ClaimDue while the first is still mid-Send would
// see them as pending and unlocked, and send them again. Several replicas
// running this loop at once (Task 6 puts this on KEDA-scaled pods) is
// exactly the case that must not double-send.
//
// So within the same transaction that claims the rows, it immediately
// pushes their next_attempt_at forward by LeaseWindow. That's what actually
// keeps the rows from being claimed again: they are still status=pending,
// but no longer due. The HTTP send itself happens entirely OUTSIDE any
// transaction — holding a connection and row locks across a blocking
// outbound call to a merchant server is not an option on a 5-connection
// pool shared with the rest of the service. RecordOutcome then overwrites
// next_attempt_at with the real outcome (retry backoff, or dead-letter).
//
// If a worker dies mid-send, no RecordOutcome ever runs and the lease
// simply expires — the row becomes due again and some worker retries it.
// That's at-least-once delivery, which is what webhooks already assume:
// the signature and delivery id are what let a merchant dedupe.
func (r *DeliveryRepo) ClaimDue(ctx context.Context, limit int) ([]Delivery, error) {
	var out []Delivery
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Raw(`
			SELECT * FROM webhook_deliveries
			 WHERE status = ? AND next_attempt_at <= now()
			 ORDER BY next_attempt_at ASC
			 LIMIT ?
			 FOR UPDATE SKIP LOCKED`, StatusPending, limit).Scan(&out).Error; err != nil {
			return fmt.Errorf("webhook: claim deliveries: %w", err)
		}
		if len(out) == 0 {
			return nil
		}

		ids := make([]uuid.UUID, len(out))
		for i, d := range out {
			ids[i] = d.ID
		}
		leaseUntil := time.Now().Add(LeaseWindow)
		if err := tx.Exec(`UPDATE webhook_deliveries SET next_attempt_at = ? WHERE id IN ?`,
			leaseUntil, ids).Error; err != nil {
			return fmt.Errorf("webhook: lease deliveries: %w", err)
		}
		// Reflect the lease in what's returned too, so a caller reading
		// NextAttemptAt off these structs doesn't see stale pre-lease data.
		for i := range out {
			out[i].NextAttemptAt = leaseUntil
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// RecordOutcome writes the result of one attempt.
func (r *DeliveryRepo) RecordOutcome(ctx context.Context, id uuid.UUID, status string, code *int, errMsg *string, next time.Time) error {
	updates := map[string]any{
		"status":           status,
		"attempts":         gorm.Expr("attempts + 1"),
		"last_status_code": code,
		"last_error":       errMsg,
		"next_attempt_at":  next,
	}
	if status == StatusDelivered {
		updates["delivered_at"] = time.Now()
	}
	if err := r.db.WithContext(ctx).Model(&Delivery{}).Where("id = ?", id).Updates(updates).Error; err != nil {
		return fmt.Errorf("webhook: record outcome: %w", err)
	}
	return nil
}

// ListForSubscription returns the most recent deliveries for one
// subscription, most recent first, for the admin delivery log.
func (r *DeliveryRepo) ListForSubscription(ctx context.Context, subscriptionID uuid.UUID, limit int) ([]Delivery, error) {
	var out []Delivery
	err := r.db.WithContext(ctx).
		Where("subscription_id = ?", subscriptionID).
		Order("created_at DESC").
		Limit(limit).
		Find(&out).Error
	if err != nil {
		return nil, fmt.Errorf("webhook: list deliveries: %w", err)
	}
	return out, nil
}

// Replay resets one delivery to pending, due now, so the worker's next poll
// picks it up. Scoped to subscriptionID — the caller must have already
// verified that subscription belongs to their tenant and store — so a
// deliveryID belonging to a different subscription is silently a no-op
// rather than a cross-tenant write. Reports whether a row matched.
func (r *DeliveryRepo) Replay(ctx context.Context, subscriptionID, deliveryID uuid.UUID) (bool, error) {
	res := r.db.WithContext(ctx).Exec(`
		UPDATE webhook_deliveries
		   SET status = ?, attempts = 0, next_attempt_at = now()
		 WHERE id = ? AND subscription_id = ?`,
		StatusPending, deliveryID, subscriptionID)
	if res.Error != nil {
		return false, fmt.Errorf("webhook: replay delivery: %w", res.Error)
	}
	return res.RowsAffected > 0, nil
}

// Prune deletes delivery rows older than the retention window. 30 days on
// every plan, deliberately not tied to FeatureAuditRetentionDays: "forever"
// retention of delivery bodies on Pro is storage cost on a db-f1-micro with
// no matching merchant value.
func (r *DeliveryRepo) Prune(ctx context.Context, olderThan time.Duration) (int64, error) {
	res := r.db.WithContext(ctx).Exec(
		`DELETE FROM webhook_deliveries WHERE created_at < now() - ?::interval`,
		fmt.Sprintf("%d hours", int(olderThan.Hours())))
	if res.Error != nil {
		return 0, fmt.Errorf("webhook: prune deliveries: %w", res.Error)
	}
	return res.RowsAffected, nil
}
