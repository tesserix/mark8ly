package outbox

import (
	"context"
	"fmt"

	"gorm.io/gorm"
)

// Repository is the data-access interface for outbox_events.
type Repository interface {
	EnqueueInTx(ctx context.Context, tx *gorm.DB, evt *OutboxEvent) error
	// ProcessBatch opens its own transaction, locks up to `limit` PENDING
	// rows via FOR UPDATE SKIP LOCKED, and calls fn with the rows and the
	// same tx. If fn returns nil the tx commits (the caller is expected to
	// have called MarkPublishedInTx inside fn); if fn returns an error the
	// tx rolls back and the rows become visible to the next poll. Returns
	// the number of rows the callback saw.
	//
	// PENDING means published_at IS NULL *and* error IS NULL. A row with
	// error set is terminal and is never re-selected — see MarkFailedInTx.
	// The partial index outbox_unpublished_idx (migration 000001) is on
	// published_at IS NULL; the error term is a filter on top of it, which
	// is fine while failed rows are ~0. If they ever become common, that
	// index is the thing to revisit.
	ProcessBatch(ctx context.Context, limit int,
		fn func(tx *gorm.DB, rows []OutboxEvent) error) (int, error)
	MarkPublishedInTx(tx *gorm.DB, ids []string) error
	// MarkFailedInTx records why the publisher could not process each row,
	// leaving published_at NULL. A row with error set is TERMINAL: the poll
	// in ProcessBatch excludes it, so it is never retried. Requeueing is an
	// operator action — clearing error re-enters the row into the poll.
	// Note that is re-entry, not recovery: the watermark is monotonic over
	// the row's original created_at, so a stale row publishes without moving
	// it. See the package doc in models.go.
	//
	// Reason must be one of the Reason* constants in models.go, never a raw
	// error string. Anything outside the vocabulary is recorded as ReasonUnknown
	// to prevent raw error strings (which may contain customer-data JSONB) from
	// reaching a column served cross-tenant to the platform console. See the
	// comment on those constants.
	MarkFailedInTx(tx *gorm.DB, failures []Failure) error
}

type gormRepository struct {
	db *gorm.DB
}

func NewRepository(db *gorm.DB) Repository { return &gormRepository{db: db} }

func (r *gormRepository) EnqueueInTx(ctx context.Context, tx *gorm.DB, evt *OutboxEvent) error {
	if err := tx.WithContext(ctx).Create(evt).Error; err != nil {
		return fmt.Errorf("outbox: enqueue: %w", err)
	}
	return nil
}

func (r *gormRepository) ProcessBatch(ctx context.Context, limit int,
	fn func(tx *gorm.DB, rows []OutboxEvent) error) (int, error) {
	var seen int
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var rows []OutboxEvent
		if err := tx.Raw(`
			SELECT id, tenant_id, aggregate, aggregate_id, event_type,
			       payload, created_at, published_at, error
			FROM outbox_events
			WHERE published_at IS NULL AND error IS NULL
			ORDER BY tenant_id, created_at
			LIMIT ?
			FOR UPDATE SKIP LOCKED`, limit).Scan(&rows).Error; err != nil {
			return fmt.Errorf("outbox: poll: %w", err)
		}
		seen = len(rows)
		if seen == 0 {
			return nil
		}
		return fn(tx, rows)
	})
	return seen, err
}

func (r *gormRepository) MarkPublishedInTx(tx *gorm.DB, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	if err := tx.Exec(`UPDATE outbox_events SET published_at = now() WHERE id IN ?`,
		ids).Error; err != nil {
		return fmt.Errorf("outbox: mark published: %w", err)
	}
	return nil
}

// sanitizeReason is the ONLY gate between a caller's string and the error
// column. Anything outside the closed vocabulary becomes ReasonUnknown, so a
// raw err.Error() — which encoding/json fills with fragments of the offending
// payload — cannot reach a column that is served cross-tenant to the console
// (#331), even if a future caller passes one by mistake. The interface comment
// says "never a raw error string"; this is what makes that true rather than
// advisory.
func sanitizeReason(reason string) string {
	switch reason {
	case ReasonPayloadUnparseable, ReasonPayloadMissingStoreID:
		return reason
	default:
		return ReasonUnknown
	}
}

func (r *gormRepository) MarkFailedInTx(tx *gorm.DB, failures []Failure) error {
	if len(failures) == 0 {
		return nil
	}
	byReason := make(map[string][]string, len(failures))
	seen := make(map[string]struct{}, len(failures))
	for _, f := range failures {
		// First occurrence of an id wins. Without this, one id appearing under
		// two reasons lands in two UPDATE groups and the surviving value
		// depends on Go's randomised map iteration order.
		if _, dup := seen[f.ID]; dup {
			continue
		}
		seen[f.ID] = struct{}{}
		r := sanitizeReason(f.Reason)
		byReason[r] = append(byReason[r], f.ID)
	}
	for reason, ids := range byReason {
		if err := tx.Exec(`UPDATE outbox_events SET error = ? WHERE id IN ?`,
			reason, ids).Error; err != nil {
			return fmt.Errorf("outbox: mark failed: %w", err)
		}
	}
	return nil
}
