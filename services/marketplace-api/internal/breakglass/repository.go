package breakglass

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Repository is the persistence boundary for break_glass_accounts and
// break_glass_lockouts. All methods scope by tenant_id (for Account)
// or ip_hash (for Lockout); cross-tenant reads are impossible by
// construction because the primary keys carry the scope.
type Repository struct {
	db *gorm.DB
}

// NewRepository returns a Repository bound to db.
func NewRepository(db *gorm.DB) *Repository { return &Repository{db: db} }

func (r *Repository) ctx(ctx context.Context) *gorm.DB {
	if ctx == nil {
		return r.db
	}
	return r.db.WithContext(ctx)
}

// Create inserts a new Account. Returns ErrAlreadyProvisioned if a row
// already exists for this tenant (PK conflict).
func (r *Repository) Create(ctx context.Context, a *Account) error {
	if a == nil || a.TenantID == uuid.Nil {
		return ErrInvalidCredentials
	}
	err := r.ctx(ctx).Create(a).Error
	if err == nil {
		return nil
	}
	// Map PK conflicts to a clean sentinel so callers don't have to
	// know about Postgres's error codes.
	if isUniqueViolation(err) {
		return ErrAlreadyProvisioned
	}
	return err
}

// GetByTenant returns the single account for tenantID, or ErrNotFound.
func (r *Repository) GetByTenant(ctx context.Context, tenantID uuid.UUID) (*Account, error) {
	var a Account
	err := r.ctx(ctx).
		Where("tenant_id = ?", tenantID).
		First(&a).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &a, nil
}

// UpdateAfterUse records a successful login: stamps last_used_at = now
// and schedules the post-use rotation for now+24h. Returns ErrNotFound
// when the tenant has no account (0 RowsAffected).
func (r *Repository) UpdateAfterUse(ctx context.Context, tenantID uuid.UUID) error {
	now := time.Now().UTC()
	rotate := now.Add(24 * time.Hour)
	res := r.ctx(ctx).
		Model(&Account{}).
		Where("tenant_id = ?", tenantID).
		Updates(map[string]any{
			"last_used_at":          &now,
			"rotation_scheduled_at": &rotate,
			"updated_at":            now,
		})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// ReplaceAfterRotation swaps in a new password_hash, stamps
// last_rotated_at = now, and clears rotation_scheduled_at. Callers
// MUST write Secret Manager before calling this so a DB-only failure
// never leaves the tenant with a hash that doesn't match the blob.
func (r *Repository) ReplaceAfterRotation(ctx context.Context, tenantID uuid.UUID, newHash string) error {
	now := time.Now().UTC()
	res := r.ctx(ctx).
		Model(&Account{}).
		Where("tenant_id = ?", tenantID).
		Updates(map[string]any{
			"password_hash":         newHash,
			"last_rotated_at":       now,
			"rotation_scheduled_at": nil,
			"updated_at":            now,
		})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// Disable marks tenantID's account disabled with reason (#404). The login
// handler refuses a disabled account before ever reaching Secret Manager
// (see break_glass_login.go step 3.5), by reading DisabledAt off the SAME
// row GetByTenant already fetches — no new query is added to that path.
// Returns ErrNotFound when the tenant has no account.
func (r *Repository) Disable(ctx context.Context, tenantID uuid.UUID, reason string) error {
	now := time.Now().UTC()
	res := r.ctx(ctx).
		Model(&Account{}).
		Where("tenant_id = ?", tenantID).
		Updates(map[string]any{
			"disabled_at":     &now,
			"disabled_reason": reason,
			"updated_at":      now,
		})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// Enable clears BOTH disabled_at and disabled_reason, restoring login.
// A partial clear would leave stale forensic text on an otherwise clean,
// re-enabled account. Returns ErrNotFound when the tenant has no account.
//
// Enable is the ONLY thing that clears disabled_at — rotation
// (ReplaceAfterRotation, rotation.go) deliberately never touches it, so an
// operator rotating credentials for an unrelated reason cannot silently
// re-enable an account someone disabled on purpose.
func (r *Repository) Enable(ctx context.Context, tenantID uuid.UUID) error {
	res := r.ctx(ctx).
		Model(&Account{}).
		Where("tenant_id = ?", tenantID).
		Updates(map[string]any{
			"disabled_at":     nil,
			"disabled_reason": nil,
			"updated_at":      time.Now().UTC(),
		})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// ClearIPLock deletes ACTIVE lockout rows (locked_until > now()) for
// ipHash, returning how many were removed. An expired row is left alone —
// it is already inert and harmless as forensic history, and deleting it
// would not change anything IsIPLocked observes.
//
// ipHash MUST be computed with the same HMAC key the login path uses
// (breakglass.HMACIPHash) — a mismatched key hashes to different bytes,
// so this deletes zero rows while still reporting success.
func (r *Repository) ClearIPLock(ctx context.Context, ipHash []byte) (int64, error) {
	if len(ipHash) == 0 {
		return 0, ErrInvalidCredentials
	}
	res := r.ctx(ctx).
		Where("ip_hash = ? AND locked_until > ?", ipHash, time.Now().UTC()).
		Delete(&Lockout{})
	if res.Error != nil {
		return 0, res.Error
	}
	return res.RowsAffected, nil
}

// FindDueForRotation returns every account that needs a new password
// right now: either the post-use rotation clock has elapsed, or the
// 90-day cadence has lapsed.
//
// `olderThan` is the cutoff for the 90-day rule (callers pass
// time.Now().Add(-90*24*time.Hour)); separating the clock into the
// caller keeps the repository deterministic for testing.
func (r *Repository) FindDueForRotation(ctx context.Context, olderThan time.Time) ([]Account, error) {
	var accs []Account
	err := r.ctx(ctx).
		Where(`
			(rotation_scheduled_at IS NOT NULL AND rotation_scheduled_at <= now())
			OR last_rotated_at <= ?
		`, olderThan).
		Find(&accs).Error
	return accs, err
}

// LockIP persists a lockout row. Safe to call repeatedly — PK is
// (ip_hash, locked_until) so parallel attackers can't thrash the
// table.
func (r *Repository) LockIP(ctx context.Context, ipHash []byte, tenantID *uuid.UUID, reason string, dur time.Duration) error {
	if len(ipHash) == 0 {
		return ErrInvalidCredentials
	}
	lock := &Lockout{
		IPHash:      ipHash,
		TenantID:    tenantID,
		LockedUntil: time.Now().UTC().Add(dur),
		Reason:      reason,
	}
	err := r.ctx(ctx).Create(lock).Error
	if err != nil && isUniqueViolation(err) {
		// Already locked at the same instant — treat as no-op.
		return nil
	}
	return err
}

// IsIPLocked returns true if there's any active lockout row for this
// ip_hash (locked_until > now()).
func (r *Repository) IsIPLocked(ctx context.Context, ipHash []byte) (bool, error) {
	if len(ipHash) == 0 {
		return false, nil
	}
	var count int64
	err := r.ctx(ctx).
		Model(&Lockout{}).
		Where("ip_hash = ? AND locked_until > ?", ipHash, time.Now().UTC()).
		Count(&count).Error
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// isUniqueViolation recognises Postgres 23505 regardless of driver
// wrapping; also matches sqlite's phrasing so unit tests don't have
// to pin a specific driver.
func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "duplicate key") ||
		strings.Contains(msg, "unique constraint") ||
		strings.Contains(msg, "UNIQUE constraint")
}
