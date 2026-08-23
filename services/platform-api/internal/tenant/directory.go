package tenant

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"

	apperrors "github.com/mark8ly/platform-api/pkg/errors"
)

// DefaultDirectoryPageSize applies when the caller sends no limit. The
// contract says a missing parameter takes our default and is never an error.
const DefaultDirectoryPageSize = 50

// MaxDirectoryPageSize caps a page. Mirrors the ceiling the marketplace-api
// side applies, so a caller cannot force an unbounded scan here either.
const MaxDirectoryPageSize = 500

// DirectoryFilter narrows the platform-wide tenant directory. Every field is
// optional — this is the platform operator's view, not a caller-scoped one.
type DirectoryFilter struct {
	// Q matches tenants.name, tenants.owner_email, and any of the tenant's
	// stores.slug. Tenants have no slug of their own (see models.go): the
	// URL identity moved to store.slug, and a tenant with multiple stores
	// has multiple slugs.
	Q           string
	Status      string
	CreatedFrom time.Time
	CreatedTo   time.Time
	Page        int
	Limit       int
}

// StoreSummary is the per-store rollup entry on a tenant detail.
type StoreSummary struct {
	ID     string `json:"id"`
	Slug   string `json:"slug"`
	Name   string `json:"name"`
	Status string `json:"status"`
}

// TenantWithStores is a tenant plus its store rollup.
type TenantWithStores struct {
	Tenant
	Stores []StoreSummary `json:"stores"`
}

// DirectoryResult is a page of tenants plus the unpaginated total.
type DirectoryResult struct {
	Tenants []Tenant
	Total   int64
}

// applyDirectoryFilter builds the WHERE clause shared by the count and the
// page query, so the two can never drift apart.
func applyDirectoryFilter(q *gorm.DB, f DirectoryFilter) *gorm.DB {
	if f.Q != "" {
		like := "%" + f.Q + "%"
		// EXISTS rather than a JOIN: a tenant with two matching stores must
		// appear once, and EXISTS says that directly instead of relying on a
		// DISTINCT that also has to be threaded through the count.
		q = q.Where(
			`tenants.name ILIKE ? OR tenants.owner_email ILIKE ?
			 OR EXISTS (SELECT 1 FROM stores s WHERE s.tenant_id = tenants.id AND s.slug ILIKE ?)`,
			like, like, like,
		)
	}
	if f.Status != "" {
		q = q.Where("tenants.status = ?", f.Status)
	}
	if !f.CreatedFrom.IsZero() {
		q = q.Where("tenants.created_at >= ?", f.CreatedFrom)
	}
	if !f.CreatedTo.IsZero() {
		q = q.Where("tenants.created_at <= ?", f.CreatedTo)
	}
	return q
}

func (r *gormRepository) ListDirectory(ctx context.Context, f DirectoryFilter) (DirectoryResult, error) {
	var result DirectoryResult

	countQ := applyDirectoryFilter(r.db.WithContext(ctx).Model(&Tenant{}), f)
	if err := countQ.Count(&result.Total).Error; err != nil {
		return result, fmt.Errorf("tenant directory count: %w", err)
	}

	page := max(f.Page, 1)
	limit := f.Limit
	switch {
	case limit <= 0:
		limit = DefaultDirectoryPageSize
	case limit > MaxDirectoryPageSize:
		limit = MaxDirectoryPageSize
	}

	// Allocate before Find: a nil slice marshals to {} downstream, which
	// defeats a caller's `?? []`.
	result.Tenants = make([]Tenant, 0, limit)

	pageQ := applyDirectoryFilter(r.db.WithContext(ctx).Model(&Tenant{}), f)
	if err := pageQ.
		Order("tenants.created_at DESC").
		Offset((page - 1) * limit).
		Limit(limit).
		Find(&result.Tenants).Error; err != nil {
		return result, fmt.Errorf("tenant directory list: %w", err)
	}
	return result, nil
}

func (r *gormRepository) GetWithStores(ctx context.Context, id string) (*TenantWithStores, error) {
	t, err := r.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	out := &TenantWithStores{Tenant: *t, Stores: make([]StoreSummary, 0)}

	// One query for every store, not one per store — #277 asks for the
	// detail "without a round trip per store".
	if err := r.db.WithContext(ctx).
		Table("stores").
		Select("id, slug, name, status").
		Where("tenant_id = ?", id).
		Order("slug ASC").
		Scan(&out.Stores).Error; err != nil {
		return nil, fmt.Errorf("tenant store rollup: %w", err)
	}
	return out, nil
}

// GetByOwnerEmail returns the tenant owned by the given email (#279).
func (r *gormRepository) GetByOwnerEmail(ctx context.Context, email string) (*Tenant, error) {
	// Same normalisation as OwnerEmailExists: the unique index is on
	// lower(owner_email), so anything else silently misses it.
	normalized := strings.ToLower(strings.TrimSpace(email))
	if normalized == "" {
		return nil, apperrors.NotFound("tenant_not_found", "no tenant owns that email")
	}

	var t Tenant
	err := r.db.WithContext(ctx).
		Where("lower(owner_email) = ?", normalized).
		First(&t).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, apperrors.NotFound("tenant_not_found", "no tenant owns that email")
	}
	if err != nil {
		return nil, fmt.Errorf("tenant: get by owner email: %w", err)
	}
	return &t, nil
}
