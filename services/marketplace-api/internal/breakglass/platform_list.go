package breakglass

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Page sizing, mirroring the outbox/email-log platform reads (#331, #348D).
const (
	DefaultPlatformPageSize = 50
	MaxPlatformPageSize     = 200
)

// PlatformListFilter narrows the cross-tenant break-glass inventory (#333).
type PlatformListFilter struct {
	TenantID *uuid.UUID
	// UsedAfter/UsedBefore filter on last_used_at.
	UsedAfter, UsedBefore *time.Time
	// Used, when non-nil, narrows to accounts that have (true) or have not
	// (false) ever been used — i.e. whether last_used_at IS NOT NULL.
	Used *bool
	// Locked, when non-nil, narrows to accounts whose tenant currently has
	// (true) or does not have (false) an active lockout row. See
	// PlatformRow.LockedOut for what "locked" means here.
	Locked   *bool
	SortDesc bool
	Page     int
	Limit    int
}

// PlatformRow is one break-glass account in the cross-tenant platform-console
// view.
//
// Deliberately excludes secret_path, password_hash, and totp_secret_ref —
// the three columns on break_glass_accounts that can reach the live
// plaintext credential or its bcrypt hash. See
// TestPlatformRowCannotCarryCredentialFields, which guards this by
// reflection so a column added later cannot silently become reachable.
//
// TenantName is populated by the handler layer from tenantdirectory, not by
// ListPlatform — break_glass_accounts has no tenant_name column.
type PlatformRow struct {
	TenantID            uuid.UUID  `json:"tenant_id"`
	TenantName          string     `json:"tenant_name"`
	TOTPEnrolled        bool       `json:"totp_enrolled"`
	LastUsedAt          *time.Time `json:"last_used_at"`
	LastRotatedAt       time.Time  `json:"last_rotated_at"`
	RotationScheduledAt *time.Time `json:"rotation_scheduled_at"`
	// LockedOut means "at least one break_glass_lockouts row currently
	// names this tenant" — lockouts are keyed per-IP, not per-account, and
	// tenant_id is only populated on a lockout row once a failed login
	// attempt actually resolved a tenant. An attempt that never resolved a
	// tenant is invisible here. This is NOT "this account is locked" in
	// any account-level sense.
	LockedOut bool `json:"locked_out"`
	// LockoutExpiresAt is the latest locked_until among this tenant's
	// active lockout rows, or nil when LockedOut is false.
	LockoutExpiresAt *time.Time `json:"lockout_expires_at"`
	CreatedAt        time.Time  `json:"created_at"`
}

// PlatformListResult is a page plus the unpaginated total.
type PlatformListResult struct {
	Accounts []PlatformRow
	Total    int64
}

// ListPlatform serves the platform console's cross-tenant break-glass
// inventory (#333).
//
// A package function rather than a method, so it can be wired through a
// ...Func adapter in main.go exactly as emaillog.ListPlatform is.
//
// asOf pins "now" for the active-lockout window (locked_until > asOf) so
// the query is reproducible in tests instead of racing time.Now().
func ListPlatform(ctx context.Context, db *gorm.DB, f PlatformListFilter,
	asOf time.Time) (PlatformListResult, error) {
	result := PlatformListResult{Accounts: make([]PlatformRow, 0)}

	q := db.WithContext(ctx).Table("break_glass_accounts a")

	if f.TenantID != nil {
		q = q.Where("a.tenant_id = ?", *f.TenantID)
	}
	if f.UsedAfter != nil {
		q = q.Where("a.last_used_at >= ?", *f.UsedAfter)
	}
	if f.UsedBefore != nil {
		q = q.Where("a.last_used_at <= ?", *f.UsedBefore)
	}
	if f.Used != nil {
		if *f.Used {
			q = q.Where("a.last_used_at IS NOT NULL")
		} else {
			q = q.Where("a.last_used_at IS NULL")
		}
	}
	if f.Locked != nil {
		exists := `EXISTS (
			SELECT 1 FROM break_glass_lockouts l
			WHERE l.tenant_id = a.tenant_id AND l.locked_until > ?
		)`
		if *f.Locked {
			q = q.Where(exists, asOf)
		} else {
			q = q.Where("NOT "+exists, asOf)
		}
	}

	// Count BEFORE Select: the page below adds computed lockout columns,
	// and Total must be the full match count, not the page size.
	if err := q.Count(&result.Total).Error; err != nil {
		return result, fmt.Errorf("breakglass platform list count: %w", err)
	}

	limit := f.Limit
	if limit <= 0 {
		limit = DefaultPlatformPageSize
	}
	if limit > MaxPlatformPageSize {
		limit = MaxPlatformPageSize
	}
	page := max(f.Page, 1)

	order := "a.last_used_at DESC NULLS LAST, a.tenant_id"
	if !f.SortDesc {
		order = "a.last_used_at ASC NULLS LAST, a.tenant_id"
	}

	if err := q.
		Select(`a.tenant_id, a.totp_enrolled, a.last_used_at,
			a.last_rotated_at, a.rotation_scheduled_at, a.created_at,
			EXISTS (
				SELECT 1 FROM break_glass_lockouts l
				WHERE l.tenant_id = a.tenant_id AND l.locked_until > ?
			) AS locked_out,
			(
				SELECT MAX(l.locked_until) FROM break_glass_lockouts l
				WHERE l.tenant_id = a.tenant_id AND l.locked_until > ?
			) AS lockout_expires_at`, asOf, asOf).
		Order(order).
		Offset((page - 1) * limit).
		Limit(limit).
		Scan(&result.Accounts).Error; err != nil {
		return result, fmt.Errorf("breakglass platform list: %w", err)
	}
	return result, nil
}
