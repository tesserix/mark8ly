package customererasure

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

// Status values for customer_erasure_requests.status.
//
// The vocabulary is what migration 000113 widened the CHECK constraint to.
// #259 described 'processing'/'completed'/'failed' as already existing; they
// did not — the original constraint (000059) admitted only pending, processed
// and rejected, so there was no in-flight state and a crash mid-erasure was
// indistinguishable from a request nobody had started.
const (
	// StatusPending — filed by the storefront, nobody has acted on it.
	StatusPending = "pending"
	// StatusProcessing — a worker holds the claim. This is the state that
	// makes the claim a concurrency control rather than a label.
	StatusProcessing = "processing"
	// StatusCompleted — the erasure ran and the receipt is in notes.
	StatusCompleted = "completed"
	// StatusFailed — the erasure was attempted and rolled back. Retryable:
	// the claim below matches 'failed' as well as 'pending'.
	StatusFailed = "failed"
	// StatusRejected — an operator refused the request.
	StatusRejected = "rejected"
	// StatusProcessed is the pre-000113 terminal state, kept valid so
	// existing rows need no backfill. Nothing writes it any more.
	StatusProcessed = "processed"
)

// Request is one row of customer_erasure_requests.
//
// CustomerEmail is the subject's address and is the one field in this struct
// that is personal data. It is read to build the plan and must never reach a
// log line or a receipt.
type Request struct {
	ID            uuid.UUID  `gorm:"column:id"`
	TenantID      uuid.UUID  `gorm:"column:tenant_id"`
	StoreID       uuid.UUID  `gorm:"column:store_id"`
	CustomerEmail string     `gorm:"column:customer_email"`
	Status        string     `gorm:"column:status"`
	Attempts      int        `gorm:"column:attempts"`
	RequestedAt   time.Time  `gorm:"column:requested_at"`
	ProcessedAt   *time.Time `gorm:"column:processed_at"`
	Notes         *string    `gorm:"column:notes"`
}

// TableName pins the table; GORM's default pluralisation would guess
// "requests" from the struct name.
func (Request) TableName() string { return "customer_erasure_requests" }

// Receipt is the evidence the erasure happened, and the evidence of what it
// deliberately did NOT destroy.
//
// IT CONTAINS NO PERSONAL DATA. Table names, counts and the request id only —
// no email, name, phone or address. A receipt that named the subject would
// re-create, in the audit trail, exactly the record the erasure removed.
//
// RetainedTables is not decoration. "We erased it" is a claim that may need
// evidencing; "we retained the order rows, anonymised, under legal-obligation
// basis" is the half of the answer a regulator actually asks for.
type Receipt struct {
	RequestID uuid.UUID `json:"request_id"`
	// Deleted maps table -> rows destroyed.
	Deleted map[string]int64 `json:"deleted"`
	// Anonymised maps table -> rows whose personal fields were overwritten
	// while the row itself survived. gift_cards can exceed its row count:
	// one row holds the subject in up to three roles, each its own step.
	Anonymised map[string]int64 `json:"anonymised"`
	// RetainedTables lists every table whose rows survive this erasure —
	// the anonymised ones plus those that carry no personal column at all
	// and so needed no statement.
	RetainedTables []string `json:"retained_tables"`
	// RetentionBasis records WHY they survive, next to the fact that they do.
	RetentionBasis string    `json:"retention_basis"`
	CompletedAt    time.Time `json:"completed_at"`
}

// RetentionBasis is the justification recorded in every receipt, and the same
// one migration 000113 wrote onto the tables themselves.
const RetentionBasis = "financial record; anonymised and retained 7 years under legal-obligation basis (§23.2), matching billing_archive"

// retainedWithoutStep are tables whose rows survive an erasure and that carry
// NO statement in the plan, because they hold no personal column at all —
// verified column-by-column against the live schema, not assumed. They are
// named in the receipt anyway: a table absent from both halves of a receipt
// is indistinguishable, to a reader, from one the plan never considered.
//
// payment_transactions.metadata (JSONB) is the one blob among them, and it
// is a dead column — nothing writes it — as #435 established. shipments used
// to be listed here; it now carries its own step, because its ship_to /
// ship_from blobs do hold the subject.
var retainedWithoutStep = []string{
	"order_items",
	"payment_transactions",
	"platform_fee_ledger",
	"refund_transactions",
}

// ErrRequestNotFound — no such erasure request.
var ErrRequestNotFound = errors.New("customererasure: no such erasure request")

// ErrAlreadyClaimed — the row is not available to this worker: another
// process holds it in 'processing', or an operator already rejected it.
//
// It is NOT an error state for the request. A caller that races another
// worker has nothing to fix; the inbox executor maps this to a 409.
var ErrAlreadyClaimed = errors.New("customererasure: erasure request is already claimed or decided")
