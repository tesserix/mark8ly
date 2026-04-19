// Package lifecycle implements the four post-cancellation idempotent daily crons
// that drive the subscription lifecycle pipeline (§15.2):
//
//	01:00 UTC — finalize cancellation: cancel_scheduled → expired
//	01:30 UTC — 14-day closure:        expired → store_closed
//	02:00 UTC — 90-day queue:          store_closed → pending_hard_delete
//	03:00 UTC — 150-day hard delete:   pending_hard_delete → hard_deleted  (via harddelete runner)
//	10:00 UTC — win-back:              day-30 post-expiry promo email
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

// FinalizeSpec is the cron expression for the finalize cancellation cron: 01:00 UTC daily.
const FinalizeSpec = "0 1 * * *"

// ActionProAppCancelled is the audit action emitted when a Pro+App subscription
// is finalized (cancel_scheduled → expired with has_white_label_app_add_on=true).
// P15 listens for this event to trigger app teardown.
const ActionProAppCancelled = "subscription.pro_app_cancelled"

// FinalizeCron transitions cancel_scheduled → expired for subscriptions whose
// current_period_end has passed (or is nil — treat as already ended).
// Idempotent: double-runs produce ErrCASConflict which is swallowed.
type FinalizeCron struct {
	db      *gorm.DB
	emitter *audit.Emitter
	logger  *slog.Logger
	clock   func() time.Time
}

// NewFinalizeCron constructs a FinalizeCron.
func NewFinalizeCron(db *gorm.DB, emitter *audit.Emitter, logger *slog.Logger, clock func() time.Time) *FinalizeCron {
	if clock == nil {
		clock = func() time.Time { return time.Now().UTC() }
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &FinalizeCron{db: db, emitter: emitter, logger: logger, clock: clock}
}

// Run selects cancel_scheduled rows whose current_period_end <= now (or is NULL)
// and transitions each one to expired.
func (c *FinalizeCron) Run(ctx context.Context) error {
	now := c.clock().UTC()
	var rows []subscription.StoreSubscription
	err := c.db.WithContext(ctx).
		Where("status = ?", subscription.StatusCancelScheduled).
		Where("current_period_end IS NULL OR current_period_end <= ?", now).
		Find(&rows).Error
	if err != nil {
		return err
	}
	c.logger.Info("lifecycle: finalize cron started", "eligible", len(rows), "now", now)
	for i := range rows {
		c.finalizeOne(ctx, &rows[i])
	}
	return nil
}

func (c *FinalizeCron) finalizeOne(ctx context.Context, row *subscription.StoreSubscription) {
	err := statemachine.Transition(ctx, statemachine.TransitionInput{
		DB:       c.db,
		Emitter:  c.emitter,
		TenantID: row.TenantID,
		StoreID:  row.StoreID,
		From:     subscription.StatusCancelScheduled,
		To:       subscription.StatusExpired,
		Actor:    "system:cron:lifecycle_finalize",
		Reason:   "lifecycle:finalize_cancel",
	})
	switch {
	case err == nil:
		c.logger.Info("lifecycle: finalize cancel — subscription expired",
			"store_id", row.StoreID, "tenant_id", row.TenantID)
		// Pro+App hook: if the store had the white-label app add-on, emit
		// subscription.pro_app_cancelled so P15 can trigger app teardown.
		if row.HasWhiteLabelAppAddOn {
			c.emitter.Emit(nil, audit.Event{
				Action:         ActionProAppCancelled,
				ResourceType:   "subscription",
				ResourceID:     row.StoreID.String(),
				Severity:       audit.SeverityWarning,
				TenantID:       row.TenantID,
				StoreID:        row.StoreID,
				ForceActorType: audit.ActorSystem,
				Metadata: map[string]any{
					"reason":   "cancel_scheduled_finalized",
					"store_id": row.StoreID.String(),
				},
			})
			// P15 listens for this audit event (or pub/sub topic — deferred).
			// Until P15 is wired, log intent so ops can see it.
			c.logger.Info("lifecycle: would notify pro-app service",
				"store_id", row.StoreID,
				"action", ActionProAppCancelled)
		}
	case errors.Is(err, statemachine.ErrCASConflict):
		// Another writer already moved this row — skip silently.
	case errors.Is(err, statemachine.ErrInvalidTransition):
		// Row was already expired by another path — skip.
	default:
		c.logger.Error("lifecycle: finalize cancel transition failed",
			"store_id", row.StoreID, "err", err)
	}
}
