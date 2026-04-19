// Package seaqueue persists the SEA (MY/TH/PH/ID/VN) manual-review queue.
// Queue entry pauses the validation clock immediately per §5.2.
package seaqueue

import (
	"time"

	"github.com/google/uuid"
)

// Status enumerates queue lifecycle states. Only pending and in_review block
// the merchant's clock; approved/rejected resolve and the orchestrator wakes up.
type Status string

const (
	StatusPending  Status = "pending"
	StatusInReview Status = "in_review"
	StatusApproved Status = "approved"
	StatusRejected Status = "rejected"
)

// Entry is the GORM model for the sea_manual_review_queue table (migration 065).
type Entry struct {
	ID            uuid.UUID  `gorm:"column:id;type:uuid;primaryKey;default:gen_random_uuid()"`
	TenantID      uuid.UUID  `gorm:"column:tenant_id;type:uuid;not null"`
	StoreID       uuid.UUID  `gorm:"column:store_id;type:uuid;not null"`
	Country       string     `gorm:"column:country;type:char(2);not null"`
	TaxID         string     `gorm:"column:tax_id;type:varchar(50);not null"`
	BusinessName  string     `gorm:"column:business_name"`
	QueueReason   string     `gorm:"column:queue_reason;type:varchar(50);not null"`
	Status        Status     `gorm:"column:status;type:varchar(20);not null;default:pending"`
	ReviewerID    *uuid.UUID `gorm:"column:reviewer_id;type:uuid"`
	ReviewerNotes string     `gorm:"column:reviewer_notes"`
	SLADueAt      time.Time  `gorm:"column:sla_due_at;not null"`
	QueuedAt      time.Time  `gorm:"column:queued_at;not null;default:now()"`
	ResolvedAt    *time.Time `gorm:"column:resolved_at"`
}

// TableName binds the GORM model to the migration-defined table.
func (Entry) TableName() string { return "sea_manual_review_queue" }
