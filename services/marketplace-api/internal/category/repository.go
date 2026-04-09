package category

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/pkg/apperrors"
)

// Repository is the data-access interface for the category tree.
type Repository interface {
	Create(ctx context.Context, c *Category) error
	GetByIDForStore(ctx context.Context, id, storeID, tenantID string) (*Category, error)
	ListByStore(ctx context.Context, storeID, tenantID string) ([]Category, error)
	UpdateInTx(ctx context.Context, tx *gorm.DB, id, storeID, tenantID string, fields map[string]any) error
	SoftDeleteInTx(ctx context.Context, tx *gorm.DB, id, storeID, tenantID string) error
	HasChildren(ctx context.Context, parentID string) (int64, error)
	HasProducts(ctx context.Context, categoryID string) (int64, error)
}

type gormRepository struct {
	db *gorm.DB
}

// NewRepository constructs a Repository.
func NewRepository(db *gorm.DB) Repository { return &gormRepository{db: db} }

func (r *gormRepository) Create(ctx context.Context, c *Category) error {
	err := r.db.WithContext(ctx).Create(c).Error
	if isUniqueSlug(err) {
		return apperrors.SlugTaken(c.Slug, c.Slug+"-2")
	}
	if err != nil {
		return fmt.Errorf("category: create: %w", err)
	}
	return nil
}

func (r *gormRepository) GetByIDForStore(ctx context.Context, id, storeID, tenantID string) (*Category, error) {
	var c Category
	err := r.db.WithContext(ctx).
		Where("id = ? AND store_id = ? AND tenant_id = ? AND deleted_at IS NULL", id, storeID, tenantID).
		First(&c).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, apperrors.NotFound("category")
	}
	if err != nil {
		return nil, fmt.Errorf("category: get by id: %w", err)
	}
	return &c, nil
}

func (r *gormRepository) ListByStore(ctx context.Context, storeID, tenantID string) ([]Category, error) {
	var cats []Category
	if err := r.db.WithContext(ctx).
		Where("store_id = ? AND tenant_id = ? AND deleted_at IS NULL", storeID, tenantID).
		Order("position ASC, name ASC").
		Find(&cats).Error; err != nil {
		return nil, fmt.Errorf("category: list by store: %w", err)
	}
	return cats, nil
}

func (r *gormRepository) UpdateInTx(ctx context.Context, tx *gorm.DB, id, storeID, tenantID string, fields map[string]any) error {
	res := tx.WithContext(ctx).
		Model(&Category{}).
		Where("id = ? AND store_id = ? AND tenant_id = ? AND deleted_at IS NULL", id, storeID, tenantID).
		Updates(fields)
	if isUniqueSlug(res.Error) {
		slug, _ := fields["slug"].(string)
		return apperrors.SlugTaken(slug, slug+"-2")
	}
	if res.Error != nil {
		return fmt.Errorf("category: update: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return apperrors.NotFound("category")
	}
	return nil
}

func (r *gormRepository) SoftDeleteInTx(ctx context.Context, tx *gorm.DB, id, storeID, tenantID string) error {
	res := tx.WithContext(ctx).
		Model(&Category{}).
		Where("id = ? AND store_id = ? AND tenant_id = ? AND deleted_at IS NULL", id, storeID, tenantID).
		Update("deleted_at", gorm.Expr("now()"))
	if res.Error != nil {
		return fmt.Errorf("category: soft delete: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return apperrors.NotFound("category")
	}
	return nil
}

func (r *gormRepository) HasChildren(ctx context.Context, parentID string) (int64, error) {
	var n int64
	if err := r.db.WithContext(ctx).
		Model(&Category{}).
		Where("parent_id = ? AND deleted_at IS NULL", parentID).
		Count(&n).Error; err != nil {
		return 0, fmt.Errorf("category: has children: %w", err)
	}
	return n, nil
}

func (r *gormRepository) HasProducts(ctx context.Context, categoryID string) (int64, error) {
	var n int64
	err := r.db.WithContext(ctx).
		Raw(`
			SELECT count(*)
			FROM product_categories pc
			JOIN products p ON p.id = pc.product_id
			WHERE pc.category_id = ? AND p.deleted_at IS NULL`, categoryID).
		Scan(&n).Error
	if err != nil {
		return 0, fmt.Errorf("category: has products: %w", err)
	}
	return n, nil
}

func isUniqueSlug(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505" &&
		pgErr.ConstraintName == "categories_slug_per_store_live_unique"
}
