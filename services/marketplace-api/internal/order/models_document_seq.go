package order

import (
	"time"

	"github.com/google/uuid"
)

// DocumentNumberSeq is the per-store per-day counter for orders AND returns.
//
// Never UPDATE-or-SELECT-FOR-UPDATE this table directly — always go through
// NextDocumentNumber(), which uses an atomic INSERT ... ON CONFLICT DO UPDATE
// ... RETURNING statement. The row lock is held for microseconds (one
// statement), NOT across the full create-order transaction.
//
// Kind is one of "order" or "return" (enforced by a DB CHECK constraint).
//
// There is deliberately no PendingEvent / outbox struct in this package.
// The shared outbox_events table (and its outbox.OutboxEvent GORM model)
// is owned by products; orders M2 writes to it via new aggregate and
// event_type constants.
type DocumentNumberSeq struct {
	StoreID uuid.UUID `gorm:"column:store_id;type:uuid;primaryKey"`
	Kind    string    `gorm:"column:kind;type:varchar(10);primaryKey"`
	Day     time.Time `gorm:"column:day;type:date;primaryKey"`
	LastSeq int       `gorm:"column:last_seq;type:integer;not null;default:0"`
}

func (DocumentNumberSeq) TableName() string { return "document_number_seq" }
