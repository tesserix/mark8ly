package audit

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"time"

	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/plangate"
	"github.com/mark8ly/marketplace-api/internal/subscription"
)

// PruneSpec is the cron expression for the audit retention prune cron —
// runs at 02:00 UTC daily, well outside business hours and offset from the
// other daily crons (00:15 expiry, 09:00 banner, 09:15 SCA, 09:30 trial).
const PruneSpec = "0 2 * * *"

// PruneBatchSize is the number of audit rows deleted per transaction. Keeps
// individual transactions short so the writer queue isn't blocked. Tuned
// against the (tenant_id, store_id, created_at DESC) index supporting fast
// keyset deletion.
const PruneBatchSize = 5000

// retentionBucket pairs a set of plans that share a retention window with
// the cutoff in days. Pro is intentionally absent — its retention is
// plangate.Unlimited per the matrix, and pruning Pro audit logs would
// violate the spec's "forever audit retention" promise.
type retentionBucket struct {
	Plans         []subscription.SubscriptionPlan
	RetentionDays int
	Description   string
	MetricLabel   string // stable label fed to the AuditPruneRowsDeletedTotal counter
}

// retentionBuckets — derived from plangate.matrix.go FeatureAuditRetentionDays.
// Kept as a code-level mirror rather than a per-row plangate.Limit() lookup
// so the prune sweep is one DELETE per bucket instead of per-store.
var retentionBuckets = []retentionBucket{
	{
		Plans:         []subscription.SubscriptionPlan{subscription.PlanTrial, subscription.PlanStarter},
		RetentionDays: 90,
		Description:   "trial+starter (90 day retention)",
		MetricLabel:   "trial_starter_90d",
	},
	{
		Plans:         []subscription.SubscriptionPlan{subscription.PlanStudio},
		RetentionDays: 365,
		Description:   "studio (365 day retention)",
		MetricLabel:   "studio_365d",
	},
	// PlanPro intentionally omitted — Unlimited retention.
	// PlanMarketplace is platform-internal; it's also not pruned (operator-managed).
}

// PruneCron deletes audit_logs rows via two paths per spec §9: (1) per-plan
// buckets (Trial 90d, Starter 90d, Studio 365d, Pro unlimited) joined to
// store_subscriptions on store_id for multi-tenant safety, and (2) platform
// operator rows (actor_type='operator', no store_id) pruned separately at
// seven years (#365). Each path is safe against cross-tenant leakage.
type PruneCron struct {
	db        *gorm.DB
	logger    *slog.Logger
	clock     func() time.Time
	batchSize int
	counter   CounterFn
}

// NewPruneCron constructs a PruneCron with sensible defaults. clock and
// logger may be nil; batchSize <= 0 falls back to PruneBatchSize.
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

// PruneStats summarises one Run pass for telemetry / logging.
type PruneStats struct {
	BucketsRun   int
	RowsDeleted  int64
	BatchesRun   int
	ErrorsByPlan map[string]int

	// OperatorRowsDeleted counts rows removed by the 7-year operator path
	// (#365), kept separate from RowsDeleted so the two retention rules stay
	// distinguishable in logs and telemetry.
	OperatorRowsDeleted int64

	// OperatorFreeTextStripped counts operator rows whose free-text `reason`
	// was removed by the 180-day window (#369). These rows are NOT deleted.
	OperatorFreeTextStripped int64
}

// Run executes one prune pass over every retention bucket. Per-bucket
// failures are logged and accumulated in stats.ErrorsByPlan rather than
// aborting the whole run, so a transient lock conflict on one bucket
// doesn't block the others.
func (c *PruneCron) Run(ctx context.Context) (PruneStats, error) {
	now := c.clock().UTC()
	stats := PruneStats{ErrorsByPlan: map[string]int{}}

	for _, b := range retentionBuckets {
		stats.BucketsRun++
		cutoff := now.AddDate(0, 0, -b.RetentionDays)
		deleted, batches, err := c.pruneBucket(ctx, b.Plans, cutoff)
		stats.RowsDeleted += deleted
		stats.BatchesRun += batches
		if err != nil {
			stats.ErrorsByPlan[b.Description]++
			c.logger.Error("audit prune: bucket failed; continuing",
				"bucket", b.Description, "deleted_so_far", deleted, "err", err.Error())
			if c.counter != nil && deleted > 0 {
				c.counter(b.MetricLabel, deleted)
			}
			continue
		}
		if c.counter != nil && deleted > 0 {
			c.counter(b.MetricLabel, deleted)
		}
		c.logger.Info("audit prune: bucket complete",
			"bucket", b.Description,
			"cutoff", cutoff.Format(time.RFC3339),
			"rows_deleted", deleted,
			"batches", batches)
	}

	// #365 — operator rows, 7 years, no join. Failure is logged and
	// swallowed like a bucket failure: a lock conflict here must not fail
	// the whole pass.
	opCutoff := now.AddDate(-OperatorRetentionYears, 0, 0)
	opDeleted, opBatches, err := c.pruneOperatorRows(ctx, opCutoff)
	stats.OperatorRowsDeleted = opDeleted
	stats.BatchesRun += opBatches
	if c.counter != nil && opDeleted > 0 {
		c.counter(OperatorMetricLabel, opDeleted)
	}
	if err != nil {
		stats.ErrorsByPlan["operator (7 year retention)"]++
		// #365 F3: ctx.Err() (context.Canceled / context.DeadlineExceeded)
		// means workerCtx was cancelled by a routine shutdown mid-pass, not
		// that the prune itself failed. Logging that at ERROR reads as "the
		// prune broke" on every clean shutdown — exactly the kind of line
		// that costs an operator ten minutes at the wrong moment. Anything
		// else (a real DB/driver error) still logs at ERROR unchanged.
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			c.logger.Info("audit prune: operator path interrupted by shutdown",
				"deleted_so_far", opDeleted, "err", err.Error())
		} else {
			c.logger.Error("audit prune: operator path failed",
				"deleted_so_far", opDeleted, "err", err.Error())
		}
	} else {
		c.logger.Info("audit prune: operator path complete",
			"cutoff", opCutoff.Format(time.RFC3339),
			"rows_deleted", opDeleted, "batches", opBatches)
	}

	// #369 — operator FREE TEXT, 180 days. The row itself stays for
	// OperatorRetentionYears; only the incidental personal data in `reason`
	// expires early. Same failure handling as the operator path above: a
	// lock conflict must not fail the whole pass.
	ftCutoff := now.AddDate(0, 0, -OperatorFreeTextRetentionDays)
	ftStripped, ftBatches, ftErr := c.pruneOperatorFreeText(ctx, ftCutoff)
	stats.OperatorFreeTextStripped = ftStripped
	stats.BatchesRun += ftBatches
	if c.counter != nil && ftStripped > 0 {
		c.counter(OperatorFreeTextMetricLabel, ftStripped)
	}
	if ftErr != nil {
		stats.ErrorsByPlan["operator free text (180 day retention)"]++
		if errors.Is(ftErr, context.Canceled) || errors.Is(ftErr, context.DeadlineExceeded) {
			c.logger.Info("audit prune: operator free-text path interrupted by shutdown",
				"stripped_so_far", ftStripped, "err", ftErr.Error())
		} else {
			c.logger.Error("audit prune: operator free-text path failed",
				"stripped_so_far", ftStripped, "err", ftErr.Error())
		}
	} else {
		c.logger.Info("audit prune: operator free-text path complete",
			"cutoff", ftCutoff.Format(time.RFC3339),
			"rows_stripped", ftStripped, "batches", ftBatches)
	}

	return stats, nil
}

// CounterFn is the metric injection hook for the prune cron. Called once
// per bucket per Run with (label, rowsDeleted). Kept generic to avoid
// importing the prometheus package directly into the audit package.
type CounterFn func(label string, increment int64)

// WithCounter attaches a metric incrementer. Pass nil to disable.
func (c *PruneCron) WithCounter(fn CounterFn) *PruneCron {
	c.counter = fn
	return c
}

// pruneBucket loops batched DELETEs against audit_logs joined to
// store_subscriptions, removing only rows whose store's plan is in plans
// AND whose created_at predates cutoff. Loops until the DELETE affects 0
// rows or ctx cancels. Returns total rows deleted plus batch count.
//
// The DELETE uses a keyset subquery rather than DELETE ... LIMIT (which
// Postgres doesn't support). The subquery's LIMIT bounds the per-statement
// work; the WHERE id IN (…) lets the planner use the audit_logs PK to
// locate rows to drop.
func (c *PruneCron) pruneBucket(ctx context.Context, plans []subscription.SubscriptionPlan, cutoff time.Time) (int64, int, error) {
	var totalDeleted int64
	batchCount := 0

	// Convert plan enum values to their string form for the parameterised IN clause.
	planStrings := make([]string, len(plans))
	for i, p := range plans {
		planStrings[i] = string(p)
	}

	for {
		select {
		case <-ctx.Done():
			return totalDeleted, batchCount, ctx.Err()
		default:
		}

		// #311: tenant-scoped platform operator audit rows (store_id IS
		// NULL — suspend/unsuspend/purge/trial-extend, possible since
		// migration 000101) are never pruned on this timer. They're rare,
		// individually consequential integrity records of privileged
		// actions against a customer's tenant ("who suspended this tenant,
		// and why"), unlike the high-volume, low-consequence merchant
		// activity this retention policy targets. The INNER JOIN below
		// already excludes them (it can't match a NULL store_id), but that
		// exclusion is incidental — the explicit `a.store_id IS NOT NULL`
		// makes it the policy. DO NOT remove this condition, and DO NOT
		// change the JOIN to a LEFT JOIN without preserving it, or this
		// prune will start deleting operator audit history with nothing to
		// catch it.
		//
		// "Never pruned" means never pruned on this timer, not immune to
		// deletion — but tenant purge does NOT get you there either.
		// internal/tenantpurge/purge.go:370 runs
		// `DELETE FROM audit_logs WHERE tenant_id = ? AND actor_type <> 'operator'`,
		// which deliberately EXCLUDES operator rows: the purge's own audit
		// row is written after the purge commits, and the outbox drainer
		// re-runs the same plan ~1s later as a backstop, so the exclusion
		// stops that backstop from destroying the record of the destruction
		// it is backing up (#288). That's exactly why operator rows need
		// their own retention path rather than riding on GDPR erasure: see
		// pruneOperatorRows in operator_prune.go, which prunes them at
		// seven years instead (#365).
		res := c.db.WithContext(ctx).Exec(`
			DELETE FROM audit_logs
			WHERE id IN (
				SELECT a.id
				FROM audit_logs a
				JOIN store_subscriptions s ON s.store_id = a.store_id
				WHERE s.plan IN ?
				  AND a.created_at < ?
				  AND a.store_id IS NOT NULL
				LIMIT ?
			)`,
			planStrings, cutoff, c.batchSize,
		)
		if res.Error != nil {
			return totalDeleted, batchCount, fmt.Errorf("audit prune delete: %w", res.Error)
		}
		batchCount++
		totalDeleted += res.RowsAffected

		// Bail when the batch is short — either we hit the end or the planner
		// is starving. Either way, next cron tick will resume.
		if res.RowsAffected < int64(c.batchSize) {
			return totalDeleted, batchCount, nil
		}

		// Sanity guard against unbounded looping in case of a clock skew or
		// runaway condition. 100 batches × 5000 = 500K rows per cron tick —
		// more than enough for daily incremental prune.
		if batchCount >= 100 {
			c.logger.Warn("audit prune: hit per-bucket batch ceiling; deferring rest to next tick",
				"bucket_plans", plans, "batches", batchCount)
			return totalDeleted, batchCount, nil
		}
	}
}

// AuditRetentionDaysForPlan exposes the retention window used by the prune
// cron so callers (e.g. admin handlers exposing AllFeatureLimits) can
// surface the same value the prune cron will enforce. Returns
// plangate.Unlimited for Pro/Marketplace and the bucket value for others.
func AuditRetentionDaysForPlan(p subscription.SubscriptionPlan) int {
	for _, b := range retentionBuckets {
		if slices.Contains(b.Plans, p) {
			return b.RetentionDays
		}
	}
	// Default — matches plangate.matrix.go for unspecified plans.
	return plangate.Limit(p, plangate.FeatureAuditRetentionDays)
}
