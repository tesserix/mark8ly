// services/marketplace-api/internal/stores/repository.go
package stores

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Repository is the data-access interface for the local stores projection.
// Writes happen only through StoreMiddleware's lazy pull-through (Upsert).
// Reads are scoped by (id, tenant_id) — see ErrNotFound semantics.
type Repository interface {
	GetByIDForTenant(ctx context.Context, storeID, tenantID string) (*Store, error)
	Upsert(ctx context.Context, s *Store) error
}

type gormRepository struct {
	db *gorm.DB
}

// NewRepository constructs a Repository.
func NewRepository(db *gorm.DB) Repository { return &gormRepository{db: db} }

func (r *gormRepository) GetByIDForTenant(ctx context.Context, storeID, tenantID string) (*Store, error) {
	var s Store
	err := r.db.WithContext(ctx).
		Where("id = ? AND tenant_id = ?", storeID, tenantID).
		First(&s).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("stores: get by id for tenant: %w", err)
	}
	return &s, nil
}

// Upsert writes the row keyed on primary key id. On conflict it replaces
// every non-pk column and bumps synced_at. Caller is responsible for
// setting SyncedAt to time.Now() before calling.
func (r *gormRepository) Upsert(ctx context.Context, s *Store) error {
	if err := r.db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "id"}},
			DoUpdates: clause.AssignmentColumns([]string{
				"tenant_id", "slug", "name", "country_code",
				"currency_code", "timezone", "status", "synced_at",
			}),
		}).
		Create(s).Error; err != nil {
		return fmt.Errorf("stores: upsert: %w", err)
	}
	return nil
}
