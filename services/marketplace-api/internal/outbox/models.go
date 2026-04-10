// Package outbox holds the OutboxEvent model. Events are written in the
// same transaction as the mutation that produces them (see spec §13.2.7).
// Slice 1's publisher reads these rows, upserts store_watermarks, and
// marks them published. Slice 2 adds real Pub/Sub delivery.
package outbox

import (
	"time"

	"gorm.io/datatypes"
)

// OutboxEvent represents one pending or published domain event.
// The index on (tenant_id, created_at) WHERE published_at IS NULL
// supports the publisher's SELECT … FOR UPDATE SKIP LOCKED pattern
// (spec §14.6).
type OutboxEvent struct {
	ID          string         `gorm:"primaryKey;column:id;type:uuid;default:gen_random_uuid()" json:"id"`
	TenantID    string         `gorm:"column:tenant_id;type:uuid;not null"                      json:"tenant_id"`
	Aggregate   string         `gorm:"column:aggregate;type:varchar(64);not null"               json:"aggregate"`
	AggregateID string         `gorm:"column:aggregate_id;type:uuid;not null"                   json:"aggregate_id"`
	EventType   string         `gorm:"column:event_type;type:varchar(64);not null"              json:"event_type"`
	Payload     datatypes.JSON `gorm:"column:payload;type:jsonb;not null"                       json:"payload"`
	CreatedAt   time.Time      `gorm:"column:created_at;not null;default:now()"                 json:"created_at"`
	PublishedAt *time.Time     `gorm:"column:published_at"                                      json:"published_at,omitempty"`
	Error       *string        `gorm:"column:error;type:text"                                   json:"error,omitempty"`
}

func (OutboxEvent) TableName() string { return "outbox_events" }

// Aggregate constants used by producers.
const (
	AggregateProduct        = "product"
	AggregateCategory       = "category"
	AggregateMedia          = "media"
	AggregateOrder          = "order"
	AggregateReturn         = "return"
	AggregateAbandonedCart  = "abandoned_cart"
)

// EventType constants.
const (
	EventProductCreated  = "product.created"
	EventProductUpdated  = "product.updated"
	EventProductDeleted  = "product.deleted"
	EventCategoryCreated = "category.created"
	EventCategoryUpdated = "category.updated"
	EventCategoryDeleted = "category.deleted"

	// Orders slice 1.
	EventOrderPlaced               = "order.placed"
	EventOrderConfirmed            = "order.confirmed"
	EventOrderFulfilled            = "order.fulfilled"
	EventOrderCancelled            = "order.cancelled"
	EventOrderRefunded             = "order.refunded"
	EventReturnRequested           = "return.requested"
	EventReturnApproved            = "return.approved"
	EventReturnReceived            = "return.received"
	EventReturnRefunded            = "return.refunded"
	EventReturnRejected            = "return.rejected"
	EventAbandonedCartRecoveryEmail = "abandoned_cart.recovery_email"
)

// IsOrderAggregate reports whether an aggregate string belongs to the orders
// domain. The watermark publisher uses this to decide which store_watermarks
// column to bump (orders_updated_at vs products_updated_at).
func IsOrderAggregate(a string) bool {
	switch a {
	case AggregateOrder, AggregateReturn, AggregateAbandonedCart:
		return true
	}
	return false
}
