// Package webhook implements merchant-facing outbound webhooks (#562).
//
// It is a CONSUMER of internal/outbox, not a producer: outbox_events already
// records every domain event transactionally. See
// docs/superpowers/specs/2026-09-02-outbound-webhooks-design.md.
package webhook

import (
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
)

// MaxEventTypes bounds how many event types one subscription may select.
// There are 18 today; the cap exists so a malformed request cannot store an
// unbounded array.
const MaxEventTypes = 32

// Delivery statuses.
const (
	StatusPending   = "pending"
	StatusDelivered = "delivered"
	StatusFailed    = "failed"
)

type Subscription struct {
	ID         uuid.UUID      `gorm:"column:id;type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	TenantID   uuid.UUID      `gorm:"column:tenant_id;type:uuid;not null"                      json:"-"`
	StoreID    uuid.UUID      `gorm:"column:store_id;type:uuid;not null"                       json:"store_id"`
	URL        string         `gorm:"column:url;not null"                                      json:"url"`
	EventTypes pq.StringArray `gorm:"column:event_types;type:text[];not null"                   json:"event_types"`
	// Secret never leaves the server after creation — the handler returns it
	// once in its own response field and this tag keeps it out of every
	// subsequent read.
	Secret              string     `gorm:"column:secret;not null"                                   json:"-"`
	Enabled             bool       `gorm:"column:enabled;not null;default:true"                     json:"enabled"`
	DisabledReason      *string    `gorm:"column:disabled_reason"                                   json:"disabled_reason,omitempty"`
	DisabledAt          *time.Time `gorm:"column:disabled_at"                                       json:"disabled_at,omitempty"`
	ConsecutiveFailures int        `gorm:"column:consecutive_failures;not null;default:0"           json:"-"`
	CreatedAt           time.Time  `gorm:"column:created_at;not null;default:now()"                 json:"created_at"`
	UpdatedAt           time.Time  `gorm:"column:updated_at;not null;default:now()"                 json:"updated_at"`
}

func (Subscription) TableName() string { return "webhook_subscriptions" }

type Delivery struct {
	ID             uuid.UUID  `gorm:"column:id;type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	SubscriptionID uuid.UUID  `gorm:"column:subscription_id;type:uuid;not null"                json:"subscription_id"`
	OutboxEventID  uuid.UUID  `gorm:"column:outbox_event_id;type:uuid;not null"                json:"-"`
	EventType      string     `gorm:"column:event_type;not null"                               json:"event_type"`
	AggregateID    uuid.UUID  `gorm:"column:aggregate_id;type:uuid;not null"                   json:"aggregate_id"`
	Status         string     `gorm:"column:status;not null;default:pending"                   json:"status"`
	Attempts       int        `gorm:"column:attempts;not null;default:0"                       json:"attempts"`
	NextAttemptAt  time.Time  `gorm:"column:next_attempt_at;not null;default:now()"            json:"next_attempt_at"`
	LastStatusCode *int       `gorm:"column:last_status_code"                                  json:"last_status_code,omitempty"`
	LastError      *string    `gorm:"column:last_error"                                        json:"last_error,omitempty"`
	DeliveredAt    *time.Time `gorm:"column:delivered_at"                                      json:"delivered_at,omitempty"`
	CreatedAt      time.Time  `gorm:"column:created_at;not null;default:now()"                 json:"created_at"`
}

func (Delivery) TableName() string { return "webhook_deliveries" }
