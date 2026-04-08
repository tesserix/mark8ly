// Package invitation owns the tenant invitation domain for Phase P.
//
// An invitation is a pending "add user X to tenant Y with role Z"
// request. The row stores a sha256 hash of the random token; the
// plaintext only ever exists in the email sent to the invitee. On
// accept the handler hashes the incoming token and looks up the row.
//
// Roles are a constrained subset of authz.Role: admin/staff/viewer.
// Owner is intentionally excluded — owners are founders (created by
// onboarding) and can only change via a dedicated transfer-ownership
// flow that doesn't exist yet.
package invitation

import "time"

// Invitation is the DB row.
//
// Phase R: StoreID is nullable. When set, the accept endpoint
// writes a store-level FGA tuple (e.g. user:X manager store:Y).
// When null, it writes a tenant-level tuple (Phase P behaviour).
type Invitation struct {
	ID              string     `gorm:"primaryKey;column:id;type:uuid;default:gen_random_uuid()" json:"id"`
	TenantID        string     `gorm:"column:tenant_id;type:uuid;not null"                      json:"tenant_id"`
	StoreID         *string    `gorm:"column:store_id;type:uuid"                                json:"store_id,omitempty"`
	Email           string     `gorm:"column:email;type:varchar(320);not null"                  json:"email"`
	Role            string     `gorm:"column:role;type:varchar(20);not null"                    json:"role"`
	TokenHash       string     `gorm:"column:token_hash;type:char(64);not null;uniqueIndex"     json:"-"`
	ExpiresAt       time.Time  `gorm:"column:expires_at;not null"                               json:"expires_at"`
	Status          string     `gorm:"column:status;type:varchar(20);not null;default:'pending'" json:"status"`
	InvitedByUserID string     `gorm:"column:invited_by_user_id;type:varchar(128);not null"     json:"invited_by_user_id"`
	CreatedAt       time.Time  `gorm:"column:created_at;not null;default:now()"                 json:"created_at"`
	AcceptedAt      *time.Time `gorm:"column:accepted_at"                                       json:"accepted_at,omitempty"`
	// AcceptedByUserID is the GIP UID of the user that accepted the
	// invitation. Populated at accept time so admin Settings → Team
	// can change the teammate's role later without a roundtrip to
	// auth-bff to resolve email → UID. Nullable for invitations that
	// were accepted before migration 0012 landed.
	AcceptedByUserID *string `gorm:"column:accepted_by_user_id;type:varchar(128)"             json:"accepted_by_user_id,omitempty"`
}

// TableName overrides GORM's default pluralization.
func (Invitation) TableName() string { return "invitations" }

// Status constants match the CHECK constraint in migration 0007.
const (
	StatusPending  = "pending"
	StatusAccepted = "accepted"
	StatusExpired  = "expired"
	StatusRevoked  = "revoked"
)

// isAllowedTenantRole enumerates the tenant-level roles a tenant-
// wide invitation can grant. Owner is excluded — tenants have
// exactly one owner, transferred only through a dedicated flow.
func isAllowedTenantRole(role string) bool {
	switch role {
	case "admin", "staff", "viewer":
		return true
	}
	return false
}

// isAllowedStoreRole enumerates the store-level roles a Phase R
// store-scoped invitation can grant. These map to the `store` type
// relations defined in infra/openfga/model.fga: manager (closest
// to a tenant admin), staff, viewer. Owner is excluded for the
// same reason as at the tenant level.
func isAllowedStoreRole(role string) bool {
	switch role {
	case "manager", "staff", "viewer":
		return true
	}
	return false
}
