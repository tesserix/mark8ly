package campaign

import (
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"gorm.io/datatypes"
)

// Campaign status constants.
const (
	StatusDraft     = "draft"
	StatusScheduled = "scheduled"
	StatusSending   = "sending"
	StatusSent      = "sent"
	StatusPaused    = "paused"
	StatusCancelled = "cancelled"
)

// Campaign type constants.
const (
	TypeEmail = "email"
)

// Recipient status constants.
const (
	RecipientPending      = "pending"
	RecipientSent         = "sent"
	RecipientDelivered    = "delivered"
	RecipientOpened       = "opened"
	RecipientClicked      = "clicked"
	RecipientBounced      = "bounced"
	RecipientUnsubscribed = "unsubscribed"
)

// Analytics column constants — typed to catch typos at compile time.
const (
	AnalyticsDelivered    = "delivered"
	AnalyticsOpened       = "opened"
	AnalyticsClicked      = "clicked"
	AnalyticsConverted    = "converted"
	AnalyticsUnsubscribed = "unsubscribed"
	AnalyticsFailed       = "failed"
)

// CustomerSegment defines a reusable audience filter. Rules is a JSONB
// array of rule objects; the segment engine resolves them to email lists.
type CustomerSegment struct {
	ID          uuid.UUID      `gorm:"column:id;type:uuid;primaryKey;default:gen_random_uuid()"`
	TenantID    uuid.UUID      `gorm:"column:tenant_id;type:uuid;not null"`
	StoreID     uuid.UUID      `gorm:"column:store_id;type:uuid;not null"`
	Name        string         `gorm:"column:name;type:varchar(200);not null"`
	Description *string        `gorm:"column:description;type:text"`
	Rules       datatypes.JSON `gorm:"column:rules;type:jsonb;not null;default:'[]'::jsonb"`
	MemberCount int            `gorm:"column:member_count;type:int;not null;default:0"`
	CreatedAt   time.Time      `gorm:"column:created_at;not null;default:now()"`
	UpdatedAt   time.Time      `gorm:"column:updated_at;not null;default:now()"`
}

func (CustomerSegment) TableName() string { return "customer_segments" }

// Campaign is the root aggregate for email campaigns.
type Campaign struct {
	ID              uuid.UUID       `gorm:"column:id;type:uuid;primaryKey;default:gen_random_uuid()"`
	TenantID        uuid.UUID       `gorm:"column:tenant_id;type:uuid;not null"`
	StoreID         uuid.UUID       `gorm:"column:store_id;type:uuid;not null"`
	Name            string          `gorm:"column:name;type:varchar(200);not null"`
	Type            string          `gorm:"column:type;type:varchar(20);not null;default:email"`
	Status          string          `gorm:"column:status;type:varchar(20);not null;default:draft"`
	Subject         *string         `gorm:"column:subject;type:varchar(300)"`
	Content         *string         `gorm:"column:content;type:text"`
	SegmentID       *uuid.UUID      `gorm:"column:segment_id;type:uuid"`
	CouponID        *uuid.UUID      `gorm:"column:coupon_id;type:uuid"`
	ScheduledAt     *time.Time      `gorm:"column:scheduled_at"`
	SentAt          *time.Time      `gorm:"column:sent_at"`
	HeartbeatAt     *time.Time      `gorm:"column:heartbeat_at"`
	TotalRecipients int             `gorm:"column:total_recipients;not null;default:0"`
	Delivered       int             `gorm:"column:delivered;not null;default:0"`
	Opened          int             `gorm:"column:opened;not null;default:0"`
	Clicked         int             `gorm:"column:clicked;not null;default:0"`
	Converted       int             `gorm:"column:converted;not null;default:0"`
	Unsubscribed    int             `gorm:"column:unsubscribed;not null;default:0"`
	Failed          int             `gorm:"column:failed;not null;default:0"`
	Revenue         decimal.Decimal `gorm:"column:revenue;type:numeric(12,2);not null;default:0"`
	CreatedAt       time.Time       `gorm:"column:created_at;not null;default:now()"`
	UpdatedAt       time.Time       `gorm:"column:updated_at;not null;default:now()"`
}

func (Campaign) TableName() string { return "campaigns" }

// IsTerminal reports whether the campaign is in a final state.
func (c Campaign) IsTerminal() bool {
	return c.Status == StatusSent || c.Status == StatusCancelled
}

// CampaignRecipient tracks per-recipient delivery state.
type CampaignRecipient struct {
	ID            uuid.UUID  `gorm:"column:id;type:uuid;primaryKey;default:gen_random_uuid()"`
	TenantID      uuid.UUID  `gorm:"column:tenant_id;type:uuid;not null"`
	CampaignID    uuid.UUID  `gorm:"column:campaign_id;type:uuid;not null"`
	CustomerEmail string     `gorm:"column:customer_email;type:varchar(300);not null"`
	Status        string     `gorm:"column:status;type:varchar(20);not null;default:pending"`
	SentAt        *time.Time `gorm:"column:sent_at"`
	OpenedAt      *time.Time `gorm:"column:opened_at"`
	ClickedAt     *time.Time `gorm:"column:clicked_at"`
	CreatedAt     time.Time  `gorm:"column:created_at;not null;default:now()"`
}

func (CampaignRecipient) TableName() string { return "campaign_recipients" }

// SegmentRule is the Go representation of a single rule in the segments
// rules JSONB array. The segment engine interprets these.
//
// Supported rule types for M4:
//   - "loyalty_tier" — field: tier value (e.g., "gold", "silver")
//   - "has_ordered" — customers with at least one order in the store
//   - "inactive_days" — no order in N days (value is string int, e.g., "90")
//   - "all" — all customers in customer_loyalties for this store
type SegmentRule struct {
	Type  string `json:"type"`  // rule type
	Field string `json:"field"` // optional field qualifier
	Value string `json:"value"` // filter value
}
