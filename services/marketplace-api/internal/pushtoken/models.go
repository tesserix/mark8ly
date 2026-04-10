// Package pushtoken provides the domain model and repository for storefront
// push notification tokens. Separate from the admin push.Token which tracks
// admin-user tokens — these are customer-facing device registrations.
package pushtoken

import (
	"time"

	"github.com/google/uuid"
)

// StorefrontPushToken maps to the storefront_push_tokens table.
type StorefrontPushToken struct {
	ID         uuid.UUID `gorm:"column:id;type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	TenantID   uuid.UUID `gorm:"column:tenant_id;type:uuid;not null" json:"tenant_id"`
	StoreSlug  string    `gorm:"column:store_slug;type:varchar(63);not null" json:"store_slug"`
	CustomerID uuid.UUID `gorm:"column:customer_id;type:uuid;not null" json:"customer_id"`
	DeviceID   string    `gorm:"column:device_id;type:varchar(255);not null" json:"device_id"`
	Token      string    `gorm:"column:token;type:text;not null" json:"token"`
	Platform   string    `gorm:"column:platform;type:varchar(10);not null" json:"platform"`
	CreatedAt  time.Time `gorm:"column:created_at;not null;default:now()" json:"created_at"`
	UpdatedAt  time.Time `gorm:"column:updated_at;not null;default:now()" json:"updated_at"`
}

func (StorefrontPushToken) TableName() string { return "storefront_push_tokens" }

// RegisterTokenInput is the JSON binding struct for push token registration.
type RegisterTokenInput struct {
	Token    string `json:"token" binding:"required"`
	DeviceID string `json:"device_id" binding:"required,max=255"`
	Platform string `json:"platform" binding:"required,oneof=ios android"`
}
