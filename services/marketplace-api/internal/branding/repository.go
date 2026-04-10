package branding

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/pkg/apperrors"
)

// Repository is the data-access surface for store branding.
type Repository interface {
	GetByStoreID(ctx context.Context, db *gorm.DB, storeID uuid.UUID) (*StoreBranding, error)
	Upsert(ctx context.Context, db *gorm.DB, b *StoreBranding) error
}

type gormRepository struct{}

// NewRepository constructs a stateless GORM-backed repository.
func NewRepository() Repository { return &gormRepository{} }

func (gormRepository) GetByStoreID(ctx context.Context, db *gorm.DB, storeID uuid.UUID) (*StoreBranding, error) {
	var b StoreBranding
	if err := db.WithContext(ctx).Where("store_id = ?", storeID).First(&b).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, apperrors.NotFound("branding")
		}
		return nil, fmt.Errorf("branding get by store: %w", err)
	}
	return &b, nil
}

func (gormRepository) Upsert(ctx context.Context, db *gorm.DB, b *StoreBranding) error {
	b.UpdatedAt = time.Now()

	result := db.WithContext(ctx).
		Where("store_id = ?", b.StoreID).
		First(&StoreBranding{})

	if result.Error == gorm.ErrRecordNotFound {
		b.CreatedAt = time.Now()
		if err := db.WithContext(ctx).Create(b).Error; err != nil {
			return fmt.Errorf("branding create: %w", err)
		}
		return nil
	}
	if result.Error != nil {
		return fmt.Errorf("branding upsert check: %w", result.Error)
	}

	if err := db.WithContext(ctx).
		Where("store_id = ?", b.StoreID).
		Updates(b).Error; err != nil {
		return fmt.Errorf("branding update: %w", err)
	}
	return nil
}
