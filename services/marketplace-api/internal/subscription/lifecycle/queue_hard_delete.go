package lifecycle

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/audit"
	"github.com/mark8ly/marketplace-api/internal/subscription"
	"github.com/mark8ly/marketplace-api/internal/subscription/statemachine"
)

// QueueHardDeleteSpec is the cron expression for the 90-day hard-delete queue cron: 02:00 UTC daily.
const QueueHardDeleteSpec = "0 2 * * *"

// pendingHardDelete90Days is the duration a store stays in store_closed before
// being queued for hard deletion (§15.2).
const pendingHardDelete90Days = 90 * 24 * time.Hour

// QueueHardDeleteCron transitions store_closed → pending_hard_delete for stores
// that have been closed for at least 90 days (§15.2).
// Idempotent: ErrCASConflict and ErrInvalidTransition are swallowed.
type QueueHardDeleteCron struct {
	db      *gorm.DB
	emitter *audit.Emitter
	logger  *slog.Logger
	clock   func() time.Time
}

// NewQueueHardDeleteCron constructs a QueueHardDeleteCron.
func NewQueueHardDeleteCron(db *gorm.DB, emitter *audit.Emitter, logger *slog.Logger, clock func() time.Time) *QueueHardDeleteCron {
	if clock == nil {
		clock = func() time.Time { return time.Now().UTC() }
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &QueueHardDeleteCron{db: db, emitter: emitter, logger: logger, clock: clock}
}

// Run selects store_closed rows whose updated_at is older than 90 days and
// transitions each to pending_hard_delete.
func (c *QueueHardDeleteCron) Run(ctx context.Context) error {
	cutoff := c.clock().UTC().Add(-pendingHardDelete90Days)
	var rows []subscription.StoreSubscription
	err := c.db.WithContext(ctx).
		Where("status = ?", subscription.StatusStoreClosed).
		Where("updated_at <= ?", cutoff).
		Find(&rows).Error
	if err != nil {
		return err
	}
	c.logger.Info("lifecycle: queue-hard-delete cron started", "eligible", len(rows), "cutoff", cutoff)
	for i := range rows {
		c.queueOne(ctx, &rows[i])
	}
	return nil
}

func (c *QueueHardDeleteCron) queueOne(ctx context.Context, row *subscription.StoreSubscription) {
	err := statemachine.Transition(ctx, statemachine.TransitionInput{
		DB:       c.db,
		Emitter:  c.emitter,
		TenantID: row.TenantID,
		StoreID:  row.StoreID,
		From:     subscription.StatusStoreClosed,
		To:       subscription.StatusPendingHardDelete,
		Actor:    "system:cron:lifecycle_queue_hard_delete",
		Reason:   "lifecycle:store_closed_90d",
	})
	switch {
	case err == nil:
		c.logger.Info("lifecycle: store queued for hard delete (90d post-expiry)",
			"store_id", row.StoreID, "tenant_id", row.TenantID)
	case errors.Is(err, statemachine.ErrCASConflict):
		// Another writer moved it — skip.
	case errors.Is(err, statemachine.ErrInvalidTransition):
		// Already queued — skip.
	default:
		c.logger.Error("lifecycle: queue-hard-delete transition failed",
			"store_id", row.StoreID, "err", err)
	}
}
