// Package outbox holds the OutboxEvent model. Events are written in the
// same transaction as the mutation that produces them (see spec §13.2.7).
// Slice 1's publisher reads these rows, upserts store_watermarks, and marks
// them published. Slice 2 adds real Pub/Sub delivery.
//
// A row is in one of FOUR states, all derived from existing columns.
// Precedence, most to least authoritative:
//
//	published      published_at IS NOT NULL
//	dead_lettered  dead_lettered_at IS NOT NULL                                          (terminal, reversible)
//	failed         published_at IS NULL AND dead_lettered_at IS NULL AND error IS NOT NULL (terminal)
//	pending        published_at IS NULL AND dead_lettered_at IS NULL AND error IS NULL
//
// published is checked FIRST: a row carrying both published_at and error (or
// dead_lettered_at) was delivered, whatever stale error or dead-letter mark
// it still carries — see TestIntegration_ListPlatform_PublishedWinsOverError.
//
// failed is terminal: ProcessBatch's poll excludes it (error IS NULL), so it
// is never retried. dead_lettered is ALSO terminal, and excluded from the
// poll explicitly (dead_lettered_at IS NULL) rather than by relying on error
// also being set — a dead-letter written with a NULL error must not be
// silently re-picked up and delivered (#405). Requeueing (internal/outbox/
// dead_letter.go) clears error, dead_lettered_at and dead_letter_reason
// together and re-enters the row into the poll, but that alone is NOT
// recovery: the watermark upsert is monotonic (GREATEST) over the row's
// created_at, so a stale row would republish without moving the watermark —
// no consumer learns, and the health alarm clears. That is why requeue also
// bumps created_at to now(): it is what makes the watermark actually move.
// See #336, #405.
//
// The single most important invariant in the write path (#405): requeue and
// dead-letter both REFUSE any row whose published_at is non-nil. Clearing
// error (or setting dead_lettered_at) on an already-published row would hand
// it back to the publisher and cause a double-publish — turning a delivery
// failure into a data-corruption problem. See dead_letter.go's lockForWrite.
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
	// DeadLetteredAt and DeadLetterReason are set by an operator decision
	// (#405), never by the publisher. See the package doc above and
	// dead_letter.go.
	DeadLetteredAt   *time.Time `gorm:"column:dead_lettered_at"                                  json:"dead_lettered_at,omitempty"`
	DeadLetterReason *string    `gorm:"column:dead_letter_reason;type:text"                      json:"dead_letter_reason,omitempty"`
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
	EventOrderPartiallyFulfilled    = "order.partially_fulfilled"
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
	ReasonPayloadUnparseable = "payload_unparseable"
	// ReasonPayloadMissingStoreID is written when a payload's store_id is
	// absent, not a string, an empty string, or a non-empty string that does
	// not parse as a UUID. All four are the same terminal producer bug and
	// require the same operator action, so they share one code rather than
	// widening this closed vocabulary. The UUID case is rejected here, in
	// the row loop, rather than left to the store pre-check below: stores.id
	// is uuid, and passing a non-UUID value to that SELECT raises `invalid
	// input syntax for type uuid`, which ABORTS the transaction and rolls
	// back the whole batch — see #374.
	ReasonPayloadMissingStoreID = "payload_missing_store_id"
	// ReasonStoreNotFound is written when a payload's store_id is
	// well-formed but has no matching row in `stores`. The watermark upsert
	// would raise an FK violation (store_watermarks.store_id REFERENCES
	// stores(id)), which ABORTS the whole transaction rather than failing
	// one row — see #374. Permanent in practice: stores are removed only by
	// tenant purge and hard-delete, both of which sweep this tenant's
	// outbox_events too.
	ReasonStoreNotFound = "store_not_found"
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
