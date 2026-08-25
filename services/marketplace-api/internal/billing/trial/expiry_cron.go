package trial

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

// ExpirySpec is the cron expression for the expiry cron: 00:15 UTC daily.
// Late enough that any last-second card-add webhook from the prior day has
// settled; early enough that merchants see expiry at start-of-day.
const ExpirySpec = "15 0 * * *"

// ExpiryCron transitions trialing stores that have no card (stripe_subscription_id
// IS NULL) and whose trial has passed its effective end (EndsAt — normally
// TrialDays, extended if an operator has set trial_ends_at) to the "expired"
// status via the state machine. It is idempotent: stores already in "expired"
// are never selected.
type ExpiryCron struct {
	db      *gorm.DB
	emitter *audit.Emitter
	logger  *slog.Logger
	clock   func() time.Time
}

// NewExpiryCron constructs an ExpiryCron. If clock is nil, time.Now().UTC() is
// used. If logger is nil, slog.Default() is used.
func NewExpiryCron(db *gorm.DB, em *audit.Emitter, logger *slog.Logger, clock func() time.Time) *ExpiryCron {
	if clock == nil {
		clock = func() time.Time { return time.Now().UTC() }
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &ExpiryCron{db: db, emitter: em, logger: logger, clock: clock}
}

// Run selects all trialing stores without a card whose effective trial end
// has passed and transitions each one to "expired" via statemachine.Transition.
// Row-level errors are logged and skipped so one bad row never blocks the rest.
func (c *ExpiryCron) Run(ctx context.Context) error {
	// Effective trial end, not created_at + TrialDays: a trial an operator
	// extended must survive its original day 90. EndedBeforeScope carries
	// both branches — see endsat.go.
	now := c.clock().UTC()
	var rows []subscription.StoreSubscription
	err := EndedBeforeScope(
		c.db.WithContext(ctx).
			Where("status = ?", subscription.StatusTrialing).
			Where("stripe_subscription_id IS NULL"),
		now,
	).Find(&rows).Error
	if err != nil {
		return err
	}
	for i := range rows {
		c.expireOne(ctx, &rows[i])
	}
	return nil
}

func (c *ExpiryCron) expireOne(ctx context.Context, row *subscription.StoreSubscription) {
	err := statemachine.Transition(ctx, statemachine.TransitionInput{
		DB:       c.db,
		Emitter:  c.emitter,
		TenantID: row.TenantID,
		StoreID:  row.StoreID,
		From:     subscription.StatusTrialing,
		To:       subscription.StatusExpired,
		Actor:    "system:cron:trial_expiry",
		Reason:   "trial_ended_no_card",
	})
	switch {
	case err == nil:
		// TODO: trial expiry email deferred — see banner cron comment.
		// TODO: trial lifecycle emails land when StoreSubscription gets email/store_name
		// columns (deferred from P5 scope). For now, log intent for ops visibility.
		c.logger.Info("trial expired; email deferred",
			"store_id", row.StoreID.String(),
			"tenant_id", row.TenantID.String(),
			"template", "trial_expired")
	case errors.Is(err, statemachine.ErrCASConflict):
		// Another writer moved it — likely a last-second webhook from card-add.
		// Intended behaviour; no email, no error log.
	default:
		c.logger.Error("trial expiry: transition failed",
			"store_id", row.StoreID.String(), "err", err)
	}
}
