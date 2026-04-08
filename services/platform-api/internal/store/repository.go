package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"

	apperrors "github.com/mark8ly/platform-api/pkg/errors"
)

// Repository is the data-access interface for stores.
type Repository interface {
	// CreateInTx inserts a store inside an existing transaction. The
	// caller must commit the tx for the row to persist. Used by
	// onboarding completion which writes the tenant + default store
	// in the same transaction as the FGA outbox event.
	CreateInTx(ctx context.Context, tx *gorm.DB, s *Store) error
	// GetByID returns a store by its UUID.
	GetByID(ctx context.Context, id string) (*Store, error)
	// GetBySlug returns a store by its globally unique slug.
	GetBySlug(ctx context.Context, slug string) (*Store, error)
	// ListByTenant returns every store under the given tenant,
	// ordered by created_at ascending (the first store is always
	// the default created during onboarding).
	ListByTenant(ctx context.Context, tenantID string) ([]Store, error)
	// SlugExists checks uniqueness without loading the row.
	SlugExists(ctx context.Context, slug string) (bool, error)
	// UpdateEditable applies a patch to the editable subset and
	// returns the updated row. Same contract as tenant.UpdateEditable
	// pre-Phase-Q; store is now where name/currency/timezone live.
	UpdateEditable(ctx context.Context, id string, patch map[string]any) (*Store, error)
}

type gormRepository struct {
	db *gorm.DB
}

func NewRepository(db *gorm.DB) Repository {
	return &gormRepository{db: db}
}

func (r *gormRepository) CreateInTx(ctx context.Context, tx *gorm.DB, s *Store) error {
	if err := tx.WithContext(ctx).Create(s).Error; err != nil {
		if isUniqueViolation(err) {
			return apperrors.Conflict(
				"slug_taken",
				fmt.Sprintf("slug %q is already taken", s.Slug),
			)
		}
		return fmt.Errorf("store: create: %w", err)
	}
	return nil
}

func (r *gormRepository) GetByID(ctx context.Context, id string) (*Store, error) {
	var s Store
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&s).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, apperrors.NotFound("store_not_found", fmt.Sprintf("store %q does not exist", id))
	}
	if err != nil {
		return nil, fmt.Errorf("store: get by id %q: %w", id, err)
	}
	return &s, nil
}

func (r *gormRepository) GetBySlug(ctx context.Context, slug string) (*Store, error) {
	var s Store
	err := r.db.WithContext(ctx).Where("slug = ?", slug).First(&s).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, apperrors.NotFound("store_not_found", fmt.Sprintf("store %q does not exist", slug))
	}
	if err != nil {
		return nil, fmt.Errorf("store: get by slug %q: %w", slug, err)
	}
	return &s, nil
}

func (r *gormRepository) ListByTenant(ctx context.Context, tenantID string) ([]Store, error) {
	var rows []Store
	err := r.db.WithContext(ctx).
		Where("tenant_id = ?", tenantID).
		Order("created_at ASC").
		Find(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("store: list by tenant %q: %w", tenantID, err)
	}
	return rows, nil
}

func (r *gormRepository) SlugExists(ctx context.Context, slug string) (bool, error) {
	var count int64
	if err := r.db.WithContext(ctx).Model(&Store{}).Where("slug = ?", slug).Count(&count).Error; err != nil {
		return false, fmt.Errorf("store: slug exists %q: %w", slug, err)
	}
	return count > 0, nil
}

func (r *gormRepository) UpdateEditable(ctx context.Context, id string, patch map[string]any) (*Store, error) {
	if len(patch) == 0 {
		return r.GetByID(ctx, id)
	}
	patch["updated_at"] = gorm.Expr("NOW()")

	res := r.db.WithContext(ctx).Model(&Store{}).Where("id = ?", id).Updates(patch)
	if err := res.Error; err != nil {
		if isUniqueViolation(err) {
			return nil, apperrors.Conflict("store_update_conflict", "store update violates a unique constraint")
		}
		return nil, fmt.Errorf("store: update %q: %w", id, err)
	}
	if res.RowsAffected == 0 {
		return nil, apperrors.NotFound("store_not_found", fmt.Sprintf("store %q does not exist", id))
	}
	return r.GetByID(ctx, id)
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "23505"
	}
	return false
}
