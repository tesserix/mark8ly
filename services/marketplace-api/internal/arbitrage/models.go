package arbitrage

import (
	"time"

	"github.com/google/uuid"
)

// Resolution enumerates the outcomes of an arbitrage audit review.
type Resolution string

const (
	ResolutionOngoing              Resolution = "ongoing"
	ResolutionFalsePositiveCleared Resolution = "false_positive_cleared"
	ResolutionRepriceDeveloped     Resolution = "reprice_developed"
	ResolutionTerminated           Resolution = "terminated"
)

// SubscriptionArbitrageAudit is the GORM model for the
// subscription_arbitrage_audit table. It records every case where the
// platform detects a mismatch between a merchant's card country, billing
// address, and IP geolocation — the signals used to detect PPP price abuse.
// Raw IP addresses are never stored; only a hashed value (ip_hash) is kept.
type SubscriptionArbitrageAudit struct {
	ID                uuid.UUID  `gorm:"column:id;type:uuid;primaryKey;default:gen_random_uuid()"`
	SubscriptionID    uuid.UUID  `gorm:"column:subscription_id;type:uuid;not null"`
	TenantID          uuid.UUID  `gorm:"column:tenant_id;type:uuid;not null"`
	StoreID           uuid.UUID  `gorm:"column:store_id;type:uuid;not null"`
	CardCountry       *string    `gorm:"column:card_country;type:char(2)"`
	BillingCountry    *string    `gorm:"column:billing_country;type:char(2)"`
	IPCountry         *string    `gorm:"column:ip_country;type:char(2)"`
	IPHash            *string    `gorm:"column:ip_hash;type:varchar(64)"`
	ResolvedPriceTier string     `gorm:"column:resolved_price_tier;type:varchar(20);not null"`
	MismatchReason    *string    `gorm:"column:mismatch_reason;type:varchar(100)"`
	FlaggedAt         time.Time  `gorm:"column:flagged_at;not null;default:now()"`
	ReviewedBy        *uuid.UUID `gorm:"column:reviewed_by;type:uuid"`
	ReviewedAt        *time.Time `gorm:"column:reviewed_at"`
	Resolution        Resolution `gorm:"column:resolution;type:varchar(30);not null;default:ongoing"`
	CreatedAt         time.Time  `gorm:"column:created_at;not null;default:now()"`
}

// TableName returns the database table name for GORM.
func (SubscriptionArbitrageAudit) TableName() string { return "subscription_arbitrage_audit" }
