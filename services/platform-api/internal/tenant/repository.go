package tenant

import (
	"context"
	"errors"
	"fmt"
	"strings"

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
	// OwnerEmailExists reports whether any tenant is already owned by the
	// given email. Comparison is case-insensitive to match the
	// tenants_owner_email_unique index (migration 0014), so
	// Founder@example.com and founder@example.com collide.
	//
	// An admin email is globally unique across tenants: one email owns at
	// most one tenant. Callers use this to reject a duplicate at the START
	// of onboarding rather than letting it fail at the final insert.
	OwnerEmailExists(ctx context.Context, email string) (bool, error)
	UpdateEditable(ctx context.Context, id string, patch map[string]any) (*Tenant, error)
	// ListByIDs returns tenant rows for each id in the given slice.
	// Used by Phase P multi-tenant membership lookups.
	ListByIDs(ctx context.Context, ids []string) ([]Tenant, error)
	// ListStoreIDs returns the IDs of every store under the given tenant.
	// Used by account-deletion teardown to remove FGA store-parent tuples
	// BEFORE the DB cascade (stores ON DELETE CASCADE from tenants) removes
	// the rows out from under it. If tx is nil, the repository's own db is
	// used (read-only lookups don't require a transaction).
	ListStoreIDs(ctx context.Context, tx *gorm.DB, tenantID string) ([]string, error)
	// SnapshotForTeardown reads the identifying state an operator purge
	// confirms against, with SELECT ... FOR UPDATE on the tenant row.
	//
	// It exists rather than composing GetByID + ListStoreIDs because both
	// of those read OUTSIDE the caller's transaction: reading in-tx narrows
	// (does not close) the window for a store created concurrently to be
	// missed by the confirmation check. Exactly one concurrent purge
	// succeeding is enforced separately, by DeleteInTx's row-count check
	// on DELETE — not demonstrated to depend on this FOR UPDATE, which is
	// kept for the narrower snapshot window and as defence in depth.
	//
	// Returns apperrors.NotFound("tenant_not_found") when the tenant does
	// not exist — including when a concurrent purge just removed it.
	SnapshotForTeardown(ctx context.Context, tx *gorm.DB, tenantID string) (*TeardownSnapshot, error)
	// DeleteInTx deletes a tenant and everything that must be reconciled
	// first. onboarding_sessions.tenant_id is ON DELETE SET NULL, but the
	// onboarding_sessions_completed_consistency CHECK requires tenant_id to
	// stay NOT NULL while status='completed' — so deleting the tenant
	// without reconciling first would null that column and violate the
	// CHECK, failing the tenant DELETE. DeleteInTx removes the tenant's
	// onboarding_sessions rows first (their verification codes cascade via
	// ON DELETE CASCADE), then deletes the tenant row. stores and
	// invitations FK to tenants ON DELETE CASCADE, so they clean up
	// automatically and are not touched here. Returns apperrors.NotFound if
	// the tenant does not exist. Must run inside the caller's transaction.
	DeleteInTx(ctx context.Context, tx *gorm.DB, tenantID string) error
	// ListDirectory returns a page of the platform-wide tenant directory.
	// NOT caller-scoped — this is the platform operator's view. See #277.
	ListDirectory(ctx context.Context, f DirectoryFilter) (DirectoryResult, error)
	// GetWithStores returns a tenant plus its store rollup in one query.
	GetWithStores(ctx context.Context, id string) (*TenantWithStores, error)
	// GetByOwnerEmail returns the tenant owned by the given email, or
	// apperrors.NotFound when no tenant is. Comparison is case-insensitive
	// and trims surrounding whitespace, matching OwnerEmailExists and the
	// tenants_owner_email_unique index (migration 0014) — which is also why
	// this returns at most one row rather than a slice.
	GetByOwnerEmail(ctx context.Context, email string) (*Tenant, error)
	// Suspend transitions a tenant from active to suspended and cascades
	// the suspension to its currently-ACTIVE stores, marking each one
	// suspended_by_tenant=true so Unsuspend can reverse ONLY what this
	// cascade did (see Unsuspend). A store that is already suspended
	// (suspended individually, outside this cascade) is left untouched —
	// it is not "affected" and its suspended_by_tenant flag is not set.
	// A no-op (tenant already suspended) returns Changed=false. Returns
	// apperrors.NotFound if the tenant does not exist, or
	// apperrors.Conflict if the tenant is in a status this cannot
	// transition from (e.g. archived).
	Suspend(ctx context.Context, tenantID string) (*SuspendResult, error)
	// Unsuspend transitions a tenant from suspended back to active and
	// restores ONLY the stores this package's Suspend cascade suspended
	// (suspended_by_tenant=true), clearing the flag on each. A store
	// suspended individually BEFORE (or during) the tenant suspension
	// stays suspended — reversibility is scoped to what the cascade
	// changed, not to every suspended store under the tenant. A no-op
	// (tenant already active) returns Changed=false. Returns
	// apperrors.NotFound if the tenant does not exist.
	Unsuspend(ctx context.Context, tenantID string) (*SuspendResult, error)
}

// SuspendResult reports what a Suspend/Unsuspend call actually changed.
type SuspendResult struct {
	Status         string // the tenant's status AFTER the call
	StoresAffected int    // number of store rows the cascade touched
	Changed        bool   // false when the tenant was already in the target state
}

// StoreRef identifies one store under a tenant: the id an operator purge
// passes to marketplace-api, and the slug it confirms against.
type StoreRef struct {
	ID   string `gorm:"column:id"`
	Slug string `gorm:"column:slug"`
}

// TeardownSnapshot is a tenant's identifying state as of the moment the
// teardown transaction locked its row.
type TeardownSnapshot struct {
	TenantID    string
	Name        string
	OwnerUserID string
	Stores      []StoreRef
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

func (r *gormRepository) OwnerEmailExists(ctx context.Context, email string) (bool, error) {
	normalized := strings.ToLower(strings.TrimSpace(email))
	if normalized == "" {
		return false, nil
	}
	var count int64
	err := r.db.WithContext(ctx).Model(&Tenant{}).
		Where("lower(owner_email) = ?", normalized).
		Count(&count).Error
	if err != nil {
		return false, fmt.Errorf("tenant: owner email exists %q: %w", normalized, err)
	}
	return count > 0, nil
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

func (r *gormRepository) ListStoreIDs(ctx context.Context, tx *gorm.DB, tenantID string) ([]string, error) {
	db := r.db
	if tx != nil {
		db = tx
	}
	var ids []string
	if err := db.WithContext(ctx).
		Table("stores").Where("tenant_id = ?", tenantID).Pluck("id", &ids).Error; err != nil {
		return nil, fmt.Errorf("tenant: list store ids: %w", err)
	}
	return ids, nil
}

func (r *gormRepository) SnapshotForTeardown(ctx context.Context, tx *gorm.DB, tenantID string) (*TeardownSnapshot, error) {
	db := r.db
	if tx != nil {
		db = tx
	}

	var row struct {
		ID          string
		Name        string
		OwnerUserID string
	}
	res := db.WithContext(ctx).
		Raw(`SELECT id, name, owner_user_id FROM tenants WHERE id = ? FOR UPDATE`, tenantID).
		Scan(&row)
	if res.Error != nil {
		return nil, fmt.Errorf("tenant: snapshot for teardown: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return nil, apperrors.NotFound("tenant_not_found",
			fmt.Sprintf("tenant %q does not exist", tenantID))
	}

	stores := make([]StoreRef, 0, 4)
	if err := db.WithContext(ctx).
		Raw(`SELECT id, slug FROM stores WHERE tenant_id = ? ORDER BY slug`, tenantID).
		Scan(&stores).Error; err != nil {
		return nil, fmt.Errorf("tenant: snapshot store refs: %w", err)
	}

	return &TeardownSnapshot{
		TenantID:    row.ID,
		Name:        row.Name,
		OwnerUserID: row.OwnerUserID,
		Stores:      stores,
	}, nil
}

func (r *gormRepository) DeleteInTx(ctx context.Context, tx *gorm.DB, tenantID string) error {
	// onboarding_sessions.tenant_id is ON DELETE SET NULL, but the
	// onboarding_sessions_completed_consistency CHECK requires tenant_id
	// NOT NULL when status='completed'. Deleting the tenant would null it and
	// violate the CHECK. Delete the completed session rows first; their
	// verification codes cascade (ON DELETE CASCADE).
	if err := tx.WithContext(ctx).
		Exec(`DELETE FROM onboarding_sessions WHERE tenant_id = ?`, tenantID).Error; err != nil {
		return fmt.Errorf("tenant: reconcile onboarding_sessions: %w", err)
	}
	// stores + invitations FK to tenants ON DELETE CASCADE — removed automatically.
	res := tx.WithContext(ctx).Exec(`DELETE FROM tenants WHERE id = ?`, tenantID)
	if res.Error != nil {
		return fmt.Errorf("tenant: delete: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return apperrors.NotFound("tenant_not_found", fmt.Sprintf("tenant %q does not exist", tenantID))
	}
	return nil
}

func (r *gormRepository) SlugExists(ctx context.Context, slug string) (bool, error) {
	var count int64
	if err := r.db.WithContext(ctx).Model(&Tenant{}).Where("slug = ?", slug).Count(&count).Error; err != nil {
		return false, fmt.Errorf("tenant: slug exists %q: %w", slug, err)
	}
	return count > 0, nil
}

func (r *gormRepository) Suspend(ctx context.Context, tenantID string) (*SuspendResult, error) {
	out := &SuspendResult{}
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var t Tenant
		if err := tx.Raw(
			`SELECT * FROM tenants WHERE id = ? FOR UPDATE`, tenantID).Scan(&t).Error; err != nil {
			return fmt.Errorf("tenant: suspend: lock tenant %q: %w", tenantID, err)
		}
		if t.ID == "" {
			return apperrors.NotFound("tenant_not_found", fmt.Sprintf("tenant %q does not exist", tenantID))
		}
		out.Status = t.Status
		if t.Status == StatusSuspended {
			return nil // already suspended: Changed stays false, StoresAffected 0
		}
		if t.Status != StatusActive {
			// archived, or anything added later: refuse rather than guess.
			return apperrors.Conflict("tenant_suspend_conflict",
				fmt.Sprintf("tenant %q has status %q and cannot be suspended", tenantID, t.Status))
		}

		// Cascade to currently-ACTIVE stores only, flagging exactly what we
		// changed so Unsuspend can undo precisely this and nothing else.
		res := tx.Exec(`
			UPDATE stores SET status = ?, suspended_by_tenant = true
			WHERE tenant_id = ? AND status = ?`, StatusSuspended, tenantID, StatusActive)
		if res.Error != nil {
			return fmt.Errorf("tenant: suspend: cascade stores for %q: %w", tenantID, res.Error)
		}
		out.StoresAffected = int(res.RowsAffected)

		if err := tx.Exec(`UPDATE tenants SET status = ? WHERE id = ?`,
			StatusSuspended, tenantID).Error; err != nil {
			return fmt.Errorf("tenant: suspend: update tenant %q: %w", tenantID, err)
		}
		out.Status = StatusSuspended
		out.Changed = true
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (r *gormRepository) Unsuspend(ctx context.Context, tenantID string) (*SuspendResult, error) {
	out := &SuspendResult{}
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var t Tenant
		if err := tx.Raw(
			`SELECT * FROM tenants WHERE id = ? FOR UPDATE`, tenantID).Scan(&t).Error; err != nil {
			return fmt.Errorf("tenant: unsuspend: lock tenant %q: %w", tenantID, err)
		}
		if t.ID == "" {
			return apperrors.NotFound("tenant_not_found", fmt.Sprintf("tenant %q does not exist", tenantID))
		}
		out.Status = t.Status
		if t.Status != StatusSuspended {
			return nil // not suspended: no-op, Changed stays false
		}

		// Restore ONLY the stores THIS cascade suspended, then clear the
		// flag. A store suspended individually (suspended_by_tenant=false)
		// must stay suspended — that is the whole point of the column.
		res := tx.Exec(`
			UPDATE stores SET status = ?, suspended_by_tenant = false
			WHERE tenant_id = ? AND suspended_by_tenant`, StatusActive, tenantID)
		if res.Error != nil {
			return fmt.Errorf("tenant: unsuspend: restore stores for %q: %w", tenantID, res.Error)
		}
		out.StoresAffected = int(res.RowsAffected)

		if err := tx.Exec(`UPDATE tenants SET status = ? WHERE id = ?`,
			StatusActive, tenantID).Error; err != nil {
			return fmt.Errorf("tenant: unsuspend: update tenant %q: %w", tenantID, err)
		}
		out.Status = StatusActive
		out.Changed = true
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
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
