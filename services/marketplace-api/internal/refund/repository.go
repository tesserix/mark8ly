package refund

import (
	"context"
	"fmt"

	"gorm.io/gorm"
)

// Repository is the data-access surface for refund_audit rows.
type Repository interface {
	// Create inserts a new refund audit row.
	Create(ctx context.Context, db *gorm.DB, r *RefundAudit) error

	// ExistsByCardFingerprint returns true when a refund_audit row already
	// exists for the given card fingerprint (§8 cross-subscription fraud guard).
	ExistsByCardFingerprint(ctx context.Context, db *gorm.DB, fingerprint string) (bool, error)

	// ExistsByStore returns true when any refund row exists for the given store.
	ExistsByStore(ctx context.Context, db *gorm.DB, storeID string) (bool, error)
}

type gormRepository struct{}

// NewRepository constructs a stateless GORM-backed repository.
func NewRepository() Repository { return &gormRepository{} }

func (gormRepository) Create(ctx context.Context, db *gorm.DB, r *RefundAudit) error {
	if err := db.WithContext(ctx).Create(r).Error; err != nil {
		return fmt.Errorf("refund: create audit: %w", err)
	}
	return nil
}

func (gormRepository) ExistsByCardFingerprint(ctx context.Context, db *gorm.DB, fingerprint string) (bool, error) {
	var count int64
	if err := db.WithContext(ctx).Model(&RefundAudit{}).
		Where("card_fingerprint = ?", fingerprint).
		Limit(1).
		Count(&count).Error; err != nil {
		return false, fmt.Errorf("refund: exists by card fingerprint: %w", err)
	}
	return count > 0, nil
}

func (gormRepository) ExistsByStore(ctx context.Context, db *gorm.DB, storeID string) (bool, error) {
	var count int64
	if err := db.WithContext(ctx).Model(&RefundAudit{}).
		Where("store_id = ?", storeID).
		Limit(1).
		Count(&count).Error; err != nil {
		return false, fmt.Errorf("refund: exists by store: %w", err)
	}
	return count > 0, nil
}
