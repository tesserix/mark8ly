package tenant

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"

	apperrors "github.com/mark8ly/platform-api/pkg/errors"
)

// Repository is the data-access interface for tenants.
type Repository interface {
	// CreateInTx inserts a tenant inside an existing transaction.
	// The caller must commit the tx for the row to persist. Used by
	// onboarding completion which writes both the tenant and an outbox
	// row in one tx.
	CreateInTx(ctx context.Context, tx *gorm.DB, t *Tenant) error
	// GetByID returns a tenant by its UUID.
	GetByID(ctx context.Context, id string) (*Tenant, error)
	// GetBySlug returns a tenant by its public slug.
	GetBySlug(ctx context.Context, slug string) (*Tenant, error)
	// SlugExists checks slug uniqueness without loading the row.
	SlugExists(ctx context.Context, slug string) (bool, error)
}

// gormRepository is the GORM-backed implementation.
type gormRepository struct {
	db *gorm.DB
}

// NewRepository constructs a Repository.
func NewRepository(db *gorm.DB) Repository {
	return &gormRepository{db: db}
}

func (r *gormRepository) CreateInTx(ctx context.Context, tx *gorm.DB, t *Tenant) error {
	if err := tx.WithContext(ctx).Create(t).Error; err != nil {
		// Catch slug-uniqueness violations and translate them into a
		// caller-friendly conflict error.
		if isUniqueViolation(err) {
			return apperrors.Conflict("slug_taken", fmt.Sprintf("slug %q is already taken", t.Slug))
		}
		return fmt.Errorf("tenant: create: %w", err)
	}
	return nil
}

func (r *gormRepository) GetByID(ctx context.Context, id string) (*Tenant, error) {
	var t Tenant
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&t).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, apperrors.NotFound("tenant_not_found", fmt.Sprintf("tenant %q does not exist", id))
	}
	if err != nil {
		return nil, fmt.Errorf("tenant: get by id %q: %w", id, err)
	}
	return &t, nil
}

func (r *gormRepository) GetBySlug(ctx context.Context, slug string) (*Tenant, error) {
	var t Tenant
	err := r.db.WithContext(ctx).Where("slug = ?", slug).First(&t).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, apperrors.NotFound("tenant_not_found", fmt.Sprintf("tenant %q does not exist", slug))
	}
	if err != nil {
		return nil, fmt.Errorf("tenant: get by slug %q: %w", slug, err)
	}
	return &t, nil
}

func (r *gormRepository) SlugExists(ctx context.Context, slug string) (bool, error) {
	var count int64
	if err := r.db.WithContext(ctx).Model(&Tenant{}).Where("slug = ?", slug).Count(&count).Error; err != nil {
		return false, fmt.Errorf("tenant: slug exists %q: %w", slug, err)
	}
	return count > 0, nil
}

// isUniqueViolation returns true for Postgres unique constraint violations.
// We rely on pgx's typed error so we don't string-match on error messages.
func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "23505" // unique_violation
	}
	return false
}
