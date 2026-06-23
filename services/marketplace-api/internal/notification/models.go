// Package notification implements notification CRUD and preference
// management for Settings S5.
package notification

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// NotificationType enumerates the kinds of notifications.
type NotificationType string

const (
	TypeNewOrder        NotificationType = "new_order"
	TypeLowStock        NotificationType = "low_stock"
	TypeReturnRequested NotificationType = "return_requested"
	TypePaymentReceived NotificationType = "payment_received"
	TypeReviewSubmitted NotificationType = "review_submitted"
	TypeOrderCancelled  NotificationType = "order_cancelled"
	TypeOrderFulfilled  NotificationType = "order_fulfilled"
	TypeSystemAlert     NotificationType = "system_alert"

	// Customer-facing types (recipient_user_id set) — power the storefront
	// customer notification bell.
	TypeOrderPlaced     NotificationType = "order_placed"
	TypeOrderShipped    NotificationType = "order_shipped"
	TypeOrderDelivered  NotificationType = "order_delivered"
	TypeRefundProcessed NotificationType = "refund_processed"
)

// Notification is the GORM model for the notifications table.
type Notification struct {
	ID           uuid.UUID        `gorm:"column:id;type:uuid;primaryKey;default:gen_random_uuid()"`
	TenantID     uuid.UUID        `gorm:"column:tenant_id;type:uuid;not null"`
	StoreID      uuid.UUID        `gorm:"column:store_id;type:uuid;not null"`
	// RecipientUserID, when set, targets a storefront customer (GIP string id)
	// for the customer notification bell. NULL = store/staff notification.
	RecipientUserID *string        `gorm:"column:recipient_user_id;type:varchar(255)"`
	Type         NotificationType `gorm:"column:type;type:varchar(40);not null"`
	Title        string           `gorm:"column:title;type:varchar(200);not null"`
	Message      *string          `gorm:"column:message;type:text"`
	ResourceType *string          `gorm:"column:resource_type;type:varchar(40)"`
	ResourceID   *uuid.UUID       `gorm:"column:resource_id;type:uuid"`
	IsRead       bool             `gorm:"column:is_read;type:boolean;not null;default:false"`
	CreatedAt    time.Time        `gorm:"column:created_at;not null;default:now()"`
}

func (Notification) TableName() string { return "notifications" }

// NotificationPreferences is the GORM model for the notification_preferences table.
type NotificationPreferences struct {
	ID          uuid.UUID       `gorm:"column:id;type:uuid;primaryKey;default:gen_random_uuid()"`
	TenantID    uuid.UUID       `gorm:"column:tenant_id;type:uuid;not null"`
	StoreID     uuid.UUID       `gorm:"column:store_id;type:uuid;not null;uniqueIndex"`
	Preferences json.RawMessage `gorm:"column:preferences;type:jsonb;not null;default:'{\"new_order\": true, \"low_stock\": true, \"return_requested\": true, \"payment_received\": true, \"review_submitted\": true}'"`
	CreatedAt   time.Time       `gorm:"column:created_at;not null;default:now()"`
	UpdatedAt   time.Time       `gorm:"column:updated_at;not null;default:now()"`
}

func (NotificationPreferences) TableName() string { return "notification_preferences" }
