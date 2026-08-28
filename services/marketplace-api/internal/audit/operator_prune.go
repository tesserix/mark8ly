package audit

import (
	"context"
	"fmt"
	"time"
)

// OperatorRetentionYears is how long an operator audit row is kept, counted
// from created_at.
//
// Seven years is not a new number: billing_archive is documented as
// "retained 7 years after hard-delete under legal-obligation basis"
// (migration 000046_billing_archive.up.sql:24, §23.2). An operator
// governance record about a destruction is the same class of artefact under
// the same basis, and reusing the number leaves the estate ONE retention
// story to defend rather than two that have to be reconciled (#365).
const OperatorRetentionYears = 7

// OperatorMetricLabel is the bucket label this path reports under, so
// operator pruning and plan-based pruning are distinguishable in monitoring.
const OperatorMetricLabel = "operator_7y"

// OperatorFreeTextRetentionDays is how long the free-text `reason` on an
// operator audit row is kept, counted from created_at. The row itself lives
// OperatorRetentionYears; only the free text expires on this shorter clock.
//
// Retention is split by FIELD because the two carry different value. The
// structural fields — actor, action, reason_code, timestamps — are what a
// governance question actually turns on, and they justify seven years. The
// free text is incidental personal data: an operator writes "jane@example.com
// disputing the chargeback, ticket 4471", and under #365's rule that row
// survives a GDPR art.17 erasure of the same tenant by seven years (#369).
//
// 180 days covers the full card chargeback window (Visa/Mastercard run to
// roughly 120-180 days), which is the main reason an operator re-reads a
// suspension note. Past that the note's operational value is near zero while
// its privacy cost persists.
//
// NOT redaction-at-erasure-time: that is the same two-stage rule #365
// rejected for the purge row, and it edits audit history on demand.
const OperatorFreeTextRetentionDays = 180

// OperatorFreeTextMetricLabel is the bucket label the free-text strip
// reports under, so it is distinguishable from row deletion in monitoring.
const OperatorFreeTextMetricLabel = "operator_freetext_180d"

// pruneOperatorRows deletes audit_logs rows with actor_type='operator' older
// than cutoff, in batches, and returns (rowsDeleted, batches, error).
//
// Deliberately NOT a fourth retentionBucket. The plan-based path derives its
// window from the row's store's plan and JOINs store_subscriptions on
// store_id; operator rows carry store_id = NULL by design (a store-scoped
// operator row would surface a platform action inside the MERCHANT's own
// audit view), and after a purge that tenant's store_subscriptions rows are
// gone anyway. So the join can never match, and a bucket would mean
// special-casing the very join pruneBucket is built around. The two rules
// have different shapes: one is plan-derived and store-scoped, this one is
// flat and store-less.
//
// This NARROWS #311, which decided store-less audit rows are never pruned.
// That decision still stands for every actor_type EXCEPT 'operator'.
func (c *PruneCron) pruneOperatorRows(ctx context.Context, cutoff time.Time) (int64, int, error) {
	var totalDeleted int64
	batchCount := 0

	for {
		select {
		case <-ctx.Done():
			return totalDeleted, batchCount, ctx.Err()
		default:
		}

		res := c.db.WithContext(ctx).Exec(`
			DELETE FROM audit_logs
			WHERE id IN (
				SELECT id
				FROM audit_logs
				WHERE actor_type = 'operator'
				  AND created_at < ?
				LIMIT ?
			)`,
			cutoff, c.batchSize,
		)
		if res.Error != nil {
			return totalDeleted, batchCount, fmt.Errorf("audit operator prune delete: %w", res.Error)
		}
		batchCount++
		totalDeleted += res.RowsAffected
		if res.RowsAffected == 0 {
			return totalDeleted, batchCount, nil
		}
	}
}

// pruneOperatorFreeText removes the free-text `reason` key from the metadata
// of operator audit rows older than cutoff, in batches, and returns
// (rowsStripped, batches, error).
//
// Strips `reason` ONLY — never `reason_code`. They are different keys: the
// code is a closed validated vocabulary and is the field a regulator's
// question turns on; the text is incidental. `metadata - 'reason'` removes
// exactly the one key and leaves the rest of the object intact.
//
// The jsonb_exists(metadata, 'reason') guard is what terminates the loop:
// once a row is stripped it no longer matches, so RowsAffected reaches 0 the
// same way the DELETE loop's does. It also means a row with no reason is
// never rewritten. jsonb_exists is used rather than the equivalent infix
// `metadata ? 'reason'` because GORM treats `?` as its own bind placeholder
// and would misbind the statement's arguments.
func (c *PruneCron) pruneOperatorFreeText(ctx context.Context, cutoff time.Time) (int64, int, error) {
	var totalStripped int64
	batchCount := 0

	for {
		select {
		case <-ctx.Done():
			return totalStripped, batchCount, ctx.Err()
		default:
		}

		res := c.db.WithContext(ctx).Exec(`
			UPDATE audit_logs
			SET metadata = metadata - 'reason'
			WHERE id IN (
				SELECT id
				FROM audit_logs
				WHERE actor_type = 'operator'
				  AND created_at < ?
				  AND jsonb_exists(metadata, 'reason')
				LIMIT ?
			)`,
			cutoff, c.batchSize,
		)
		if res.Error != nil {
			return totalStripped, batchCount, fmt.Errorf("audit operator free-text strip: %w", res.Error)
		}
		batchCount++
		totalStripped += res.RowsAffected
		if res.RowsAffected == 0 {
			return totalStripped, batchCount, nil
		}
	}
}
