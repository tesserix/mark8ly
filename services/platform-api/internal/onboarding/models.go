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
// Those three — in_progress, verifying, completed — are the ONLY statuses
// this package ever writes.
//
// "abandoned" and "expired" are declared below and permitted by the
// migration's CHECK constraint, but nothing writes them: there is no gc,
// cron, sweep or reaper for onboarding sessions anywhere in the workspace.
// Every session in production is in_progress, verifying or completed.
//
// This comment used to claim "at any point a session may transition to
// status='abandoned' (browser close) or 'expired' (gc)", which described a
// lifecycle the code does not implement. #283 nearly built an abandoned-
// session counter on `status = 'abandoned'`; it would have returned zero for
// every window, forever, and read as a data problem rather than a missing
// feature. It instead derives abandonment from `last_activity_at` (idle >24h,
// not completed) and keeps the stored status and the derived flag as separate
// fields on the wire, so the gap stays visible.
//
// If a gc is wanted for its own sake (#322 option 2), the constants are here
// and the constraint already permits them — but the derived flag and the
// stored status would then need to agree during the transition.
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
	ID    string `gorm:"primaryKey;column:id;type:uuid;default:gen_random_uuid()" json:"id"`
	Email string `gorm:"column:email;type:varchar(320);not null;index"            json:"email"`
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
//
// StatusExpired and StatusAbandoned are RESERVED: permitted by the
// constraint, referenced by no writer. See the package doc above before
// building anything that reads for them.
const (
	StatusInProgress = "in_progress"
	StatusVerifying  = "verifying"
	StatusCompleted  = "completed"
	StatusExpired    = "expired"
	StatusAbandoned  = "abandoned"
)
