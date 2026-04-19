package campaignbudget

import (
	"context"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Service bundles the campaign-budget operations that callers outside this
// package need. Construct once in main and inject.
type Service struct {
	db *gorm.DB
}

// NewService constructs a Service backed by db.
func NewService(db *gorm.DB) *Service { return &Service{db: db} }

// Reserve is the hot-path pre-send decrement (spec §10.1).
// Returns remaining budget after decrement on success.
func (s *Service) Reserve(ctx context.Context, storeID uuid.UUID, recipientCount int) (int, error) {
	return Reserve(ctx, s.db, storeID, recipientCount)
}

// RecomputeLimitForPlan is invoked by P4 inside its change-plan transaction.
// Pass the caller's *gorm.DB (mid-tx or not) via tx.
// If tx is nil, falls back to the service's own db.
func (s *Service) RecomputeLimitForPlan(ctx context.Context, tx *gorm.DB, storeID uuid.UUID, plan string) error {
	if tx == nil {
		tx = s.db
	}
	return RecomputeLimitForPlan(ctx, tx, storeID, plan)
}
