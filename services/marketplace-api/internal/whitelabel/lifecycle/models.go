// Package lifecycle owns the white-label app teardown state machine
// (spec §13.5). The Advancer drives the day 7/30/60/90 cadence. The
// Consumer seeds state rows on subscription.pro_app_cancelled events.
package lifecycle

import (
	"time"

	"github.com/google/uuid"

	"github.com/mark8ly/marketplace-api/internal/subscription"
)

// Status aliases subscription.WhiteLabelAppStatus so callers inside this
// package can use the shorter name without a circular import. The
// canonical enum values live in the subscription package (migration
// 000048 constraints them).
type Status = subscription.WhiteLabelAppStatus

// Re-export canonical status constants for advancer convenience.
const (
	StatusActive             = subscription.AppStatusActive
	StatusSunsetScheduled    = subscription.AppStatusSunsetScheduled
	StatusDownloadsBlocked   = subscription.AppStatusDownloadsBlocked
	StatusPulled             = subscription.AppStatusPulled
	StatusFirebaseArchived   = subscription.AppStatusFirebaseArchived
	StatusCredentialsPurged  = subscription.AppStatusCredentialsPurged
)

// Row is the GORM model for the white_label_app_state table (migration
// 000076). One row per store; mutable. See models.go comment and the
// migration for the pair-of-tables design rationale.
type Row struct {
	ID                uuid.UUID  `gorm:"column:id;type:uuid;primaryKey;default:gen_random_uuid()"`
	TenantID          uuid.UUID  `gorm:"column:tenant_id;type:uuid;not null"`
	StoreID           uuid.UUID  `gorm:"column:store_id;type:uuid;not null;uniqueIndex"`
	Status            Status     `gorm:"column:status;type:varchar(30);not null"`
	NextActionAt      *time.Time `gorm:"column:next_action_at"`
	ScheduledAt       *time.Time `gorm:"column:scheduled_at"`
	AppleAppID        string     `gorm:"column:apple_app_id;type:varchar(100)"`
	GooglePackage     string     `gorm:"column:google_package;type:varchar(255)"`
	FirebaseProjectID string     `gorm:"column:firebase_project_id;type:varchar(100)"`
	MerchantInitiated bool       `gorm:"column:merchant_initiated;not null;default:false"`
	CreatedAt         time.Time  `gorm:"column:created_at;not null;default:now()"`
	UpdatedAt         time.Time  `gorm:"column:updated_at;not null;default:now()"`
}

// TableName maps Row to the mutable state table.
func (Row) TableName() string { return "white_label_app_state" }
