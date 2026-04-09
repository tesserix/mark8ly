package order

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// NextDocumentNumber issues the next sequence number for a (store, kind) pair.
//
// Uses a per-store Postgres SEQUENCE that is created eagerly by the
// stores_after_insert_create_sequences trigger at store-insert time (see
// migration 000004_orders_seq_eager). Because the sequence is guaranteed to
// exist before any order can reference the store, this function needs no
// on-demand creation, no process-level name cache, and no roundtrip to a
// PL/pgSQL helper — it calls nextval directly.
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

	seqName := buildSequenceName(storeID, kind)

	// nextval() is not parameterizable (it takes a regclass, not a text
	// bind variable), so we format the name into the SQL. The name is
	// derived from a uuid and a validated kind — the character set is
	// [a-z0-9_] only, so SQL injection is not possible here.
	var next int64
	err := tx.WithContext(ctx).
		Raw(fmt.Sprintf("SELECT nextval('%s')", seqName)).
		Scan(&next).Error
	if err != nil {
		return 0, fmt.Errorf("order: nextval %s: %w", seqName, err)
	}
	return next, nil
}

// buildSequenceName returns the Postgres sequence name for a given
// (store_id, kind) pair. MUST stay in lockstep with the SQL trigger
// function mk_create_store_sequences() in migration 000004 — both sides
// produce the same name from the same inputs.
//
// Format: mk_seq_<kind>_<uuid with dashes replaced by underscores>
// Example: mk_seq_order_11111111_1111_1111_1111_111111111111
func buildSequenceName(storeID uuid.UUID, kind string) string {
	return "mk_seq_" + kind + "_" + strings.ReplaceAll(storeID.String(), "-", "_")
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
