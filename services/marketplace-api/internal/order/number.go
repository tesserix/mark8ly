package order

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// NextDocumentNumber issues the next per-day sequence number for a (store, kind)
// pair via a single atomic upsert.
//
// The row lock is held for exactly one statement — do NOT wrap this in
// SELECT ... FOR UPDATE or hold it across a wider transaction. The atomic
// INSERT ... ON CONFLICT DO UPDATE ... RETURNING pattern eliminates the
// read-check-write window that would otherwise serialize concurrent checkouts
// on a single row per store per day.
//
// kind must be one of: "order", "return".
//
// This function MUST be called from within an open transaction so the sequence
// increment is rolled back atomically with the domain write that uses it.
// Pass the same *gorm.DB handle you got from s.Unit(...). Calling it outside
// a transaction would let the sequence advance even if the downstream INSERT
// fails — creating gaps in the human-readable order number range.
func NextDocumentNumber(ctx context.Context, tx *gorm.DB, storeID uuid.UUID, kind string, day time.Time) (int, error) {
	if kind != "order" && kind != "return" {
		return 0, fmt.Errorf("order: invalid document kind %q", kind)
	}
	if tx == nil {
		return 0, fmt.Errorf("order: NextDocumentNumber requires a transaction handle")
	}
	dayOnly := day.UTC().Truncate(24 * time.Hour)

	var lastSeq int
	err := tx.WithContext(ctx).Raw(`
		INSERT INTO document_number_seq (store_id, kind, day, last_seq)
		VALUES (?, ?, ?, 1)
		ON CONFLICT (store_id, kind, day)
		DO UPDATE SET last_seq = document_number_seq.last_seq + 1
		RETURNING last_seq
	`, storeID, kind, dayOnly).Scan(&lastSeq).Error
	if err != nil {
		return 0, fmt.Errorf("order: next document number: %w", err)
	}
	return lastSeq, nil
}
