package domain

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/pkg/apperrors"
)

// Repository is the data-access surface for custom domains.
type Repository interface {
	// List returns all custom domains for a store.
	List(ctx context.Context, db *gorm.DB, storeID uuid.UUID) ([]CustomDomain, error)

	// GetByID returns a single custom domain by primary key, scoped to store.
	GetByID(ctx context.Context, db *gorm.DB, storeID, id uuid.UUID) (*CustomDomain, error)

	// Create inserts a new custom domain row.
	Create(ctx context.Context, db *gorm.DB, d *CustomDomain) error

	// Update saves all fields on a custom domain.
	Update(ctx context.Context, db *gorm.DB, d *CustomDomain) error

	// Delete removes a custom domain row.
	Delete(ctx context.Context, db *gorm.DB, storeID, id uuid.UUID) error

	// ListByStatus returns all domains with a given status (across all stores).
	ListByStatus(ctx context.Context, db *gorm.DB, status DomainStatus) ([]CustomDomain, error)
}

type gormRepository struct{}

// NewRepository constructs a stateless GORM-backed repository.
func NewRepository() Repository { return &gormRepository{} }

func (gormRepository) List(ctx context.Context, db *gorm.DB, storeID uuid.UUID) ([]CustomDomain, error) {
	var domains []CustomDomain
	if err := db.WithContext(ctx).Where("store_id = ?", storeID).Order("created_at DESC").Find(&domains).Error; err != nil {
		return nil, fmt.Errorf("domain list: %w", err)
	}
	return domains, nil
}

func (gormRepository) GetByID(ctx context.Context, db *gorm.DB, storeID, id uuid.UUID) (*CustomDomain, error) {
	var d CustomDomain
	if err := db.WithContext(ctx).Where("store_id = ? AND id = ?", storeID, id).First(&d).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, apperrors.NotFound("custom domain")
		}
		return nil, fmt.Errorf("domain get by id: %w", err)
	}
	return &d, nil
}

func (gormRepository) Create(ctx context.Context, db *gorm.DB, d *CustomDomain) error {
	if err := db.WithContext(ctx).Create(d).Error; err != nil {
		return fmt.Errorf("domain create: %w", err)
	}
	return nil
}

func (gormRepository) Update(ctx context.Context, db *gorm.DB, d *CustomDomain) error {
	if err := db.WithContext(ctx).Save(d).Error; err != nil {
		return fmt.Errorf("domain update: %w", err)
	}
	return nil
}

func (gormRepository) Delete(ctx context.Context, db *gorm.DB, storeID, id uuid.UUID) error {
	res := db.WithContext(ctx).Where("store_id = ? AND id = ?", storeID, id).Delete(&CustomDomain{})
	if res.Error != nil {
		return fmt.Errorf("domain delete: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return apperrors.NotFound("custom domain")
	}
	return nil
}

func (gormRepository) ListByStatus(ctx context.Context, db *gorm.DB, status DomainStatus) ([]CustomDomain, error) {
	var domains []CustomDomain
	if err := db.WithContext(ctx).Where("status = ?", status).Find(&domains).Error; err != nil {
		return nil, fmt.Errorf("domain list by status: %w", err)
	}
	return domains, nil
}
