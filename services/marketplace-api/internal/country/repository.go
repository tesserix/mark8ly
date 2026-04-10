package country

import (
	"context"

	"gorm.io/gorm"
)

// Repository defines data access for supported countries.
type Repository interface {
	ListActive(ctx context.Context) ([]SupportedCountry, error)
	GetByCode(ctx context.Context, code string) (*SupportedCountry, error)
}

type gormRepository struct{ db *gorm.DB }

// NewRepository returns a GORM-backed Repository.
func NewRepository(db *gorm.DB) Repository { return &gormRepository{db: db} }

func (r *gormRepository) ListActive(ctx context.Context) ([]SupportedCountry, error) {
	var rows []SupportedCountry
	err := r.db.WithContext(ctx).
		Where("is_active = true").
		Order("name").
		Find(&rows).Error
	return rows, err
}

func (r *gormRepository) GetByCode(ctx context.Context, code string) (*SupportedCountry, error) {
	var c SupportedCountry
	err := r.db.WithContext(ctx).
		Where("country_code = ? AND is_active = true", code).
		First(&c).Error
	if err != nil {
		return nil, err
	}
	return &c, nil
}
