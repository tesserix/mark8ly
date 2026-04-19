// Package cancel implements the merchant-initiated subscription cancellation flow
// including the save-offer branch (§15, §15.1 — prospective-only discount).
package cancel

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

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
	Status       string `json:"status"`        // "cancel_scheduled" | "active" (save-offer accepted)
	CancelsAt    string `json:"cancels_at"`    // RFC3339 of current_period_end; "" when save offer accepted
	SaveOfferMsg string `json:"save_offer_msg,omitempty"` // set when save offer was accepted
}

// Service owns the cancellation domain logic.
type Service struct {
	db      *gorm.DB
	repo    subscription.Repository
	emitter *audit.Emitter
	logger  *slog.Logger
}

// NewService constructs a cancel.Service.
func NewService(db *gorm.DB, repo subscription.Repository, emitter *audit.Emitter, logger *slog.Logger) *Service {
	return &Service{db: db, repo: repo, emitter: emitter, logger: logger}
}

// Cancel processes a merchant cancellation request.
//
// Save-offer branch (§15.1):
//   - AcceptSaveOffer=true AND status=cancel_scheduled → transition back to active.
//     The discount (20%-off-6-months) is prospective-only and is NOT applied here —
//     P10's promo service owns that; P11 only drives the state transition and emits
//     the audit event with a note so P10 can wire it later.
//   - AcceptSaveOffer=false (or no offer presented) → transition active → cancel_scheduled
//     setting cancel_at_period_end=true. The subscription expires at Stripe's
//     current_period_end; we do NOT call Stripe here because Stripe already has
//     cancel_at_period_end=true from the webhook layer (or this is a test run).
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

	err := statemachine.Transition(ctx, statemachine.TransitionInput{
		DB:      s.db,
		Emitter: s.emitter,
		TenantID: in.TenantID,
		StoreID:  in.StoreID,
		From:    sub.Status,
		To:      subscription.StatusCancelScheduled,
		Actor:   in.Actor,
		Reason:  reasonLabel("merchant_cancelled", in.SurveyReason),
	})
	if err != nil {
		return Output{}, fmt.Errorf("cancel: transition: %w", err)
	}

	s.logger.Info("subscription cancel scheduled",
		"store_id", in.StoreID,
		"tenant_id", in.TenantID,
		"actor", in.Actor,
		"reason", in.SurveyReason)

	var cancelsAt string
	if sub.CurrentPeriodEnd != nil {
		cancelsAt = sub.CurrentPeriodEnd.UTC().Format("2006-01-02T15:04:05Z")
	}
	return Output{
		Status:    string(subscription.StatusCancelScheduled),
		CancelsAt: cancelsAt,
	}, nil
}

// acceptSaveOffer reverts cancel_scheduled → active (prospective save-offer path).
// The actual promo/discount is NOT applied here; P10 owns that wiring.
func (s *Service) acceptSaveOffer(ctx context.Context, in Input, sub *subscription.StoreSubscription) (Output, error) {
	if sub.Status != subscription.StatusCancelScheduled {
		return Output{}, fmt.Errorf("%w: must be cancel_scheduled to accept save offer, got %s", ErrSaveOfferAlreadyAccepted, sub.Status)
	}

	err := statemachine.Transition(ctx, statemachine.TransitionInput{
		DB:      s.db,
		Emitter: s.emitter,
		TenantID: in.TenantID,
		StoreID:  in.StoreID,
		From:    subscription.StatusCancelScheduled,
		To:      subscription.StatusActive,
		Actor:   in.Actor,
		Reason:  "save_offer_accepted",
	})
	if err != nil {
		return Output{}, fmt.Errorf("cancel: save-offer transition: %w", err)
	}

	// NOTE: P10 promo service would be called here to attach a prospective-only
	// discount (20%-off-6-months). Until P10 is wired, log intent.
	s.logger.Info("cancel: save offer accepted — would apply prospective promo (P10 not yet wired)",
		"store_id", in.StoreID,
		"tenant_id", in.TenantID,
		"actor", in.Actor)

	return Output{
		Status:       string(subscription.StatusActive),
		SaveOfferMsg: "Save offer accepted. A 20% discount applies to your next billing cycle.",
	}, nil
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
