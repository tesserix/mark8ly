// Package breakglass implements §12.4 emergency local admin accounts
// for Pro+SSO tenants. One account per tenant, dual-factor login
// (password + mandatory TOTP), credentials live in GCP Secret Manager
// at `/projects/tesserix-prod/secrets/break-glass-{tenant_id}`. DB
// stores only bcrypt(password) + a secret-path pointer. Login triggers
// an immediate 24h rotation; a daily cron also rotates anything older
// than 90 days.
package breakglass

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

// Account is the per-tenant break-glass record. Exactly one row per
// tenant (tenant_id is the primary key).
//
// Invariants enforced by the rotator + login handler:
//   - `password_hash` is always bcrypt cost 12.
//   - Plaintext password + totp secret live ONLY in Secret Manager at
//     `secret_path`. Neither ever hits this row.
//   - `rotation_scheduled_at` is set to now()+24h on every successful
//     login; cleared when the rotator runs.
type Account struct {
	TenantID            uuid.UUID  `gorm:"column:tenant_id;type:uuid;primaryKey"`
	SecretPath          string     `gorm:"column:secret_path;not null"`
	PasswordHash        string     `gorm:"column:password_hash;not null"`
	TOTPSecretRef       string     `gorm:"column:totp_secret_ref;not null"`
	TOTPEnrolled        bool       `gorm:"column:totp_enrolled;not null;default:false"`
	LastRotatedAt       time.Time  `gorm:"column:last_rotated_at;not null;default:now()"`
	LastUsedAt          *time.Time `gorm:"column:last_used_at"`
	RotationScheduledAt *time.Time `gorm:"column:rotation_scheduled_at"`
	CreatedAt           time.Time  `gorm:"column:created_at;not null;autoCreateTime"`
	UpdatedAt           time.Time  `gorm:"column:updated_at;not null;autoUpdateTime"`
}

// TableName pins the Postgres table name.
func (Account) TableName() string { return "break_glass_accounts" }

// Lockout persists a rate-limit decision so it survives Redis flushes
// and process restarts. `IPHash` is an HMAC-SHA256 of the client IP —
// raw IPs must never be stored on this table.
type Lockout struct {
	IPHash      []byte    `gorm:"column:ip_hash;type:bytea;primaryKey"`
	TenantID    *uuid.UUID `gorm:"column:tenant_id;type:uuid"`
	LockedUntil time.Time `gorm:"column:locked_until;not null;primaryKey"`
	Reason      string    `gorm:"column:reason;not null"`
	CreatedAt   time.Time `gorm:"column:created_at;not null;autoCreateTime"`
}

// TableName pins the Postgres table name.
func (Lockout) TableName() string { return "break_glass_lockouts" }

// Sentinel errors. Callers use errors.Is to distinguish these from
// arbitrary SQL / Secret Manager failures.
var (
	ErrNotFound            = errors.New("breakglass: not found")
	ErrAlreadyProvisioned  = errors.New("breakglass: account already exists for tenant")
	ErrInvalidCredentials  = errors.New("breakglass: invalid credentials")
	ErrLocked              = errors.New("breakglass: ip locked out")
	ErrSecretManagerFailed = errors.New("breakglass: secret manager i/o failed")
)

// TOTP pointer value used everywhere in this package. Secret Manager
// blobs store the TOTP secret under this JSON field.
const TOTPJSONPointer = "$.totp_secret"
