package apikeys

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Repo is the persistence boundary for enterprise_api_keys.
type Repo struct {
	db *gorm.DB
}

// NewRepo constructs a stateless repo around a *gorm.DB.
func NewRepo(db *gorm.DB) *Repo { return &Repo{db: db} }

func (r *Repo) dbCtx(ctx context.Context) *gorm.DB {
	if ctx == nil {
		return r.db
	}
	return r.db.WithContext(ctx)
}

// Create inserts a new APIKey row. Caller must populate ID, TenantID,
// StoreID, KeyPrefix, KeyHash, and Label; CreatedAt defaults via GORM.
func (r *Repo) Create(ctx context.Context, k *APIKey) error {
	return r.dbCtx(ctx).Create(k).Error
}

// FindByTenantPrefix is the hot-path lookup used by the auth middleware.
// Returns ErrNotFound when no row matches; revocation/rotation-overlap is
// the caller's responsibility (via APIKey.IsUsable).
func (r *Repo) FindByTenantPrefix(ctx context.Context, tenantID uuid.UUID, prefix string) (APIKey, error) {
	var k APIKey
	err := r.dbCtx(ctx).
		Where("tenant_id = ? AND key_prefix = ?", tenantID, prefix).
		First(&k).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return APIKey{}, ErrNotFound
	}
	return k, err
}

// FindByPrefix returns the (potentially multiple) rows matching a prefix
// across tenants. Used by the auth middleware when the caller supplies only
// a bearer token (no tenant header). Returns at most a small handful in
// practice — collisions across tenants are vanishingly rare.
func (r *Repo) FindByPrefix(ctx context.Context, prefix string) ([]APIKey, error) {
	var out []APIKey
	err := r.dbCtx(ctx).Where("key_prefix = ?", prefix).Find(&out).Error
	return out, err
}

// FindByID returns a single key by ID. Used by Rotate + Revoke.
func (r *Repo) FindByID(ctx context.Context, id uuid.UUID) (APIKey, error) {
	var k APIKey
	err := r.dbCtx(ctx).Where("id = ?", id).First(&k).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return APIKey{}, ErrNotFound
	}
	return k, err
}

// ListForStore returns all rows (active + revoked) for a tenant+store, most
// recent first. Caller is responsible for filtering before HTTP response.
func (r *Repo) ListForStore(ctx context.Context, tenantID, storeID uuid.UUID) ([]APIKey, error) {
	var out []APIKey
	err := r.dbCtx(ctx).
		Where("tenant_id = ? AND store_id = ?", tenantID, storeID).
		Order("created_at DESC").
		Find(&out).Error
	return out, err
}

// CountActiveForStore counts active (non-revoked) keys for a tenant+store.
// Used for plan-ceiling enforcement.
// CountActiveForStore counts keys still usable for a store.
//
// This is the ONLY place in this package that compares a Go-written
// `revoked_at` against Postgres's own `now()`. Everywhere revocation actually
// gates access — IsUsable (model.go), the auth middleware (middleware.go:74),
// Rotate and Revoke (service.go) — both sides come from the same application
// clock, so those paths cannot disagree with themselves.
//
// That asymmetry is deliberate and worth keeping in mind rather than
// "fixing": making Revoke write the database's now() instead would introduce
// a mixed-clock comparison on the AUTH path, where today there is none. The
// cost is confined here, to a quota count, where a few milliseconds of skew
// between the app and the database cannot matter — no caller revokes a key
// and counts it in the same instant. A test did, and was flaky for it (#447).
func (r *Repo) CountActiveForStore(ctx context.Context, tenantID, storeID uuid.UUID) (int64, error) {
	var n int64
	err := r.dbCtx(ctx).Model(&APIKey{}).
		Where("tenant_id = ? AND store_id = ? AND (revoked_at IS NULL OR revoked_at > now())", tenantID, storeID).
		Count(&n).Error
	return n, err
}

// Revoke sets revoked_at + revoked_reason on a single key. Pass `at` =
// `now()+24h` for rotation-overlap behaviour, or `at` = `now()` for
// immediate revocation.
func (r *Repo) Revoke(ctx context.Context, id uuid.UUID, at time.Time, reason string) error {
	return r.dbCtx(ctx).Model(&APIKey{}).
		Where("id = ?", id).
		Updates(map[string]any{
			"revoked_at":     at,
			"revoked_reason": reason,
		}).Error
}

// UpdateLastUsed is fire-and-forget from the middleware worker. Bounded
// retries are unnecessary — losing an update just means one fewer last_used
// observation.
func (r *Repo) UpdateLastUsed(ctx context.Context, id uuid.UUID, at time.Time, ipHash string) error {
	return r.dbCtx(ctx).Model(&APIKey{}).
		Where("id = ?", id).
		Updates(map[string]any{
			"last_used_at":      at,
			"last_used_ip_hash": ipHash,
		}).Error
}
