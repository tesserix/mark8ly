// Package onboarding owns the wizard-state lifecycle for new merchants.
//
// Lifecycle:
//
//	create     →  status="in_progress", draft={}
//	  ↓ (wizard collects business info, country, slug, etc.)
//	save_draft →  draft = merged JSON, status="in_progress"
//	  ↓
//	send_otp   →  status="verifying"
//	  ↓
//	verify_otp →  email_verified_at = now, status="in_progress"
//	  ↓
//	complete   →  spawns tenant + outbox FGA write, status="completed"
//
// At any point a session may transition to status="abandoned" (browser close)
// or "expired" (gc).
//
// The completion handler is the bug-fix landing point for auth-bug #2 and #3
// from docs/planning/auth-bugs.md. It writes both the tenant row and the
// outbox FGA-tuple write in a single DB transaction. The outbox drainer
// (separate goroutine) eventually ships the tuple to OpenFGA. The user's
// auto-login on the auth-bff side does an FGA Check with retry, so the race
// window is closed at both ends.
package onboarding

import (
	"encoding/json"
	"time"
)

// Session is one in-progress onboarding flow.
type Session struct {
	ID              string          `gorm:"primaryKey;column:id;type:uuid;default:gen_random_uuid()" json:"id"`
	Email           string          `gorm:"column:email;type:varchar(320);not null;index"            json:"email"`
	// Draft holds whatever the wizard has collected so far. JSON shape is
	// not enforced at the DB layer — the handler validates per-step.
	Draft           json.RawMessage `gorm:"column:draft;type:jsonb;not null;default:'{}'::jsonb"     json:"draft"`
	Status          string          `gorm:"column:status;type:varchar(20);not null;default:'in_progress'" json:"status"`
	EmailVerifiedAt *time.Time      `gorm:"column:email_verified_at"                                  json:"email_verified_at,omitempty"`
	TenantID        *string         `gorm:"column:tenant_id;type:uuid"                                json:"tenant_id,omitempty"`
	CompletedAt     *time.Time      `gorm:"column:completed_at"                                       json:"completed_at,omitempty"`
	LastActivityAt  time.Time       `gorm:"column:last_activity_at;not null;default:now()"            json:"last_activity_at"`
	CreatedAt       time.Time       `gorm:"column:created_at;not null;default:now()"                  json:"created_at"`
	UpdatedAt       time.Time       `gorm:"column:updated_at;not null;default:now()"                  json:"updated_at"`
}

// TableName overrides GORM's default pluralization.
func (Session) TableName() string { return "onboarding_sessions" }

// Status constants. Match the CHECK constraint in the migration.
const (
	StatusInProgress = "in_progress"
	StatusVerifying  = "verifying"
	StatusCompleted  = "completed"
	StatusExpired    = "expired"
	StatusAbandoned  = "abandoned"
)
