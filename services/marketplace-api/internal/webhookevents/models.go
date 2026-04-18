package webhookevents

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
)

// StripeWebhookEvent is the GORM model for the stripe_webhook_events table.
// The primary key (event_id) doubles as the idempotency key — inserting the
// same Stripe event_id twice is a no-op via ON CONFLICT DO NOTHING.
type StripeWebhookEvent struct {
	EventID              string         `gorm:"column:event_id;type:varchar(100);primaryKey"`
	EventType            string         `gorm:"column:event_type;type:varchar(100);not null"`
	StoreID              *uuid.UUID     `gorm:"column:store_id;type:uuid"`
	TenantID             *uuid.UUID     `gorm:"column:tenant_id;type:uuid"`
	Payload              datatypes.JSON `gorm:"column:payload;type:jsonb;not null"`
	ReceivedAt           time.Time      `gorm:"column:received_at;not null;default:now()"`
	ProcessedAt          *time.Time     `gorm:"column:processed_at"`
	ProcessingError      *string        `gorm:"column:processing_error"`
	RetryCount           int            `gorm:"column:retry_count;not null;default:0"`
	ManualReviewRequired bool           `gorm:"column:manual_review_required;not null;default:false"`
}

// TableName returns the database table name for GORM.
func (StripeWebhookEvent) TableName() string { return "stripe_webhook_events" }
