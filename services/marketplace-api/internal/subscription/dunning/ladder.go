package dunning

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/audit"
	"github.com/mark8ly/marketplace-api/internal/subscription"
	"github.com/mark8ly/marketplace-api/internal/subscription/statemachine"
)

// Day thresholds per §16.2.
const (
	DaysPastDueToExpired           = 8
	DaysExpiredToStoreClosed       = 14
	DaysStoreClosedToPendingDelete = 90
)

// LadderSpec is the cron expression for the daily ladder — runs hourly.
// Idempotent: running twice in the same hour hits CAS conflicts which are
// swallowed.
const LadderSpec = "0 * * * *"

// counterIncrementer is a narrow interface so the ladder can be constructed
// with a Prometheus counter OR a no-op for tests.
type counterIncrementer interface {
	Inc()
}

type noopCounter struct{}

func (noopCounter) Inc() {}

// LadderDecision describes what the ladder should do for a given row.
type LadderDecision struct {
	Target               subscription.SubscriptionStatus
	Reason               string
	Skip                 bool
	SuppressRefundWindow bool
}

// DecideStep is the pure-function decision core of the daily ladder. It
// returns a LadderDecision for a subscription in a given status, given how
// many days the subscription has been in that status and whether it falls
// within the §16.5 refund window. This function is safe to unit-test
// without a database.
func DecideStep(status subscription.SubscriptionStatus, daysSince int, inRefundWindow bool) LadderDecision {
	switch status {
	case subscription.StatusPastDue:
		if daysSince < DaysPastDueToExpired {
			return LadderDecision{Skip: true}
		}
		if inRefundWindow {
			return LadderDecision{Skip: true, SuppressRefundWindow: true}
		}
		return LadderDecision{Target: subscription.StatusExpired, Reason: "dunning_day_8"}

	case subscription.StatusExpired:
		if daysSince < DaysExpiredToStoreClosed {
			return LadderDecision{Skip: true}
		}
		return LadderDecision{Target: subscription.StatusStoreClosed, Reason: "dunning_day_22"}

	case subscription.StatusStoreClosed:
		if daysSince < DaysStoreClosedToPendingDelete {
			return LadderDecision{Skip: true}
		}
		return LadderDecision{Target: subscription.StatusPendingHardDelete, Reason: "dunning_day_112"}
	}
	return LadderDecision{Skip: true}
}

// StepDailyLadder advances the dunning ladder for subscriptions in terminal-ish
// failure states. past_due → expired (with refund-window guard), expired →
// store_closed, store_closed → pending_hard_delete. All mutations go through
// statemachine.Transition.
type StepDailyLadder struct {
	db                *gorm.DB
	emitter           *audit.Emitter
	logger            *slog.Logger
	clock             func() time.Time
	suppressedCounter counterIncrementer
}

// NewStepDailyLadder constructs a StepDailyLadder. All parameters are
// optional with safe defaults: logger defaults to slog.Default(), clock to
// time.Now().UTC(), suppressed to a no-op counter.
func NewStepDailyLadder(db *gorm.DB, em *audit.Emitter, logger *slog.Logger, suppressed counterIncrementer, clock func() time.Time) *StepDailyLadder {
	if logger == nil {
		logger = slog.Default()
	}
	if clock == nil {
		clock = func() time.Time { return time.Now().UTC() }
	}
	if suppressed == nil {
		suppressed = noopCounter{}
	}
	return &StepDailyLadder{
		db:                db,
		emitter:           em,
		logger:            logger,
		clock:             clock,
		suppressedCounter: suppressed,
	}
}

// Run executes a single ladder pass. It fetches every subscription in a
// ladder-eligible status and processes each one under an advisory lock.
// Row-level failures are logged and skipped so one bad row never aborts
// the whole pass.
func (s *StepDailyLadder) Run(ctx context.Context) error {
	now := s.clock().UTC()

	var rows []subscription.StoreSubscription
	if err := s.db.WithContext(ctx).
		Where("status IN ?", []subscription.SubscriptionStatus{
			subscription.StatusPastDue,
			subscription.StatusExpired,
			subscription.StatusStoreClosed,
		}).Find(&rows).Error; err != nil {
		return fmt.Errorf("dunning: ladder query: %w", err)
	}

	for i := range rows {
		if err := s.stepOne(ctx, &rows[i], now); err != nil {
			s.logger.Error("dunning ladder: row failed; continuing",
				"store_id", rows[i].StoreID.String(),
				"status", string(rows[i].Status),
				"err", err.Error())
		}
	}
	return nil
}

// stepOne acquires the store's advisory lock, re-reads the fresh state, and
// calls DecideStep + transition when the threshold has been reached.
func (s *StepDailyLadder) stepOne(ctx context.Context, row *subscription.StoreSubscription, now time.Time) error {
	return subscription.WithAdvisoryLock(ctx, s.db, row.StoreID, func(tx *gorm.DB) error {
		var fresh subscription.StoreSubscription
		if err := tx.Where("store_id = ?", row.StoreID).First(&fresh).Error; err != nil {
			return err
		}

		enteredAt, err := statusEnteredAt(ctx, tx, fresh.StoreID, fresh.Status, fresh.UpdatedAt)
		if err != nil {
			return fmt.Errorf("dunning: status entered_at: %w", err)
		}
		daysSince := int(now.Sub(enteredAt).Hours() / 24)

		d := DecideStep(fresh.Status, daysSince, IsInRefundWindow(&fresh, now))
		if d.Skip {
			if d.SuppressRefundWindow {
				s.suppressedCounter.Inc()
				s.logger.Info("dunning ladder: past_due → expired suppressed (refund window)",
					"store_id", fresh.StoreID.String(),
					"days_since", daysSince)
			}
			return nil
		}

		return s.transition(ctx, tx, &fresh, d.Target, d.Reason)
	})
}

func (s *StepDailyLadder) transition(ctx context.Context, tx *gorm.DB, fresh *subscription.StoreSubscription, to subscription.SubscriptionStatus, reason string) error {
	err := statemachine.Transition(ctx, statemachine.TransitionInput{
		DB:       tx,
		Emitter:  s.emitter,
		TenantID: fresh.TenantID,
		StoreID:  fresh.StoreID,
		From:     fresh.Status,
		To:       to,
		Actor:    "system:cron:dunning_ladder",
		Reason:   reason,
	})
	if errors.Is(err, statemachine.ErrCASConflict) || errors.Is(err, statemachine.ErrInvalidTransition) {
		return nil
	}
	return err
}

// statusEnteredAt returns the timestamp when the subscription entered its
// current status. It prefers the most recent audit_logs entry for a
// subscription.state_transition event that moved the row INTO the given
// status (metadata->>'to_status'). Falls back to updated_at when no audit
// row exists (e.g. legacy subs that predate auditing).
//
// Note: the audit emitter stores "to_status" (not "to_state") per
// audit.EmitStateTransition's metadata keys.
func statusEnteredAt(ctx context.Context, tx *gorm.DB, storeID any, status subscription.SubscriptionStatus, fallback time.Time) (time.Time, error) {
	var enteredAt *time.Time
	err := tx.WithContext(ctx).
		Raw(`
			SELECT MAX(created_at)
			FROM audit_logs
			WHERE action = 'subscription.state_transition'
			  AND store_id = ?
			  AND metadata->>'to_status' = ?`,
			storeID, string(status)).
		Scan(&enteredAt).Error
	if err != nil {
		return fallback, err
	}
	if enteredAt == nil {
		return fallback, nil
	}
	return *enteredAt, nil
}
