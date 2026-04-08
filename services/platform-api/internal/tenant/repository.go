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
//
// Phase Q: GetBySlug and SlugExists moved to the store package —
// slug is a store-level identifier now.
type Repository interface {
	CreateInTx(ctx context.Context, tx *gorm.DB, t *Tenant) error
	GetByID(ctx context.Context, id string) (*Tenant, error)
	// GetByOwnerUserID returns the tenant whose owner is the given
	// GIP UID. Used by returning-user sign-in to map an authenticated
	// GIP identity back to a Mark8ly tenant before calling auth-bff
	// /auth/auto-login. v1 assumption: a UID owns at most one tenant.
	GetByOwnerUserID(ctx context.Context, uid string) (*Tenant, error)
	UpdateEditable(ctx context.Context, id string, patch map[string]any) (*Tenant, error)
	// ListByIDs returns tenant rows for each id in the given slice.
	// Used by Phase P multi-tenant membership lookups.
	ListByIDs(ctx context.Context, ids []string) ([]Tenant, error)
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
		if isUniqueViolation(err) {
			return apperrors.Conflict("tenant_conflict", "tenant row violates a unique constraint")
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

func (r *gormRepository) GetByOwnerUserID(ctx context.Context, uid string) (*Tenant, error) {
	var t Tenant
	err := r.db.WithContext(ctx).Where("owner_user_id = ?", uid).First(&t).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, apperrors.NotFound("tenant_not_found", fmt.Sprintf("no tenant owned by uid %q", uid))
	}
	if err != nil {
		return nil, fmt.Errorf("tenant: get by owner uid %q: %w", uid, err)
	}
	return &t, nil
}

func (r *gormRepository) ListByIDs(ctx context.Context, ids []string) ([]Tenant, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	var rows []Tenant
	if err := r.db.WithContext(ctx).Where("id IN ?", ids).Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("tenant: list by ids: %w", err)
	}
	return rows, nil
}

func (r *gormRepository) UpdateEditable(ctx context.Context, id string, patch map[string]any) (*Tenant, error) {
	if len(patch) == 0 {
		// Nothing to write; fetch and return current state so callers get
		// a no-op semantics instead of an empty SQL UPDATE.
		return r.GetByID(ctx, id)
	}
	// Always bump updated_at so callers see a fresh timestamp.
	patch["updated_at"] = gorm.Expr("NOW()")

	res := r.db.WithContext(ctx).Model(&Tenant{}).Where("id = ?", id).Updates(patch)
	if err := res.Error; err != nil {
		if isUniqueViolation(err) {
			return nil, apperrors.Conflict("tenant_update_conflict", "tenant update violates a unique constraint")
		}
		return nil, fmt.Errorf("tenant: update %q: %w", id, err)
	}
	if res.RowsAffected == 0 {
		return nil, apperrors.NotFound("tenant_not_found", fmt.Sprintf("tenant %q does not exist", id))
	}
	return r.GetByID(ctx, id)
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
