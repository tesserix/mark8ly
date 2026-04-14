package vendor

import (
	"context"
	"errors"

	"gorm.io/gorm"
)

// Repository is the GORM-backed data access for vendors.
type Repository struct {
	db *gorm.DB
}

func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

// Create inserts a new vendor row. The caller must set TenantID, Name,
// Slug, IsSelf. ID, CreatedAt, UpdatedAt are filled by the DB defaults.
// Violates the `vendors_tenant_self_idx` partial unique index if a
// self-vendor already exists for the tenant.
func (r *Repository) Create(ctx context.Context, v *Vendor) error {
	return r.db.WithContext(ctx).Create(v).Error
}

// GetByID returns the vendor with that id or nil if not found.
func (r *Repository) GetByID(ctx context.Context, id string) (*Vendor, error) {
	var v Vendor
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&v).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &v, nil
}

// GetSelfByTenantID returns the tenant's self-vendor or nil if none
// exists yet (backfill not run, or tenant has zero products today).
func (r *Repository) GetSelfByTenantID(ctx context.Context, tenantID string) (*Vendor, error) {
	var v Vendor
	err := r.db.WithContext(ctx).
		Where("tenant_id = ? AND is_self = ?", tenantID, true).
		First(&v).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &v, nil
}

// UpdateNameAndSlug overwrites the display name and slug of an existing
// vendor. Used by the backfill CLI to replace the migration's placeholder
// values with real tenant identity.
func (r *Repository) UpdateNameAndSlug(ctx context.Context, id, name, slug string) error {
	return r.db.WithContext(ctx).Model(&Vendor{}).
		Where("id = ?", id).
		Updates(map[string]any{"name": name, "slug": slug}).Error
}
