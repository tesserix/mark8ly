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
	// GetByOwnerUserID returns the tenant whose owner is the given GIP UID.
	// Used by returning-user sign-in to map an authenticated GIP identity
	// back to its Mark8ly tenant before calling auth-bff /auth/auto-login
	// (which requires workspace_tenant). v1 assumption: a UID owns at most
	// one tenant.
	GetByOwnerUserID(ctx context.Context, uid string) (*Tenant, error)
	// UpdateEditable applies a patch to the editable subset of a tenant row
	// and returns the updated row. Only fields present in the patch map are
	// written, so callers can PATCH a single field without clobbering the
	// rest. The caller is responsible for whitelisting which columns are
	// allowed — the repo trusts its input.
	UpdateEditable(ctx context.Context, id string, patch map[string]any) (*Tenant, error)
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
