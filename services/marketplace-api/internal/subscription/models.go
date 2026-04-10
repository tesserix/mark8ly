// Package subscription implements store billing subscription management
// for Settings S3.
package subscription

import (
	"time"

	"github.com/google/uuid"
)

// SubscriptionPlan enumerates the available billing plans.
type SubscriptionPlan string

const (
	PlanFree       SubscriptionPlan = "free"
	PlanStarter    SubscriptionPlan = "starter"
	PlanPro        SubscriptionPlan = "pro"
	PlanEnterprise SubscriptionPlan = "enterprise"
)

// SubscriptionStatus enumerates subscription lifecycle states.
type SubscriptionStatus string

const (
	StatusActive     SubscriptionStatus = "active"
	StatusTrialing   SubscriptionStatus = "trialing"
	StatusPastDue    SubscriptionStatus = "past_due"
	StatusCancelled  SubscriptionStatus = "cancelled"
	StatusIncomplete SubscriptionStatus = "incomplete"
)

// StoreSubscription is the GORM model for the store_subscriptions table.
type StoreSubscription struct {
	ID                   uuid.UUID          `gorm:"column:id;type:uuid;primaryKey;default:gen_random_uuid()"`
	TenantID             uuid.UUID          `gorm:"column:tenant_id;type:uuid;not null"`
	StoreID              uuid.UUID          `gorm:"column:store_id;type:uuid;not null;uniqueIndex"`
	StripeCustomerID     string             `gorm:"column:stripe_customer_id;type:varchar(100);not null"`
	StripeSubscriptionID *string            `gorm:"column:stripe_subscription_id;type:varchar(100)"`
	Plan                 SubscriptionPlan   `gorm:"column:plan;type:varchar(30);not null;default:free"`
	Status               SubscriptionStatus `gorm:"column:status;type:varchar(30);not null;default:active"`
	CurrentPeriodStart   *time.Time         `gorm:"column:current_period_start"`
	CurrentPeriodEnd     *time.Time         `gorm:"column:current_period_end"`
	CancelAtPeriodEnd    bool               `gorm:"column:cancel_at_period_end;not null;default:false"`
	CreatedAt            time.Time          `gorm:"column:created_at;not null;default:now()"`
	UpdatedAt            time.Time          `gorm:"column:updated_at;not null;default:now()"`
}

func (StoreSubscription) TableName() string { return "store_subscriptions" }
