// Package refund implements the 14-day cooling-off subscription refund flow (§8).
// Stripe is the backend of record for the actual refund; refund_audit is the
// local fraud-guard and compliance trail.
package refund

import (
	"time"

	"github.com/google/uuid"
)

// RefundAudit is the GORM model for the refund_audit table (migration 062).
// One row per issued subscription refund. The unique partial index on
// card_fingerprint (where not null) is the fraud guard preventing refunds
// to the same card across different subscriptions (§8 pattern D).
type RefundAudit struct {
	ID             uuid.UUID `gorm:"column:id;type:uuid;primaryKey;default:gen_random_uuid()"`
	SubscriptionID uuid.UUID `gorm:"column:subscription_id;type:uuid;not null"`
	StoreID        uuid.UUID `gorm:"column:store_id;type:uuid;not null"`
	TenantID       uuid.UUID `gorm:"column:tenant_id;type:uuid;not null"`
	StripeRefundID string    `gorm:"column:stripe_refund_id;type:varchar(100);not null"`
	StripeChargeID string    `gorm:"column:stripe_charge_id;type:varchar(100);not null"`
	AmountMinor    int64     `gorm:"column:amount_minor;not null"`
	Currency       string    `gorm:"column:currency;type:char(3);not null"`
	Reason         string    `gorm:"column:reason;not null"`
	// CardFingerprint: Stripe charge.payment_method_details.card.fingerprint.
	// NULL when card details aren't available (e.g. bank transfer).
	CardFingerprint   *string   `gorm:"column:card_fingerprint;type:varchar(100)"`
	DeviceFingerprint *string   `gorm:"column:device_fingerprint;type:varchar(200)"`
	IssuedBy          string    `gorm:"column:issued_by;not null"`
	IssuedAt          time.Time `gorm:"column:issued_at;not null;default:now()"`
}

// TableName returns the database table name for GORM.
func (RefundAudit) TableName() string { return "refund_audit" }
