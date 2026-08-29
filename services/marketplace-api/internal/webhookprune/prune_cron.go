// Package webhookprune enforces an age-based retention window on
// webhook_events, the raw provider event log written by the storefront
// payment webhook handler.
//
// WHY AN AGE-BASED PRUNE AND NOT ERASURE (#440). webhook_events.payload holds
// the provider's event body verbatim — for Stripe that carries
// billing_details.email, shipping.address and customer_details. The table has
// only (id, provider, provider_event_id, event_type, payload, status,
// processed_at, created_at): no tenant_id, no store_id, no order_id, no
// customer link. So neither GDPR erasure (#259/#435) nor tenant purge can
// reach it — internal/tenantpurge/purge.go:21-25 records exactly that as the
// reason the table is excluded. Age is the only axis this table exposes.
//
// SAFE TO DELETE. Nothing reads the payload back. The only two statements
// touching this table are the INSERT and the `UPDATE ... SET
// status='processed'` in internal/handlers/storefront/webhooks.go. The
// payload's value is operational — debugging a delivery, replaying a stuck
// event — and it decays within days.
package webhookprune

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"gorm.io/gorm"
)

// PruneSpec is the cron expression for the webhook_events retention prune —
// 03:30 UTC daily. Deliberately offset from every other daily cron in the
// service: 00:00 campaign ramp, 00:15 trial expiry, 00:30 trial activation,
// 01:00 finalize, 01:30 close, 02:00 audit prune + tax revalidation + queue
// hard-delete, 03:00 hard delete, 05:00 signup anomaly, and the 09:xx email
// block. 03:30 is the widest gap left in the small hours.
const PruneSpec = "30 3 * * *"

// PruneBatchSize is the number of webhook_events rows deleted per statement.
// Keeps each transaction short so the webhook INSERT path — which runs on the
// provider's delivery timeout — is never queued behind the prune.
const PruneBatchSize = 5000

// ProcessedRetentionDays is how long a row that COMPLETED processing is kept,
// counted from created_at.
//
// `status = 'processed'` is written by the UPDATE at
// internal/handlers/storefront/webhooks.go once processEvent has run. Its
// payload has already done its job; thirty days is a debugging tail, not a
// record.
const ProcessedRetentionDays = 30

// UnprocessedRetentionDays is how long a row that never completed processing
// is kept, counted from created_at.
//
// IN THIS TABLE'S ACTUAL VOCABULARY: there is no 'failed' status. The INSERT
// writes 'received' and the only UPDATE writes 'processed' — those are the
// two values that exist. So the long-retention class is everything that is
// NOT 'processed', which in practice means 'received': the event was accepted
// and stored but processing never reached the UPDATE. That is the stuck or
// half-applied case, and it is the one an operator may still need to inspect
// or replay, so it gets the longer window rather than the shorter one.
//
// Ninety days matches the audit trial/starter bucket, so the estate has one
// "three months of operational history" number rather than two.
const UnprocessedRetentionDays = 90

// Metric labels for the two retention classes, so they stay distinguishable
// in monitoring.
const (
	ProcessedMetricLabel   = "processed_30d"
	UnprocessedMetricLabel = "unprocessed_90d"
)

// batchCeiling bounds one cron tick. 100 × 5000 = 500K rows, far beyond any
// realistic daily webhook volume; whatever is left resumes on the next tick.
const batchCeiling = 100

// retentionClass pairs a retention window with the status predicate that
// selects the rows it governs.
type retentionClass struct {
	// ProcessedOnly selects `status = 'processed'` when true, and everything
	// else when false. A boolean rather than a status string because the
	// long-retention class is defined by exclusion — it must cover any value
	// that is not 'processed', including one a future provider integration
	// introduces.
	ProcessedOnly bool
	RetentionDays int
	Description   string
	MetricLabel   string
}

var retentionClasses = []retentionClass{
	{
		ProcessedOnly: true,
		RetentionDays: ProcessedRetentionDays,
		Description:   "processed (30 day retention)",
		MetricLabel:   ProcessedMetricLabel,
	},
	{
		ProcessedOnly: false,
		RetentionDays: UnprocessedRetentionDays,
		Description:   "unprocessed (90 day retention)",
		MetricLabel:   UnprocessedMetricLabel,
	},
}

// CounterFn is the metric injection hook. Called once per class per Run with
// (label, rowsDeleted). Kept generic so this package does not import
// prometheus.
type CounterFn func(label string, increment int64)

// PruneCron deletes webhook_events rows past their retention window.
type PruneCron struct {
	db        *gorm.DB
	logger    *slog.Logger
	clock     func() time.Time
	batchSize int
	counter   CounterFn
}

// NewPruneCron constructs a PruneCron with sensible defaults. logger and
// clock may be nil; batchSize <= 0 falls back to PruneBatchSize.
func NewPruneCron(db *gorm.DB, logger *slog.Logger, clock func() time.Time, batchSize int) *PruneCron {
	if logger == nil {
		logger = slog.Default()
	}
	if clock == nil {
		clock = func() time.Time { return time.Now().UTC() }
	}
	if batchSize <= 0 {
		batchSize = PruneBatchSize
	}
	return &PruneCron{db: db, logger: logger, clock: clock, batchSize: batchSize}
}

// WithCounter attaches a metric incrementer. Pass nil to disable.
func (c *PruneCron) WithCounter(fn CounterFn) *PruneCron {
	c.counter = fn
	return c
}

// PruneStats summarises one Run pass.
type PruneStats struct {
	ClassesRun         int
	ProcessedDeleted   int64
	UnprocessedDeleted int64
	BatchesRun         int
	ErrorsByClass      map[string]int
}

// RowsDeleted is the total across both retention classes.
func (s PruneStats) RowsDeleted() int64 { return s.ProcessedDeleted + s.UnprocessedDeleted }

// Run executes one prune pass over both retention classes. A per-class
// failure is logged and recorded in stats.ErrorsByClass rather than aborting
// the pass: a transient lock conflict on one class must not block the other,
// and a prune failure must never take the service down.
func (c *PruneCron) Run(ctx context.Context) (PruneStats, error) {
	now := c.clock().UTC()
	stats := PruneStats{ErrorsByClass: map[string]int{}}

	for _, class := range retentionClasses {
		stats.ClassesRun++
		cutoff := now.AddDate(0, 0, -class.RetentionDays)
		deleted, batches, err := c.pruneClass(ctx, class.ProcessedOnly, cutoff)
		if class.ProcessedOnly {
			stats.ProcessedDeleted += deleted
		} else {
			stats.UnprocessedDeleted += deleted
		}
		stats.BatchesRun += batches

		if c.counter != nil && deleted > 0 {
			c.counter(class.MetricLabel, deleted)
		}

		if err != nil {
			stats.ErrorsByClass[class.Description]++
			// A cancelled context means workerCtx was torn down by a routine
			// shutdown mid-pass, not that the prune broke. Logging that at
			// ERROR on every clean shutdown is the kind of line that costs an
			// operator ten minutes at the wrong moment.
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				c.logger.Info("webhook prune: class interrupted by shutdown",
					"class", class.Description, "deleted_so_far", deleted, "err", err.Error())
			} else {
				c.logger.Error("webhook prune: class failed; continuing",
					"class", class.Description, "deleted_so_far", deleted, "err", err.Error())
			}
			continue
		}

		c.logger.Info("webhook prune: class complete",
			"class", class.Description,
			"cutoff", cutoff.Format(time.RFC3339),
			"rows_deleted", deleted,
			"batches", batches)
	}

	return stats, nil
}

// pruneClass loops batched DELETEs until a batch affects zero rows or ctx
// cancels, and returns (rowsDeleted, batches, error).
//
// The DELETE uses a keyset subquery because Postgres has no DELETE ... LIMIT.
// The subquery's LIMIT bounds per-statement work and is served by the
// (status, created_at) index added in migration 000114; the outer
// `WHERE id IN (…)` then locates rows by primary key.
func (c *PruneCron) pruneClass(ctx context.Context, processedOnly bool, cutoff time.Time) (int64, int, error) {
	statusPredicate := "status <> 'processed'"
	if processedOnly {
		statusPredicate = "status = 'processed'"
	}

	stmt := fmt.Sprintf(`
		DELETE FROM webhook_events
		WHERE id IN (
			SELECT id
			FROM webhook_events
			WHERE %s
			  AND created_at < ?
			LIMIT ?
		)`, statusPredicate)

	var totalDeleted int64
	batchCount := 0

	for {
		select {
		case <-ctx.Done():
			return totalDeleted, batchCount, ctx.Err()
		default:
		}

		res := c.db.WithContext(ctx).Exec(stmt, cutoff, c.batchSize)
		if res.Error != nil {
			return totalDeleted, batchCount, fmt.Errorf("webhook prune delete: %w", res.Error)
		}
		batchCount++
		totalDeleted += res.RowsAffected

		if res.RowsAffected == 0 {
			return totalDeleted, batchCount, nil
		}

		if batchCount >= batchCeiling {
			c.logger.Warn("webhook prune: hit batch ceiling; deferring rest to next tick",
				"processed_only", processedOnly, "batches", batchCount)
			return totalDeleted, batchCount, nil
		}
	}
}
