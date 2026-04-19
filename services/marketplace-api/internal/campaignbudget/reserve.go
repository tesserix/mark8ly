package campaignbudget

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Reserve atomically decrements the current-month budget row for storeID by
// recipientCount. Returns the post-decrement `remaining` on success.
//
// The UPDATE is the exact shape from spec §10.1 — a single round-trip with a
// WHERE guard that makes the update a no-op when remaining < recipientCount.
// That no-op becomes ErrBudgetExhausted (after a disambiguating SELECT).
//
// Thread safety: Postgres row-level locking inside the UPDATE serializes
// concurrent callers; no Go-level mutex needed. Two concurrent sends against
// the same store either both succeed (if both fit in remaining) or one wins
// and the other sees ErrBudgetExhausted.
func Reserve(ctx context.Context, db *gorm.DB, storeID uuid.UUID, recipientCount int) (int, error) {
	if recipientCount <= 0 {
		return 0, fmt.Errorf("recipient_count must be positive, got %d", recipientCount)
	}

	// Exact SQL from spec §10.1 — single UPDATE, atomic, no SELECT first.
	const sql = `
		UPDATE campaign_email_budget
		SET remaining = remaining - $1
		WHERE store_id = $2
		  AND month = date_trunc('month', (now() AT TIME ZONE 'utc'))::date
		  AND remaining >= $1
		RETURNING remaining`

	var remaining int
	row := db.WithContext(ctx).Raw(sql, recipientCount, storeID).Row()
	if err := row.Scan(&remaining); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) || err.Error() == "sql: no rows in result set" {
			return 0, classifyNoUpdate(ctx, db, storeID)
		}
		return 0, fmt.Errorf("reserve: %w", err)
	}
	SentTotal.WithLabelValues(storeID.String()).Add(float64(recipientCount))
	return remaining, nil
}

// classifyNoUpdate disambiguates a 0-row UPDATE between "no row exists"
// (ErrNoBudgetRow) and "row exists but insufficient balance" (ErrBudgetExhausted).
// Only called on the error path, so the hot path stays one round-trip.
func classifyNoUpdate(ctx context.Context, db *gorm.DB, storeID uuid.UUID) error {
	var exists bool
	err := db.WithContext(ctx).Raw(`
		SELECT EXISTS (
			SELECT 1 FROM campaign_email_budget
			WHERE store_id = $1
			  AND month = date_trunc('month', (now() AT TIME ZONE 'utc'))::date
		)`, storeID).Scan(&exists).Error
	if err != nil {
		return fmt.Errorf("classify reserve failure: %w", err)
	}
	if !exists {
		return ErrNoBudgetRow
	}
	// Row exists, so the only way the UPDATE hit 0 rows is remaining < count.
	return ErrBudgetExhausted
}
