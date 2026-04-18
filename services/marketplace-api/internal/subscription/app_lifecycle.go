package subscription

import (
	"time"

	"github.com/google/uuid"
)

// WhiteLabelAppStatus enumerates the lifecycle states of a merchant's
// white-label mobile app (§13.5 of the subscription spec).
type WhiteLabelAppStatus string

const (
	AppStatusActive            WhiteLabelAppStatus = "active"
	AppStatusSunsetScheduled   WhiteLabelAppStatus = "sunset_scheduled"
	AppStatusDownloadsBlocked  WhiteLabelAppStatus = "downloads_blocked"
	AppStatusPulled            WhiteLabelAppStatus = "pulled"
	AppStatusFirebaseArchived  WhiteLabelAppStatus = "firebase_archived"
	AppStatusCredentialsPurged WhiteLabelAppStatus = "credentials_purged"
)

// WhiteLabelAppLifecycleEntry is the GORM model for the
// white_label_app_lifecycle table. It records every transition of a
// merchant's app through its lifecycle so the platform maintains a full
// audit trail — who acted, when, and why.
type WhiteLabelAppLifecycleEntry struct {
	ID          uuid.UUID           `gorm:"column:id;type:uuid;primaryKey;default:gen_random_uuid()"`
	StoreID     uuid.UUID           `gorm:"column:store_id;type:uuid;not null"`
	TenantID    uuid.UUID           `gorm:"column:tenant_id;type:uuid;not null"`
	Status      WhiteLabelAppStatus `gorm:"column:status;type:varchar(30);not null"`
	ScheduledAt *time.Time          `gorm:"column:scheduled_at"`
	Actor       string              `gorm:"column:actor;type:varchar(100);not null"`
	Reason      *string             `gorm:"column:reason"`
	CreatedAt   time.Time           `gorm:"column:created_at;not null;default:now()"`
}

// TableName returns the database table name for GORM.
func (WhiteLabelAppLifecycleEntry) TableName() string { return "white_label_app_lifecycle" }
