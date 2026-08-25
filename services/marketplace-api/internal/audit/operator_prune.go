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
