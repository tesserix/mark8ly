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

// ClaimDue locks up to limit pending, due deliveries. FOR UPDATE SKIP LOCKED
// is what makes it safe for several replicas to run this loop at once — each
// takes a disjoint set instead of contending.
func (r *DeliveryRepo) ClaimDue(ctx context.Context, limit int) ([]Delivery, error) {
	var out []Delivery
	err := r.db.WithContext(ctx).Raw(`
		SELECT * FROM webhook_deliveries
		 WHERE status = ? AND next_attempt_at <= now()
		 ORDER BY next_attempt_at ASC
		 LIMIT ?
		 FOR UPDATE SKIP LOCKED`, StatusPending, limit).Scan(&out).Error
	if err != nil {
		return nil, fmt.Errorf("webhook: claim deliveries: %w", err)
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
