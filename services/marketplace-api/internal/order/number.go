package order

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// sequenceNameCache remembers the Postgres sequence name for each
// (store_id, kind) pair we have issued numbers for. Population is lazy on
// first use and survives for the lifetime of the process.
//
// Key format: "<store-uuid>|<kind>". Value: the sequence name returned by
// ensure_store_sequence (e.g. "mk_seq_order_11111111_1111_...").
var sequenceNameCache sync.Map

// NextDocumentNumber issues the next sequence number for a (store, kind) pair.
//
// Uses a per-store Postgres SEQUENCE created on demand by the
// ensure_store_sequence function (see migration 000003_orders_seq_pivot).
// Sequences use Postgres' native lightweight allocation mechanism — NOT row
// locks — and are configured with CACHE 50 so each backend connection
// reserves 50 values at a time, eliminating contention under burst.
//
// Sequence numbers are monotonic per store FOREVER and do NOT reset daily.
// Human-readable date information lives in the order number format string
// (M-{prefix}-{yymmdd}-{seq}), not in the sequence itself. See spec §2.8
// (revised post-benchmark).
//
// kind must be one of "order" or "return".
//
// This function MUST be called from inside an open transaction so the
// sequence usage is atomic with the domain write that uses the issued number.
// Pass the same *gorm.DB handle you got from s.Unit(...).
func NextDocumentNumber(ctx context.Context, tx *gorm.DB, storeID uuid.UUID, kind string) (int64, error) {
	if kind != "order" && kind != "return" {
		return 0, fmt.Errorf("order: invalid document kind %q", kind)
	}
	if tx == nil {
		return 0, fmt.Errorf("order: NextDocumentNumber requires a transaction handle")
	}

	seqName, err := resolveSequenceName(ctx, tx, storeID, kind)
	if err != nil {
		return 0, err
	}

	// nextval() is not parameterizable (it takes a regclass, not a text
	// bind variable), so we format the name into the SQL. The name is
	// constructed by the ensure_store_sequence server-side function from a
	// uuid and a validated kind — the character set is [a-z0-9_] only, so
	// SQL injection is not possible here.
	var next int64
	err = tx.WithContext(ctx).
		Raw(fmt.Sprintf("SELECT nextval('%s')", seqName)).
		Scan(&next).Error
	if err != nil {
		return 0, fmt.Errorf("order: nextval %s: %w", seqName, err)
	}
	return next, nil
}

// resolveSequenceName returns the Postgres sequence name for a given
// (store_id, kind) pair, creating the sequence if this is the first call
// for that pair in the current process.
//
// The result is cached in a process-wide sync.Map so subsequent calls skip
// the roundtrip to ensure_store_sequence. Cache is never evicted; on pod
// restart the first call for each (store, kind) pays the ~200µs function
// call cost once, then all subsequent calls go straight to nextval.
//
// CRITICAL: ensure_store_sequence runs via the underlying *sql.DB pool, NOT
// the caller's transaction. Creating a Postgres SEQUENCE is transactional —
// if we called CREATE SEQUENCE inside a tx that hasn't committed yet, other
// parallel transactions would fail with "relation does not exist" when they
// try to call nextval on the cached name. Issuing the ensure call on the
// pool means it auto-commits before we return, so the sequence is visible
// to all subsequent callers including the tx that triggered the create.
func resolveSequenceName(ctx context.Context, tx *gorm.DB, storeID uuid.UUID, kind string) (string, error) {
	cacheKey := storeID.String() + "|" + kind
	if cached, ok := sequenceNameCache.Load(cacheKey); ok {
		return cached.(string), nil
	}

	sqlDB, err := tx.DB()
	if err != nil {
		return "", fmt.Errorf("order: get sql.DB for sequence ensure: %w", err)
	}

	var seqName string
	err = sqlDB.QueryRowContext(ctx,
		"SELECT ensure_store_sequence($1, $2)", storeID, kind,
	).Scan(&seqName)
	if err != nil {
		return "", fmt.Errorf("order: ensure_store_sequence: %w", err)
	}
	if seqName == "" {
		return "", fmt.Errorf("order: ensure_store_sequence returned empty name for store=%s kind=%s", storeID, kind)
	}

	// LoadOrStore so concurrent first-callers for the same pair don't
	// overwrite each other's entry. The server-side function is idempotent
	// so repeated calls all return the same name anyway.
	actual, _ := sequenceNameCache.LoadOrStore(cacheKey, seqName)
	return actual.(string), nil
}

// FormatOrderNumber builds the human-readable order number string from a
// store prefix, a day, and the per-store sequence value.
//
// Format: M-<PREFIX>-<yymmdd>-<seq padded to 5 digits>
// Example: M-TST-260409-00123
//
// The sequence is monotonic per store forever (not reset daily), so the
// seq component may exceed 5 digits for high-volume stores — the padding
// is a floor, not a ceiling.
func FormatOrderNumber(prefix string, day time.Time, seq int64) string {
	return fmt.Sprintf("M-%s-%s-%05d",
		strings.ToUpper(prefix),
		day.UTC().Format("060102"),
		seq,
	)
}

// FormatReturnNumber builds the human-readable return number string.
// Same convention as FormatOrderNumber but with R- prefix.
func FormatReturnNumber(prefix string, day time.Time, seq int64) string {
	return fmt.Sprintf("R-%s-%s-%05d",
		strings.ToUpper(prefix),
		day.UTC().Format("060102"),
		seq,
	)
}
