package webhook

import (
	"context"
	"fmt"

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
