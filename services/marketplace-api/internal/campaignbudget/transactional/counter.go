// Package transactional tracks per-store transactional email volume against
// the 100k/store/month fair-use soft cap (spec §10.2). Transactional sends
// NEVER decrement campaign_email_budget — that table is campaign-only. This
// counter exists purely to surface abuse patterns to ops; callers record
// after the send succeeds, and IsOverFairUse is checked lazily on a dashboard
// job, not on the send path.
package transactional

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// FairUseCapPerMonth is the soft threshold above which ops investigates.
// No hard block — sends still succeed; the counter signals ops to review.
const FairUseCapPerMonth = 100_000

// Record increments the current-month counter for storeID by count. Creates
// the row if missing using INSERT ... ON CONFLICT DO UPDATE. Returns the new
// total. Never blocks the hot path — if the DB is unreachable the caller can
// choose to log and continue rather than failing the send.
func Record(ctx context.Context, db *gorm.DB, storeID uuid.UUID, count int) (int, error) {
	if count <= 0 {
		return 0, nil
	}
	const sql = `
		INSERT INTO store_transactional_counter (store_id, month, sent)
		VALUES ($1, date_trunc('month', (now() AT TIME ZONE 'utc'))::date, $2)
		ON CONFLICT (store_id, month) DO UPDATE
		SET sent = store_transactional_counter.sent + EXCLUDED.sent
		RETURNING sent`
	var total int
	row := db.WithContext(ctx).Raw(sql, storeID, count).Row()
	if err := row.Scan(&total); err != nil {
		return 0, fmt.Errorf("transactional record: %w", err)
	}
	return total, nil
}

// IsOverFairUse returns true when the month total exceeds the soft cap. Ops
// dashboards surface stores in this set; enforcement (if any) is manual.
func IsOverFairUse(total int) bool { return total > FairUseCapPerMonth }
