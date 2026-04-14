package page

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"

	apperrors "github.com/mark8ly/marketplace-api/pkg/apperrors"
)

// Repository is the GORM-backed data access for pages.
type Repository struct {
	db *gorm.DB
}

func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

// Create inserts a new page row. ID, CreatedAt, UpdatedAt are filled by
// DB defaults. Violates the pages_store_slug_idx unique index if the
// (store_id, slug) pair already exists.
func (r *Repository) Create(ctx context.Context, p *Page) error {
	if err := r.db.WithContext(ctx).Create(p).Error; err != nil {
		return translatePageUniqueViolation(err, p.Slug)
	}
	return nil
}

// GetByID returns the page with that id or nil if not found.
func (r *Repository) GetByID(ctx context.Context, id uuid.UUID) (*Page, error) {
	var p Page
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&p).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// GetBySlug returns the page for a given store + slug. publishedOnly
// filters unpublished pages from the storefront read path; admin calls
// with publishedOnly=false to preview drafts.
func (r *Repository) GetBySlug(ctx context.Context, storeID uuid.UUID, slug string, publishedOnly bool) (*Page, error) {
	var p Page
	q := r.db.WithContext(ctx).Where("store_id = ? AND slug = ?", storeID, slug)
	if publishedOnly {
		q = q.Where("published = ?", true)
	}
	err := q.First(&p).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// ListByStore returns the pages for a store in sort order. publishedOnly
// filters drafts for storefront callers.
func (r *Repository) ListByStore(ctx context.Context, storeID uuid.UUID, publishedOnly bool) ([]Page, error) {
	var pages []Page
	q := r.db.WithContext(ctx).Where("store_id = ?", storeID)
	if publishedOnly {
		q = q.Where("published = ?", true)
	}
	if err := q.Order("sort_order ASC, title ASC").Find(&pages).Error; err != nil {
		return nil, err
	}
	return pages, nil
}

// Update applies a patch map to the page. Uses GORM's map-based Updates
// so only supplied keys are written — unset fields are preserved.
func (r *Repository) Update(ctx context.Context, id uuid.UUID, patch map[string]any) error {
	attempted := ""
	if v, ok := patch["slug"].(string); ok {
		attempted = v
	}
	if err := r.db.WithContext(ctx).Model(&Page{}).Where("id = ?", id).Updates(patch).Error; err != nil {
		return translatePageUniqueViolation(err, attempted)
	}
	return nil
}

// Delete removes the page row. Safe to call with a non-existent id;
// GORM returns nil with zero rows affected.
func (r *Repository) Delete(ctx context.Context, id uuid.UUID) error {
	return r.db.WithContext(ctx).Where("id = ?", id).Delete(&Page{}).Error
}

// translatePageUniqueViolation maps PG unique violations on the
// pages_store_slug_idx index to apperrors.SlugTaken. Any other
// violation or non-23505 error is returned wrapped for debuggability.
func translatePageUniqueViolation(err error, attemptedSlug string) error {
	if err == nil {
		return nil
	}
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "23505" {
		return err
	}
	if pgErr.ConstraintName == "pages_store_slug_idx" {
		return apperrors.SlugTaken(attemptedSlug, attemptedSlug+"-2")
	}
	return fmt.Errorf("page: unique violation on %s: %w", pgErr.ConstraintName, err)
}
