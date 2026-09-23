// Package cancel implements the merchant-initiated subscription cancellation flow
// including the save-offer branch (§15, §15.1 — prospective-only discount).
package cancel

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/audit"
	"github.com/mark8ly/marketplace-api/internal/subscription"
	"github.com/mark8ly/marketplace-api/internal/subscription/statemachine"
)

// ErrNotCancellable is returned when the subscription is not in a state from
// which cancellation is permitted (must be active or trialing).
var ErrNotCancellable = errors.New("cancel: subscription not in a cancellable state")

// ErrSaveOfferAlreadyAccepted is returned when the merchant accepts the save
// offer but cancel_at_period_end is already false (i.e. they previously
// accepted and we're getting a duplicate submit).
var ErrSaveOfferAlreadyAccepted = errors.New("cancel: save offer already accepted")

// Input describes a cancellation request.
type Input struct {
	TenantID        uuid.UUID
	StoreID         uuid.UUID
	Actor           string // "user:<uuid>"
	SurveyReason    string // optional free-text from the exit survey; max 256 chars
	AcceptSaveOffer bool   // true → revert cancel; false → schedule cancel at period end
}

// Output reports what happened.
type Output struct {
	Status       string `json:"status"`                   // "cancel_scheduled" | "active" | "trialing" (save-offer accepted mid-trial)
	CancelsAt    string `json:"cancels_at"`               // RFC3339 of current_period_end; "" when save offer accepted
	SaveOfferMsg string `json:"save_offer_msg,omitempty"` // set when save offer was accepted
}

// Service owns the cancellation domain logic.
type Service struct {
	db      *gorm.DB
	repo    subscription.Repository
	emitter *audit.Emitter
	logger  *slog.Logger
	// promo is optional. When nil the save offer reverses the cancellation and
	// claims no discount; see WithPromo in save_offer.go.
	promo PromoApplier
	// stripe is optional only for stores Stripe is not billing. A row that
	// carries a stripe_subscription_id and finds this nil is refused rather
	// than cancelled locally; see requireStripe in stripe.go.
	stripe StripeCanceller
	// clock is injectable so a test can put a subscription either side of
	// its trial end without waiting ninety days. Nil means time.Now.
	clock func() time.Time
}

// WithClock returns a copy of the Service that reads the time from c. For
// tests; production leaves it nil and gets time.Now.
func (s *Service) WithClock(c func() time.Time) *Service {
	if s == nil || c == nil {
		return s
	}
	cp := *s
	cp.clock = c
	return &cp
}

func (s *Service) now() time.Time {
	if s.clock != nil {
		return s.clock().UTC()
	}
	return time.Now().UTC()
}

// requireStripe returns the Stripe subscription id this row is billed under,
// and whether Stripe has to be told about this cancellation at all.
//
// A store with no stripe_subscription_id is one Stripe never billed — a trial
// that added no card, or a local/dev row — and cancelling it is purely a
// local state change. Everything else is real money: if the canceller is not
// wired, refuse, because the alternative is telling a paying merchant they
// have cancelled while Stripe keeps charging them.
func (s *Service) requireStripe(sub *subscription.StoreSubscription) (string, bool, error) {
	if sub == nil || sub.StripeSubscriptionID == nil || *sub.StripeSubscriptionID == "" {
		return "", false, nil
	}
	if s.stripe == nil {
		return "", false, ErrStripeNotWired
	}
	return *sub.StripeSubscriptionID, true, nil
}

// NewService constructs a cancel.Service.
func NewService(db *gorm.DB, repo subscription.Repository, emitter *audit.Emitter, logger *slog.Logger) *Service {
	return &Service{db: db, repo: repo, emitter: emitter, logger: logger}
}

// Cancel processes a merchant cancellation request.
//
// Save-offer branch (§15.1):
//   - AcceptSaveOffer=true AND status=cancel_scheduled → clear the schedule at
//     Stripe, then transition back to where the subscription came from: active,
//     or trialing while the trial is still running (see restoredStatus). The
//     discount (20%-off-6-months) is prospective-only. It is attempted after the
//     transition via the optional promo dependency, and the response claims a
//     discount only when one was actually applied (#701).
//   - AcceptSaveOffer=false (or no offer presented) → set cancel_at_period_end
//     at Stripe FIRST, then transition active|trialing → cancel_scheduled. The
//     subscription expires at the period end Stripe reports, which for a
//     cancelled trial is the trial end — cancelling mid-trial is what stops the
//     day-90 deferred charge.
//
// NOTE: cancellation_reason is intentionally NOT persisted on the
// store_subscriptions row. The audit_logs table is the system of record for
// reasons (per plan §2 comment). EmitStateTransition carries it in metadata.
func (s *Service) Cancel(ctx context.Context, in Input) (Output, error) {
	if len(in.SurveyReason) > 256 {
		in.SurveyReason = in.SurveyReason[:256]
	}

	sub, err := s.repo.GetByStoreID(ctx, s.db, in.TenantID, in.StoreID)
	if err != nil {
		return Output{}, fmt.Errorf("cancel: find subscription: %w", err)
	}

	if in.AcceptSaveOffer {
		return s.acceptSaveOffer(ctx, in, sub)
	}
	return s.scheduleCancellation(ctx, in, sub)
}

// scheduleCancellation transitions active → cancel_scheduled.
func (s *Service) scheduleCancellation(ctx context.Context, in Input, sub *subscription.StoreSubscription) (Output, error) {
	if sub.Status != subscription.StatusActive && sub.Status != subscription.StatusTrialing {
		return Output{}, fmt.Errorf("%w: current status=%s", ErrNotCancellable, sub.Status)
	}

	// Asked before Stripe, so a state the machine will refuse never causes a
	// cancellation at Stripe that the local row cannot record.
	//
	// This guard was added when §15 and §17.2 disagreed about trialing and
	// the mismatch surfaced as a 500. That contradiction is resolved — the
	// table now carries trialing → cancel_scheduled — but the check stays:
	// the status guard above and the transition table are two separate
	// statements of what is cancellable, and they can drift again. Better a
	// 409 naming the reason than a 500 out of the state machine.
	if !statemachine.IsValidTransition(sub.Status, subscription.StatusCancelScheduled) {
		return Output{}, fmt.Errorf("%w: no %s → %s transition (§17.2)",
			ErrNotCancellable, sub.Status, subscription.StatusCancelScheduled)
	}

	// Stripe first, and the local transition only if it took.
	//
	// The other order is what shipped: the row said cancel_scheduled while
	// Stripe went on charging. Failing here leaves a merchant still
	// subscribed and still billed, which is recoverable by retrying;
	// succeeding here and failing below leaves them billed to the period end
	// and then not billed at all, which costs them nothing. Neither outcome
	// takes money for access that has stopped.
	stripeSubID, billed, err := s.requireStripe(sub)
	if err != nil {
		return Output{}, err
	}
	periodEnd := sub.CurrentPeriodEnd
	if billed {
		state, err := s.stripe.CancelAtPeriodEnd(ctx, stripeSubID)
		if err != nil {
			s.logger.Error("cancel: stripe would not schedule the cancellation — local row untouched",
				"store_id", in.StoreID, "tenant_id", in.TenantID,
				"stripe_subscription_id", stripeSubID, "err", err)
			return Output{}, fmt.Errorf("%w: %v", ErrStripeUnavailable, err)
		}
		// Stripe owns the period end, and this response is the only place
		// it is reliably known: it is written to the row at subscribe time
		// and by webhooks, but a row that missed both would otherwise be
		// finalised on the next cron tick (current_period_end IS NULL is
		// treated as already ended) — i.e. access lost immediately, which
		// is the half of the defect the merchant actually feels.
		if !state.CurrentPeriodEnd.IsZero() {
			end := state.CurrentPeriodEnd
			periodEnd = &end
		}
		if err := s.persistCancellationSchedule(ctx, in, periodEnd); err != nil {
			// The cancellation IS scheduled at Stripe; the merchant is not
			// being charged again. Carry on and let the webhook reconcile
			// the columns rather than failing a request that succeeded.
			s.logger.Error("cancel: scheduled at stripe but the local period end could not be persisted",
				"store_id", in.StoreID, "tenant_id", in.TenantID, "err", err)
		}
	}

	err = statemachine.Transition(ctx, statemachine.TransitionInput{
		DB:       s.db,
		Emitter:  s.emitter,
		TenantID: in.TenantID,
		StoreID:  in.StoreID,
		From:     sub.Status,
		To:       subscription.StatusCancelScheduled,
		Actor:    in.Actor,
		Reason:   reasonLabel("merchant_cancelled", in.SurveyReason),
	})
	if err != nil {
		// The pre-check above makes this reachable only by a concurrent
		// writer moving the row underneath us, so it stays a plain failure.
		return Output{}, fmt.Errorf("cancel: transition: %w", err)
	}

	s.logger.Info("subscription cancel scheduled",
		"store_id", in.StoreID,
		"tenant_id", in.TenantID,
		"actor", in.Actor,
		"billed_by_stripe", billed,
		"reason", in.SurveyReason)

	var cancelsAt string
	if periodEnd != nil {
		cancelsAt = periodEnd.UTC().Format("2006-01-02T15:04:05Z")
	}
	return Output{
		Status:    string(subscription.StatusCancelScheduled),
		CancelsAt: cancelsAt,
	}, nil
}

// persistCancellationSchedule mirrors what Stripe now holds onto the local
// row, so FinalizeCron expires the subscription on the date the merchant was
// told and not before.
func (s *Service) persistCancellationSchedule(ctx context.Context, in Input, periodEnd *time.Time) error {
	fields := map[string]any{
		"cancel_at_period_end": true,
		"updated_at":           time.Now().UTC(),
	}
	if periodEnd != nil {
		fields["current_period_end"] = *periodEnd
	}
	return s.db.WithContext(ctx).
		Model(&subscription.StoreSubscription{}).
		Where("tenant_id = ? AND store_id = ?", in.TenantID, in.StoreID).
		Updates(fields).Error
}

// acceptSaveOffer reverts cancel_scheduled → active (prospective save-offer path).
// The discount is best-effort: it is attempted only after the reversal has been
// committed, and a failure downgrades the message rather than failing the call.
func (s *Service) acceptSaveOffer(ctx context.Context, in Input, sub *subscription.StoreSubscription) (Output, error) {
	if sub.Status != subscription.StatusCancelScheduled {
		return Output{}, fmt.Errorf("%w: must be cancel_scheduled to accept save offer, got %s", ErrSaveOfferAlreadyAccepted, sub.Status)
	}

	// Clear the schedule at Stripe before promising the merchant their
	// subscription stays. They un-cancel in reliance on that sentence, so it
	// must not be said while Stripe still intends to stop billing them at
	// the period end.
	stripeSubID, billed, err := s.requireStripe(sub)
	if err != nil {
		return Output{}, err
	}
	if billed {
		if _, err := s.stripe.Resume(ctx, stripeSubID); err != nil {
			s.logger.Error("cancel: stripe would not clear the scheduled cancellation — reversal refused",
				"store_id", in.StoreID, "tenant_id", in.TenantID,
				"stripe_subscription_id", stripeSubID, "err", err)
			return Output{}, fmt.Errorf("%w: %v", ErrStripeUnavailable, err)
		}
		if err := s.clearCancellationSchedule(ctx, in); err != nil {
			s.logger.Error("cancel: reversed at stripe but the local flag could not be cleared",
				"store_id", in.StoreID, "tenant_id", in.TenantID, "err", err)
		}
	}

	restored := restoredStatus(sub, s.now())
	err = statemachine.Transition(ctx, statemachine.TransitionInput{
		DB:       s.db,
		Emitter:  s.emitter,
		TenantID: in.TenantID,
		StoreID:  in.StoreID,
		From:     subscription.StatusCancelScheduled,
		To:       restored,
		Actor:    in.Actor,
		Reason:   "save_offer_accepted",
	})
	if err != nil {
		return Output{}, fmt.Errorf("cancel: save-offer transition: %w", err)
	}

	// The reversal is committed. Attempt the prospective-only discount; whether
	// it lands only changes what we tell the merchant, never the reversal.
	discountApplied := s.applySaveOfferDiscount(ctx, in, sub)

	s.logger.Info("cancel: save offer accepted",
		"store_id", in.StoreID,
		"tenant_id", in.TenantID,
		"actor", in.Actor,
		"restored_status", restored,
		"discount_applied", discountApplied)

	return saveOfferOutput(restored, discountApplied), nil
}

// clearCancellationSchedule drops the local cancel_at_period_end flag after
// Stripe has cleared its own. The period end is left as it is: the
// subscription continues, so the date it renews on is still the truth.
func (s *Service) clearCancellationSchedule(ctx context.Context, in Input) error {
	return s.db.WithContext(ctx).
		Model(&subscription.StoreSubscription{}).
		Where("tenant_id = ? AND store_id = ?", in.TenantID, in.StoreID).
		Updates(map[string]any{
			"cancel_at_period_end": false,
			"updated_at":           time.Now().UTC(),
		}).Error
}

// IsCancellableStatus reports whether a subscription in the given status may be
// cancelled by the merchant. Active and trialing are the only cancellable states
// (§15). Exported so tests can assert the guard without a real DB.
func IsCancellableStatus(s subscription.SubscriptionStatus) bool {
	return s == subscription.StatusActive || s == subscription.StatusTrialing
}

// TruncateReason truncates a survey reason to 256 chars. Exported for tests.
func TruncateReason(s string) string {
	if len(s) > 256 {
		return s[:256]
	}
	return s
}

// ReasonLabel composes the audit reason from the cron/source label and the
// optional merchant-supplied survey reason. Exported for tests.
func ReasonLabel(source, survey string) string {
	return reasonLabel(source, survey)
}

// reasonLabel composes the audit reason from the cron/source label and the
// optional merchant-supplied survey reason.
func reasonLabel(source, survey string) string {
	if survey == "" {
		return source
	}
	return source + ": " + survey
}
