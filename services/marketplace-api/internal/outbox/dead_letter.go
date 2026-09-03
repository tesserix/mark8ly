// Package outbox: requeue and dead-letter — the platform console's WRITE
// half of #331/#336 (#405). Deliberately a separate file from repository.go
// (the publisher's write path) and platform_list.go (the cross-tenant
// read): this is an OPERATOR write, driven by a human decision, and shares
// nothing with either but the table.
package outbox

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
)

// ErrAlreadyPublished is returned by RequeueOne and DeadLetterOne when the
// target row's published_at is non-nil.
//
// This is the guard the whole ticket exists for. published_at is the ONLY
// marker that a row was delivered — there is no status column and no
// attempt counter — so clearing error (or setting dead_lettered_at) on an
// already-published row would hand it back to the publisher and cause a
// double-publish: a delivery failure turned into a data-corruption
// problem. See lockForWrite, which enforces this under FOR UPDATE so a
// concurrent publish cannot race past it.
var ErrAlreadyPublished = errors.New("outbox: row already published; requeue/dead-letter refused")

// ErrNotFound is returned when the target id does not exist.
var ErrNotFound = errors.New("outbox: row not found")

// ErrReasonRequired is returned by DeadLetterOne when the supplied reason
// is empty (after trimming). A dead-letter with no human explanation is
// exactly the gap this operation exists to close.
var ErrReasonRequired = errors.New("outbox: dead-letter reason is required")

// RequeueResult is the outcome of a successful RequeueOne, carrying the
// row's ORIGINAL created_at — the only place it survives, since requeue
// overwrites the column itself. Callers (the platform-admin handler) put
// this in the audit event.
type RequeueResult struct {
	ID                string
	TenantID          string
	OriginalCreatedAt time.Time
}

// RequeueOutcome is one row's result from RequeueBatch. OriginalCreatedAt
// and TenantID are meaningful only when OK is true; Err is a short,
// stable code ("not_found", "already_published", "internal_error")
// meaningful only when OK is false.
type RequeueOutcome struct {
	ID                string
	OK                bool
	TenantID          string
	OriginalCreatedAt time.Time
	Err               string
}

// DeadLetterResult is the outcome of a successful DeadLetterOne.
type DeadLetterResult struct {
	ID             string
	TenantID       string
	DeadLetteredAt time.Time
}

// lockRow is what lockForWrite reads under FOR UPDATE.
type lockRow struct {
	TenantID    string
	CreatedAt   time.Time
	PublishedAt *time.Time
}

// lockForWrite locks the target row FOR UPDATE inside tx and returns it, or
// ErrNotFound / ErrAlreadyPublished. This is the ONE place that decides
// whether a requeue or dead-letter may proceed — both operations refuse a
// row whose published_at is non-nil, and the check happens under the same
// row lock the subsequent UPDATE uses, so a concurrent publish cannot slip
// between the check and the write.
func lockForWrite(tx *gorm.DB, id string) (lockRow, error) {
	var rows []lockRow
	if err := tx.Raw(`SELECT tenant_id, created_at, published_at FROM outbox_events WHERE id = ? FOR UPDATE`, id).
		Scan(&rows).Error; err != nil {
		return lockRow{}, fmt.Errorf("outbox: lock row: %w", err)
	}
	if len(rows) == 0 {
		return lockRow{}, ErrNotFound
	}
	row := rows[0]
	if row.PublishedAt != nil {
		return lockRow{}, ErrAlreadyPublished
	}
	return row, nil
}

// RequeueOne clears error, clears BOTH dead-letter columns (dead-letter is
// reversible), and bumps created_at to now() so the monotonic watermark
// upsert (see the package doc in models.go) actually moves. Refuses any row
// whose published_at is non-nil — see lockForWrite and ErrAlreadyPublished.
//
// The WHERE clause's "AND published_at IS NULL" is defense in depth: it can
// never fail given lockForWrite already verified this under the same row
// lock, but the guard is written explicitly here too rather than trusted to
// the earlier check alone, matching how ProcessBatch's poll states its
// dead_lettered_at exclusion explicitly rather than relying on error also
// being set.
func RequeueOne(ctx context.Context, db *gorm.DB, id string) (RequeueResult, error) {
	var result RequeueResult
	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		row, err := lockForWrite(tx, id)
		if err != nil {
			return err
		}
		if err := tx.Exec(`
			UPDATE outbox_events
			   SET error = NULL, dead_lettered_at = NULL, dead_letter_reason = NULL, created_at = now()
			 WHERE id = ? AND published_at IS NULL`, id).Error; err != nil {
			return fmt.Errorf("outbox: requeue: %w", err)
		}
		result = RequeueResult{ID: id, TenantID: row.TenantID, OriginalCreatedAt: row.CreatedAt}
		return nil
	})
	if err != nil {
		return RequeueResult{}, err
	}
	return result, nil
}

// RequeueBatch requeues each id independently, in its OWN transaction, so
// one bad id (not found, or already published) cannot fail the rest of the
// set. The outcome order matches the input id order.
func RequeueBatch(ctx context.Context, db *gorm.DB, ids []string) []RequeueOutcome {
	outcomes := make([]RequeueOutcome, 0, len(ids))
	for _, id := range ids {
		res, err := RequeueOne(ctx, db, id)
		if err != nil {
			outcomes = append(outcomes, RequeueOutcome{ID: id, OK: false, Err: requeueOutcomeCode(err)})
			continue
		}
		outcomes = append(outcomes, RequeueOutcome{
			ID: id, OK: true, TenantID: res.TenantID, OriginalCreatedAt: res.OriginalCreatedAt,
		})
	}
	return outcomes
}

// requeueOutcomeCode maps a RequeueOne error to the short, stable code a
// per-row batch outcome carries. A caller (the console) needs to
// distinguish "gone" from "refused" from "something else broke"; the
// underlying error text is not part of this contract.
func requeueOutcomeCode(err error) string {
	switch {
	case errors.Is(err, ErrNotFound):
		return "not_found"
	case errors.Is(err, ErrAlreadyPublished):
		return "already_published"
	default:
		return "internal_error"
	}
}

// DeadLetterOne sets dead_lettered_at and the supplied reason. reason is
// REQUIRED: an empty (after trimming) reason is refused with
// ErrReasonRequired, since a dead-letter with no human explanation defeats
// the point of the operation. Refuses any row whose published_at is
// non-nil — a delivered row cannot be dead-lettered — see lockForWrite.
func DeadLetterOne(ctx context.Context, db *gorm.DB, id, reason string) (DeadLetterResult, error) {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return DeadLetterResult{}, ErrReasonRequired
	}

	// Captured in Go, not left to the database's now(), so the value
	// returned to the caller (and put in the audit event) is EXACTLY what
	// was written — no second read-back required.
	now := time.Now().UTC()

	var tenantID string
	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		row, err := lockForWrite(tx, id)
		if err != nil {
			return err
		}
		tenantID = row.TenantID
		if err := tx.Exec(`
			UPDATE outbox_events
			   SET dead_lettered_at = ?, dead_letter_reason = ?
			 WHERE id = ? AND published_at IS NULL`, now, reason, id).Error; err != nil {
			return fmt.Errorf("outbox: dead-letter: %w", err)
		}
		return nil
	})
	if err != nil {
		return DeadLetterResult{}, err
	}
	return DeadLetterResult{ID: id, TenantID: tenantID, DeadLetteredAt: now}, nil
}

// WriterFuncs adapts this package's requeue/dead-letter functions to
// platformadmin.OutboxWriter, so main.go can wire
// `outbox.WriterFuncs{}` directly — the same reason OutboxListerFunc
// adapts outbox.ListPlatform for the read side. The zero value is usable;
// there is no state to hold.
type WriterFuncs struct{}

func (WriterFuncs) RequeueOne(ctx context.Context, db *gorm.DB, id string) (RequeueResult, error) {
	return RequeueOne(ctx, db, id)
}

func (WriterFuncs) RequeueBatch(ctx context.Context, db *gorm.DB, ids []string) []RequeueOutcome {
	return RequeueBatch(ctx, db, ids)
}

func (WriterFuncs) DeadLetterOne(ctx context.Context, db *gorm.DB, id, reason string) (DeadLetterResult, error) {
	return DeadLetterOne(ctx, db, id, reason)
}
