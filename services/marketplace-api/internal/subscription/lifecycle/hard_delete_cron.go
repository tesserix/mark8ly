package lifecycle

import (
	"context"
	"log/slog"
	"time"

	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/audit"
	"github.com/mark8ly/marketplace-api/internal/subscription"
	"github.com/mark8ly/marketplace-api/internal/subscription/harddelete"
)

// HardDeleteSpec is the cron expression for the 150-day hard-delete cron: 03:00 UTC daily.
const HardDeleteSpec = "0 3 * * *"

// hardDelete150Days is the duration a store stays in pending_hard_delete before
// the irreversible hard-delete pipeline fires (§15.2).
const hardDelete150Days = 150 * 24 * time.Hour

// HardDeleteCron invokes the hard-delete runner for each pending_hard_delete
// subscription whose updated_at is older than 150 days (§15.2).
// Idempotent: runner.Run is idempotent per advisory lock + CAS pattern.
type HardDeleteCron struct {
	db      *gorm.DB
	repo    subscription.Repository
	runner  *harddelete.Runner
	emitter *audit.Emitter
	logger  *slog.Logger
	clock   func() time.Time
}

// NewHardDeleteCron constructs a HardDeleteCron.
func NewHardDeleteCron(db *gorm.DB, repo subscription.Repository, runner *harddelete.Runner,
	emitter *audit.Emitter, logger *slog.Logger, clock func() time.Time) *HardDeleteCron {
	if clock == nil {
		clock = func() time.Time { return time.Now().UTC() }
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &HardDeleteCron{
		db: db, repo: repo, runner: runner,
		emitter: emitter, logger: logger, clock: clock,
	}
}

// Run selects pending_hard_delete rows older than 150 days and invokes runner.Run
// on each. Row-level errors are logged and skipped.
func (c *HardDeleteCron) Run(ctx context.Context) error {
	cutoff := c.clock().UTC().Add(-hardDelete150Days)
	var rows []subscription.StoreSubscription
	err := c.db.WithContext(ctx).
		Where("status = ?", subscription.StatusPendingHardDelete).
		Where("updated_at <= ?", cutoff).
		Find(&rows).Error
	if err != nil {
		return err
	}
	c.logger.Info("lifecycle: hard-delete cron started", "eligible", len(rows), "cutoff", cutoff)
	for i := range rows {
		if err := c.runner.Run(ctx, &rows[i]); err != nil {
			c.logger.Error("lifecycle: hard-delete runner failed",
				"store_id", rows[i].StoreID, "tenant_id", rows[i].TenantID, "err", err)
			// Continue — one failure must not block other stores.
		}
	}
	return nil
}
