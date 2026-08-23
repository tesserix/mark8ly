// Package estate serves platform-wide counts for the Tesserix console's
// product tile (#282). It reads across tenants and stores, which is why it
// is its own package rather than living in either.
package estate

import (
	"context"
	"fmt"

	"gorm.io/gorm"

	"github.com/mark8ly/platform-api/internal/store"
	"github.com/mark8ly/platform-api/internal/tenant"
)

// Counts is the estate-wide headline count set.
type Counts struct {
	TenantsActive int64 `json:"tenants_active"`
	StoresActive  int64 `json:"stores_active"`
}

// Repository is the data-access interface for estate-wide counts.
type Repository interface {
	Get(ctx context.Context) (*Counts, error)
}

type gormRepository struct {
	db *gorm.DB
}

// NewRepository constructs a Repository backed by the given *gorm.DB.
func NewRepository(db *gorm.DB) Repository {
	return &gormRepository{db: db}
}

// Get runs the two independent counts that make up the estate headline:
// active tenants and active stores. An empty estate is a real answer —
// zero counts with a nil error — not distinguished from any other count;
// only a query failure returns an error.
func (r *gormRepository) Get(ctx context.Context) (*Counts, error) {
	var c Counts
	if err := r.db.WithContext(ctx).Table("tenants").
		Where("status = ?", tenant.StatusActive).
		Count(&c.TenantsActive).Error; err != nil {
		return nil, fmt.Errorf("estate: count active tenants: %w", err)
	}
	if err := r.db.WithContext(ctx).Table("stores").
		Where("status = ?", store.StatusActive).
		Count(&c.StoresActive).Error; err != nil {
		return nil, fmt.Errorf("estate: count active stores: %w", err)
	}
	return &c, nil
}
