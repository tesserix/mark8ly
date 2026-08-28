package campaignbudget

import (
	"time"

	"github.com/google/uuid"
)

// CampaignEmailBudget is the GORM model for the campaign_email_budget table.
// It tracks per-store, per-month email send quotas. The composite primary key
// (store_id, month) ensures one budget row per store per calendar month.
// month is stored as a date (first day of month) in Postgres.
type CampaignEmailBudget struct {
	StoreID         uuid.UUID `gorm:"column:store_id;type:uuid;primaryKey"`
	Month           time.Time `gorm:"column:month;type:date;primaryKey"`
	Remaining       int       `gorm:"column:remaining;not null"`
	LimitSet        int       `gorm:"column:limit_set;not null"`
	UpdatedAt       time.Time `gorm:"column:updated_at;not null;default:now()"`
	RampStepApplied int16     `gorm:"column:ramp_step_applied;not null;default:0"`
}

// TableName returns the database table name for GORM.
func (CampaignEmailBudget) TableName() string { return "campaign_email_budget" }
