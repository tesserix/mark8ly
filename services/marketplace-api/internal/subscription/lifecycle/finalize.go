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

	"github.com/google/uuid"
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

// ProAppTeardownNotifier is the delivery path from this cron to whatever
// retires a cancelled Pro+App merchant's mobile app listings.
//
// It is declared HERE, by the emitter, rather than imported from
// internal/whitelabel/lifecycle, for two reasons. Importing the consumer
// package directly would make the subscription lifecycle depend on the
// white-label teardown machinery to compile, which inverts the real
// dependency — teardown is downstream of cancellation, not the other way
// round. And a Pub/Sub topic (what the "deferred" comment this replaced
// pointed at) would add a broker, a subscription, a delivery failure
// mode and an at-least-once contract to an in-process call between two
// packages in the same binary, buying nothing.
//
// Implemented by *whitelabel/lifecycle.ProAppCancelledConsumer; wired in
// cmd/marketplace-api/main.go.
type ProAppTeardownNotifier interface {
	ProAppCancelled(ctx context.Context, tenantID, storeID uuid.UUID) error
}

// proAppNotifyTimeout bounds one store's teardown seeding. The notifier
// makes network calls to App Store Connect; without a bound, one hung
// connection would stall every remaining store's finalisation behind it,
// since finalizeOne runs serially over the cohort.
const proAppNotifyTimeout = 60 * time.Second

// FinalizeCron transitions cancel_scheduled → expired for subscriptions whose
// current_period_end has passed (or is nil — treat as already ended).
// Idempotent: double-runs produce ErrCASConflict which is swallowed.
type FinalizeCron struct {
	db      *gorm.DB
	emitter *audit.Emitter
	logger  *slog.Logger
	clock   func() time.Time
	// notifier is optional: nil means no teardown path is wired, which
	// is logged as a warning at the point it would have been used
	// rather than passing silently.
	notifier ProAppTeardownNotifier
}

// WithProAppTeardownNotifier returns a copy of the cron that hands
// cancelled Pro+App stores to n.
//
// Pass a TRUE nil interface (not a typed nil) to disable — see the
// trialStripe comment in main.go for why a typed nil would defeat the
// nil check below.
func (c *FinalizeCron) WithProAppTeardownNotifier(n ProAppTeardownNotifier) *FinalizeCron {
	next := *c
	next.notifier = n
	return &next
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
			c.onProAppCancelled(ctx, row)
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

// onProAppCancelled records the cancellation and hands the store to the
// teardown notifier.
//
// ORDER IS LOAD-BEARING: the audit event is emitted FIRST and
// unconditionally. Whether the app teardown can be seeded is a separate
// question from whether the cancellation happened, and a teardown that
// fails must not also erase the record that a Pro+App subscription
// ended. The event is the audit trail; the notifier is an action taken
// because of it.
func (c *FinalizeCron) onProAppCancelled(ctx context.Context, row *subscription.StoreSubscription) {
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

	if c.notifier == nil {
		c.logger.Warn("lifecycle: pro-app teardown notifier not wired — the merchant's app listings will not be retired",
			"store_id", row.StoreID, "tenant_id", row.TenantID, "action", ActionProAppCancelled)
		return
	}

	nctx, cancel := context.WithTimeout(ctx, proAppNotifyTimeout)
	defer cancel()
	if err := c.notifier.ProAppCancelled(nctx, row.TenantID, row.StoreID); err != nil {
		// Logged and swallowed: one store whose identifiers could not be
		// resolved must not stall the rest of the cohort's finalisation.
		// The subscription IS expired either way — the transition above
		// already committed — so this is a teardown that did not start,
		// not a cancellation that did not happen.
		c.logger.Error("lifecycle: pro-app teardown not seeded — the merchant's app listings stay live",
			"store_id", row.StoreID, "tenant_id", row.TenantID, "err", err)
		return
	}
	c.logger.Info("lifecycle: pro-app teardown seeded",
		"store_id", row.StoreID, "tenant_id", row.TenantID, "action", ActionProAppCancelled)
}
