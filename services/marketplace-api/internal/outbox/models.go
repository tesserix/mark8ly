// Package outbox holds the OutboxEvent model. Events are written in the
// same transaction as the mutation that produces them (see spec §13.2.7).
// Slice 1's publisher reads these rows, upserts store_watermarks, and marks
// them published. Slice 2 adds real Pub/Sub delivery.
//
// A row is in one of three states, all derived from existing columns:
//
//	pending    published_at IS NULL AND error IS NULL
//	failed     published_at IS NULL AND error IS NOT NULL   (terminal)
//	published  published_at IS NOT NULL
//
// failed is terminal: ProcessBatch's poll excludes it, so it is never
// retried. Clearing error requeues the row. See #336.
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
	AggregateProduct       = "product"
	AggregateCategory      = "category"
	AggregateMedia         = "media"
	AggregateOrder         = "order"
	AggregateReturn        = "return"
	AggregateAbandonedCart = "abandoned_cart"
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
	EventOrderPlaced                = "order.placed"
	EventOrderConfirmed             = "order.confirmed"
	EventOrderFulfilled             = "order.fulfilled"
	EventOrderCancelled             = "order.cancelled"
	EventOrderRefunded              = "order.refunded"
	EventReturnRequested            = "return.requested"
	EventReturnApproved             = "return.approved"
	EventReturnReceived             = "return.received"
	EventReturnRefunded             = "return.refunded"
	EventReturnRejected             = "return.rejected"
	EventAbandonedCartRecoveryEmail = "abandoned_cart.recovery_email"
)

// Failure reason codes written to outbox_events.error when the publisher
// cannot process a row. This vocabulary is CLOSED and the values are
// STABLE: #331 serves this column cross-tenant to the platform console,
// and a stable code is what lets the console render it.
//
// A raw error string must NEVER be stored here. encoding/json quotes the
// offending input in its unmarshal errors, so persisting err.Error() would
// copy fragments of an arbitrary customer-data JSONB payload into a column
// that leaves this service — defeating the same reasoning that keeps
// `payload` out of #331's response, through a field nobody would audit.
const (
	ReasonPayloadUnparseable    = "payload_unparseable"
	ReasonPayloadMissingStoreID = "payload_missing_store_id"
	// ReasonUnknown is written when a caller supplies a reason outside this
	// vocabulary. It exists so MarkFailedInTx can neutralise an unrecognised
	// string WITHOUT failing the batch: returning an error there would roll
	// back the publisher's transaction, leaving the offending rows pending
	// and re-selected forever — the exact poison pill this work removes.
	ReasonUnknown = "unknown"
)

// Failure is one row the publisher could not process, paired with the
// reason code to persist. See MarkFailedInTx.
type Failure struct {
	ID     string
	Reason string
}

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
