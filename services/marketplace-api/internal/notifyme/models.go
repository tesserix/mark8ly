// Package notifyme provides the domain model and repository for product
// notification subscriptions (back_in_stock, price_drop).
package notifyme

import (
	"time"

	"github.com/google/uuid"
)

// Notify type constants match the CHECK on product_notify_subscriptions.notify_type.
const (
	TypeBackInStock = "back_in_stock"
	TypePriceDrop   = "price_drop"
)

// ProductNotifySubscription maps to the product_notify_subscriptions table.
type ProductNotifySubscription struct {
	ID         uuid.UUID `gorm:"column:id;type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	TenantID   uuid.UUID `gorm:"column:tenant_id;type:uuid;not null" json:"tenant_id"`
	StoreSlug  string    `gorm:"column:store_slug;type:varchar(63);not null" json:"store_slug"`
	CustomerID uuid.UUID `gorm:"column:customer_id;type:uuid;not null" json:"customer_id"`
	ProductID  uuid.UUID `gorm:"column:product_id;type:uuid;not null" json:"product_id"`
	NotifyType string    `gorm:"column:notify_type;type:varchar(20);not null" json:"notify_type"`
	CreatedAt  time.Time `gorm:"column:created_at;not null;default:now()" json:"created_at"`
}

func (ProductNotifySubscription) TableName() string { return "product_notify_subscriptions" }

// SubscribeInput is the JSON binding struct for creating a subscription.
type SubscribeInput struct {
	ProductID  string `json:"product_id" binding:"required,uuid"`
	NotifyType string `json:"notify_type" binding:"required,oneof=back_in_stock price_drop"`
}
