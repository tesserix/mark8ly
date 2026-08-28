package customererasure

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/subscription"
)

// StepError is what a failed statement reports.
//
// IT DELIBERATELY DISCARDS THE DRIVER'S MESSAGE. A Postgres constraint or
// type error embeds the offending VALUE — here, the subject's email address —
// and this error is written into the request's notes column and handed to a
// logger. Carrying the driver text would put the address back into the audit
// trail the erasure exists to clear. The SQLSTATE is the actionable half and
// is PII-free, so that is what survives.
type StepError struct {
	Table       string
	Disposition Disposition
	// SQLState is the Postgres error code, or "" when the failure did not
	// come from Postgres (a cancelled context, say).
	SQLState string
}

func (e *StepError) Error() string {
	if e.SQLState == "" {
		return fmt.Sprintf("customererasure: %s step on %s failed", e.Disposition, e.Table)
	}
	return fmt.Sprintf("customererasure: %s step on %s failed with SQLSTATE %s", e.Disposition, e.Table, e.SQLState)
}

// newStepError sanitises a driver error into a StepError.
func newStepError(step Step, err error) *StepError {
	se := &StepError{Table: step.Table, Disposition: step.Disposition}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		se.SQLState = pgErr.Code
	}
	return se
}

// Executor takes a customer_erasure_requests row from pending to completed.
type Executor struct {
	db     *gorm.DB
	logger *slog.Logger
}

// NewExecutor refuses a nil db at construction rather than deferring the
// panic to the first erasure — the pattern #318 established after an audit
// emitter with a nil Repo killed the process at request time instead of at
// wiring time. logger may be nil; the default logger is used.
func NewExecutor(db *gorm.DB, logger *slog.Logger) (*Executor, error) {
	if db == nil {
		return nil, errors.New("customererasure: db must not be nil")
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Executor{db: db, logger: logger}, nil
}

// Process erases one request's subject.
//
// # The claim is the concurrency control
//
// Process begins by moving the row to 'processing' with a conditional UPDATE
// on the POOLED handle, outside the erasure transaction. Matching zero rows
// means somebody else has it, or it is already decided. That write commits
// immediately and on purpose: `attempts` must survive a rollback, or a
// request that fails deterministically would be retried forever with its
// counter reset each time.
//
// # Idempotency
//
// Re-processing a completed request is not an error and does not erase
// anything twice: the stored receipt is parsed back out of notes and
// returned. A caller that retries after a timeout gets the same answer, with
// the same counts, rather than a second destructive pass whose counts would
// all be zero and would read as "nothing was there".
//
// # The failure write is NOT in the transaction
//
// On failure the erasure transaction rolls back, and only THEN is 'failed'
// written, on the pooled handle, under context.WithoutCancel. Writing it
// inside would roll it back along with the erasure and leave the row stuck in
// 'processing' with no record of why — the #397 defect exactly. WithoutCancel
// is what stops a client disconnect from losing the same row.
func (e *Executor) Process(ctx context.Context, requestID uuid.UUID) (Receipt, error) {
	req, claimErr := e.claim(ctx, requestID)
	if claimErr != nil {
		replay, err := e.replayIfCompleted(ctx, requestID, claimErr)
		if replay != nil {
			return *replay, nil
		}
		return Receipt{}, err
	}

	receipt, runErr := e.run(ctx, req)
	if runErr != nil {
		e.markFailed(ctx, req, runErr)
		return Receipt{}, runErr
	}
	return receipt, nil
}

// claim moves the row to 'processing' and returns it. 'failed' is claimable
// as well as 'pending': a rolled-back attempt left nothing behind, so
// retrying it is safe.
func (e *Executor) claim(ctx context.Context, requestID uuid.UUID) (Request, error) {
	var req Request
	res := e.db.WithContext(ctx).Raw(`
		UPDATE customer_erasure_requests
		   SET status = ?, attempts = attempts + 1
		 WHERE id = ? AND status IN (?, ?)
		RETURNING id, tenant_id, store_id, customer_email, status, attempts, requested_at, processed_at, notes`,
		StatusProcessing, requestID, StatusPending, StatusFailed,
	).Scan(&req)
	if res.Error != nil {
		return Request{}, fmt.Errorf("customererasure: claim request %s: %w", requestID, res.Error)
	}
	if res.RowsAffected == 0 {
		return Request{}, ErrAlreadyClaimed
	}
	return req, nil
}

// replayIfCompleted turns a lost claim on an already-completed request into
// the receipt that request already produced. It returns a nil receipt (and
// the error to surface) in every other case.
func (e *Executor) replayIfCompleted(ctx context.Context, requestID uuid.UUID, claimErr error) (*Receipt, error) {
	if !errors.Is(claimErr, ErrAlreadyClaimed) {
		return nil, claimErr
	}

	var existing Request
	res := e.db.WithContext(ctx).
		Table("customer_erasure_requests").
		Where("id = ?", requestID).
		Take(&existing)
	if errors.Is(res.Error, gorm.ErrRecordNotFound) {
		return nil, ErrRequestNotFound
	}
	if res.Error != nil {
		return nil, fmt.Errorf("customererasure: read request %s: %w", requestID, res.Error)
	}
	if existing.Status != StatusCompleted {
		return nil, ErrAlreadyClaimed
	}

	var receipt Receipt
	if existing.Notes == nil || json.Unmarshal([]byte(*existing.Notes), &receipt) != nil {
		// Completed but with an unreadable receipt. Report the completion
		// honestly rather than inventing counts: the erasure DID happen, and
		// claiming zero rows were touched would be a false record.
		receipt = Receipt{RequestID: requestID}
	}
	return &receipt, nil
}

// run executes the plan and, on success, writes the receipt — both in the
// SAME transaction, under the store's advisory lock. The receipt commits with
// the erasure it describes, so there is no window in which the data is gone
// and the evidence is not.
func (e *Executor) run(ctx context.Context, req Request) (Receipt, error) {
	token := Token(req.ID)
	steps := erasurePlan(req.StoreID, req.CustomerEmail, token)

	receipt := Receipt{
		RequestID:      req.ID,
		Deleted:        map[string]int64{},
		Anonymised:     map[string]int64{},
		RetentionBasis: RetentionBasis,
	}

	err := subscription.WithAdvisoryLock(ctx, e.db, req.StoreID, func(tx *gorm.DB) error {
		// Reset per attempt: WithAdvisoryLock's transaction can be retried by
		// a caller, and counts accumulated across attempts would overstate
		// what was destroyed.
		clear(receipt.Deleted)
		clear(receipt.Anonymised)

		for _, step := range steps {
			res := tx.Exec(step.SQL, step.Args...)
			if res.Error != nil {
				return newStepError(step, res.Error)
			}
			switch step.Disposition {
			case DispositionDelete:
				receipt.Deleted[step.Table] += res.RowsAffected
			case DispositionAnonymise:
				receipt.Anonymised[step.Table] += res.RowsAffected
			}
		}

		receipt.RetainedTables = retainedTables(receipt.Anonymised)
		receipt.CompletedAt = time.Now().UTC()

		notes, mErr := json.Marshal(receipt)
		if mErr != nil {
			return fmt.Errorf("customererasure: encode receipt: %w", mErr)
		}
		res := tx.Exec(`
			UPDATE customer_erasure_requests
			   SET status = ?, processed_at = now(), notes = ?
			 WHERE id = ?`, StatusCompleted, string(notes), req.ID)
		if res.Error != nil {
			return fmt.Errorf("customererasure: write receipt: %w", res.Error)
		}
		return nil
	})
	if err != nil {
		return Receipt{}, err
	}

	// Counts and table names only — never the subject's address.
	e.logger.Info("customer erasure completed",
		"request_id", req.ID.String(),
		"store_id", req.StoreID.String(),
		"attempt", req.Attempts,
		"deleted_rows", total(receipt.Deleted),
		"anonymised_rows", total(receipt.Anonymised),
	)
	return receipt, nil
}

// failureNote is what a failed attempt records. Like Receipt, it carries no
// personal data: the failing table, the disposition it was attempting, and a
// SQLSTATE.
type failureNote struct {
	RequestID uuid.UUID `json:"request_id"`
	Status    string    `json:"status"`
	Attempt   int       `json:"attempt"`
	Table     string    `json:"failed_table,omitempty"`
	SQLState  string    `json:"sqlstate,omitempty"`
	Error     string    `json:"error"`
	FailedAt  time.Time `json:"failed_at"`
}

// markFailed writes 'failed' AFTER the erasure transaction has unwound.
//
// context.WithoutCancel: the caller's context is very often an HTTP request's,
// and the operator who triggered the erasure may well have closed the tab by
// the time it fails. A cancelled context here would drop the only record that
// the attempt happened, leaving the row in 'processing' forever — visible to
// nobody and claimable by no worker.
func (e *Executor) markFailed(ctx context.Context, req Request, cause error) {
	note := failureNote{
		RequestID: req.ID,
		Status:    StatusFailed,
		Attempt:   req.Attempts,
		Error:     cause.Error(),
		FailedAt:  time.Now().UTC(),
	}
	var se *StepError
	if errors.As(cause, &se) {
		note.Table = se.Table
		note.SQLState = se.SQLState
	}

	encoded, err := json.Marshal(note)
	if err != nil {
		encoded = []byte(`{"status":"failed","error":"receipt could not be encoded"}`)
	}

	writeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()

	if err := e.db.WithContext(writeCtx).Exec(`
		UPDATE customer_erasure_requests
		   SET status = ?, notes = ?
		 WHERE id = ?`, StatusFailed, string(encoded), req.ID).Error; err != nil {
		// Nothing left to fall back on. Log loudly: the row is now stuck in
		// 'processing' and only an operator can free it.
		e.logger.Error("customer erasure failed AND its failure could not be recorded",
			"request_id", req.ID.String(), "store_id", req.StoreID.String(), "err", err)
		return
	}
	e.logger.Error("customer erasure failed",
		"request_id", req.ID.String(),
		"store_id", req.StoreID.String(),
		"attempt", req.Attempts,
		"failed_table", note.Table,
		"sqlstate", note.SQLState,
	)
}

// Reject records an operator's refusal. It is the other half of the inbox
// decision and lives here so the status vocabulary has exactly one owner.
//
// notes is operator free text and is stored as given — it is the operator's
// reason, not the subject's data. Only a 'pending' or 'failed' request can be
// rejected; anything else returns ErrAlreadyClaimed so a second decision
// cannot overwrite the first.
func (e *Executor) Reject(ctx context.Context, requestID uuid.UUID, notes string) (Request, error) {
	var req Request
	res := e.db.WithContext(ctx).Raw(`
		UPDATE customer_erasure_requests
		   SET status = ?, processed_at = now(), notes = NULLIF(?, '')
		 WHERE id = ? AND status IN (?, ?)
		RETURNING id, tenant_id, store_id, customer_email, status, attempts, requested_at, processed_at, notes`,
		StatusRejected, notes, requestID, StatusPending, StatusFailed,
	).Scan(&req)
	if res.Error != nil {
		return Request{}, fmt.Errorf("customererasure: reject request %s: %w", requestID, res.Error)
	}
	if res.RowsAffected == 0 {
		return Request{}, ErrAlreadyClaimed
	}
	e.logger.Info("customer erasure rejected",
		"request_id", req.ID.String(), "store_id", req.StoreID.String())
	return req, nil
}

// PendingIDs lists requests waiting to be processed, oldest first. It is the
// worker's queue read; the claim is what actually makes taking one safe.
func (e *Executor) PendingIDs(ctx context.Context, limit int) ([]uuid.UUID, error) {
	if limit <= 0 {
		return nil, nil
	}
	var ids []uuid.UUID
	err := e.db.WithContext(ctx).Raw(`
		SELECT id FROM customer_erasure_requests
		 WHERE status IN (?, ?)
		 ORDER BY requested_at ASC
		 LIMIT ?`, StatusPending, StatusFailed, limit).Scan(&ids).Error
	if err != nil {
		return nil, fmt.Errorf("customererasure: list pending: %w", err)
	}
	return ids, nil
}

// retainedTables is every table whose rows survive: the ones this erasure
// anonymised, plus the ones that needed no statement because they hold no
// personal column.
func retainedTables(anonymised map[string]int64) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(anonymised)+len(retainedWithoutStep))
	for _, list := range [][]string{keys(anonymised), retainedWithoutStep} {
		for _, t := range list {
			if !seen[t] {
				seen[t] = true
				out = append(out, t)
			}
		}
	}
	sort.Strings(out)
	return out
}

func keys(m map[string]int64) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func total(m map[string]int64) int64 {
	var n int64
	for _, v := range m {
		n += v
	}
	return n
}
