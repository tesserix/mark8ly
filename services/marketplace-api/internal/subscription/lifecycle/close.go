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

// CloseSpec is the cron expression for the 14-day store closure cron: 01:30 UTC daily.
const CloseSpec = "30 1 * * *"

// storeClosed14Days is the grace period before a cancelled store is marked closed.
const storeClosed14Days = 14 * 24 * time.Hour

// CloseCron transitions expired → store_closed for subscriptions that have been
// expired for at least 14 days (§15.2).
// Idempotent: ErrCASConflict and ErrInvalidTransition are swallowed.
type CloseCron struct {
	db      *gorm.DB
	emitter *audit.Emitter
	logger  *slog.Logger
	clock   func() time.Time
}

// NewCloseCron constructs a CloseCron.
func NewCloseCron(db *gorm.DB, emitter *audit.Emitter, logger *slog.Logger, clock func() time.Time) *CloseCron {
	if clock == nil {
		clock = func() time.Time { return time.Now().UTC() }
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &CloseCron{db: db, emitter: emitter, logger: logger, clock: clock}
}

// Run selects expired rows whose updated_at (i.e. the time they entered the
// expired state) is older than 14 days and transitions each to store_closed.
func (c *CloseCron) Run(ctx context.Context) error {
	cutoff := c.clock().UTC().Add(-storeClosed14Days)
	var rows []subscription.StoreSubscription
	err := c.db.WithContext(ctx).
		Where("status = ?", subscription.StatusExpired).
		Where("updated_at <= ?", cutoff).
		Find(&rows).Error
	if err != nil {
		return err
	}
	c.logger.Info("lifecycle: close cron started", "eligible", len(rows), "cutoff", cutoff)
	for i := range rows {
		c.closeOne(ctx, &rows[i])
	}
	return nil
}

func (c *CloseCron) closeOne(ctx context.Context, row *subscription.StoreSubscription) {
	err := statemachine.Transition(ctx, statemachine.TransitionInput{
		DB:       c.db,
		Emitter:  c.emitter,
		TenantID: row.TenantID,
		StoreID:  row.StoreID,
		From:     subscription.StatusExpired,
		To:       subscription.StatusStoreClosed,
		Actor:    "system:cron:lifecycle_close",
		Reason:   "lifecycle:expired_14d",
	})
	switch {
	case err == nil:
		c.logger.Info("lifecycle: store closed (14d post-expiry)",
			"store_id", row.StoreID, "tenant_id", row.TenantID)
	case errors.Is(err, statemachine.ErrCASConflict):
		// Another writer moved it — skip.
	case errors.Is(err, statemachine.ErrInvalidTransition):
		// Already closed — skip.
	default:
		c.logger.Error("lifecycle: close transition failed",
			"store_id", row.StoreID, "err", err)
	}
}
