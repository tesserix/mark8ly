package promo

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	billingstripe "github.com/mark8ly/marketplace-api/internal/billing/stripe"
	"github.com/mark8ly/marketplace-api/internal/billing/trial"
	"github.com/mark8ly/marketplace-api/internal/metrics"
	"github.com/mark8ly/marketplace-api/internal/subscription"
	"github.com/mark8ly/marketplace-api/pkg/apperrors"
)

// ApplyInput is the input to Service.ApplyPromo.
type ApplyInput struct {
	TenantID       uuid.UUID
	StoreID        uuid.UUID
	SubscriptionID uuid.UUID
	// Code is the human-readable promo code submitted by the merchant.
	Code string
	// MerchantEmail is used for per-email redemption tracking.
	MerchantEmail string
	// Plan is the store's current subscription plan (for plan + floor checks).
	Plan subscription.SubscriptionPlan
	// Period is the store's billing period (for annual-only check).
	Period subscription.SubscriptionPeriod
	// BasePriceMinor is the undiscounted recurring price in minor units.
	BasePriceMinor int64
	// Currency is the store's billing currency (ISO 4217 lower-case).
	Currency string
	// StripeSubscriptionID is the Stripe subscription to attach the coupon to.
	StripeSubscriptionID string
	// Actor is the audit actor string ("user:<uuid>" or "system:…").
	Actor string
	// Sub is the store subscription this code is being applied to.
	//
	// Needed ONLY by a code that carries a trial extension, and needed in
	// full rather than as a copied-out date: both "may this trial move" and
	// "what does it move from" are properties of the row, and each has
	// exactly one definition (trial.Extendable, trial.EndsAt). Passing
	// scalars would put a second copy of each here.
	//
	// Nil is correct for a discount-only code. Nil with a trial-extension
	// code is a call-site bug and fails closed — see ApplyPromo.
	Sub *subscription.StoreSubscription
}

// ApplyOutput is returned by Service.ApplyPromo on success.
type ApplyOutput struct {
	PromoCodeID uuid.UUID
	// StripeCouponID is the Stripe Coupon backing the code, or "" when the
	// code has none (trial-extension-only, or not minted in this Stripe mode).
	// "" is not a valid Stripe coupon id, so it is unambiguously "no coupon".
	StripeCouponID string
	EffectiveMinor int64
	// RejectReason is empty on success; set when the code was rejected (for audit).
	RejectReason ValidationRejectReason
	// PercentOffBps is the code's percentage discount in basis points (2000
	// = 20%), or 0 when the code carries no percentage discount — it may
	// carry a flat amount instead, or none at all. Exposed so a caller that
	// has to DESCRIBE the offer (the day-30 win-back email, #727) states the
	// row's own number rather than one written into prose.
	PercentOffBps int
	// MaxDurationMonths is how many months the discount runs for, or 0 when
	// the row sets no bound. 0 is "unbounded", never "zero months".
	MaxDurationMonths int
	// TrialExtensionDays is the number of days this code adds to the trial,
	// or 0 when it extends none. Set by terms(), so ValidateCode reports the
	// same number ApplyPromo would grant — a client can state "+14 days"
	// before the merchant commits.
	TrialExtensionDays int
	// TrialEndsAt is the trial end AFTER the extension was applied. Set by
	// ApplyPromo alone and zero everywhere else, including ValidateCode:
	// asking whether a code would be accepted grants no date, and reporting
	// one would invite a client to display a trial end that was never
	// written.
	TrialEndsAt time.Time
}

// terms copies the describable parts of a promo row into an output. Kept in
// one place so ApplyPromo and ValidateCode cannot describe the same row
// differently.
func terms(out ApplyOutput, pc *PromoCode) ApplyOutput {
	if pc == nil {
		return out
	}
	if pc.StripeCouponID != nil {
		out.StripeCouponID = *pc.StripeCouponID
	}
	if pc.DiscountType != nil && *pc.DiscountType == DiscountTypePercentage && pc.DiscountValue != nil {
		out.PercentOffBps = *pc.DiscountValue
	}
	if pc.MaxDurationMonths != nil {
		out.MaxDurationMonths = *pc.MaxDurationMonths
	}
	if pc.TrialExtensionDays != nil {
		out.TrialExtensionDays = *pc.TrialExtensionDays
	}
	return out
}

// CancelInput is the input to Service.CancelPromo.
type CancelInput struct {
	TenantID             uuid.UUID
	StoreID              uuid.UUID
	PromoCodeID          uuid.UUID
	StripeSubscriptionID string
	Actor                string
}

// TrialExtender is the subset of *trial.Extender this package needs. Declared
// here rather than imported as a concrete type so a redemption can be tested
// without a database, and so the dependency points inward.
type TrialExtender interface {
	Extend(ctx context.Context, db *gorm.DB, storeID uuid.UUID,
		newEnd, now time.Time, callerIdemKey string) (trial.ExtendResult, error)
}

// Service is the promo-code application service.
type Service struct {
	db     *gorm.DB
	repo   Repository
	stripe *billingstripe.Client
	trial  TrialExtender
	logger *slog.Logger
}

// NewService constructs a Service. stripeClient may be nil (Stripe calls are
// skipped with a warning) for unit-test environments without a real key.
func NewService(db *gorm.DB, repo Repository, stripe *billingstripe.Client, logger *slog.Logger) *Service {
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{db: db, repo: repo, stripe: stripe, logger: logger}
}

// WithTrialExtender wires the trial extender a trial-extension code needs
// (#620) and returns s, so it can be chained onto NewService.
//
// Deliberately NOT a NewService parameter. A nil Stripe client is a supported
// configuration — a code with no coupon simply attaches none — but a nil
// extender is not: a code that promises days and silently grants none is the
// exact defect #620 was opened for. Keeping it off the constructor means the
// four existing call sites that pass no extender keep compiling, while
// ApplyPromo refuses rather than degrades. See the trial branch there.
func (s *Service) WithTrialExtender(e TrialExtender) *Service {
	s.trial = e
	return s
}

// ApplyPromo applies a promo code to a store subscription. Returns
// ErrInvalidOrExpired (uniform) for any validation failure so the caller
// cannot distinguish the failure mode from the HTTP response. The true
// reject reason is captured in ApplyOutput.RejectReason for the audit emitter.
func (s *Service) ApplyPromo(ctx context.Context, in ApplyInput) (ApplyOutput, error) {
	// 1. Look up the promo code row.
	pc, err := s.repo.GetByCode(ctx, s.db, in.Code)
	if err != nil {
		if errors.As(err, new(*apperrors.Error)) {
			// Not found — return uniform error; don't reveal "code doesn't exist".
			return ApplyOutput{RejectReason: RejectReasonNotFound}, ErrInvalidOrExpired
		}
		return ApplyOutput{}, fmt.Errorf("promo: apply: lookup code: %w", err)
	}

	// 2. Count global and per-email redemptions.
	totalRed, err := s.repo.CountRedemptions(ctx, s.db, pc.ID)
	if err != nil {
		return ApplyOutput{}, fmt.Errorf("promo: apply: count redemptions: %w", err)
	}
	emailRed, err := s.repo.CountRedemptionsByEmail(ctx, s.db, pc.ID, normaliseEmail(in.MerchantEmail))
	if err != nil {
		return ApplyOutput{}, fmt.Errorf("promo: apply: count email redemptions: %w", err)
	}

	// 3. Run timing-safe validation.
	result := Validate(ValidationInput{
		SubmittedCode:    in.Code,
		PromoCode:        pc,
		Now:              time.Now().UTC(),
		TotalRedemptions: totalRed,
		EmailRedemptions: emailRed,
		Plan:             in.Plan,
		Period:           in.Period,
		BasePriceMinor:   in.BasePriceMinor,
		Currency:         in.Currency,
	})
	if !result.Accepted {
		if metrics.Subscription != nil {
			metrics.Subscription.PromoAppliedTotal.
				WithLabelValues(string(in.Plan), in.Currency, string(result.RejectReason)).Inc()
		}
		return ApplyOutput{RejectReason: result.RejectReason}, ErrInvalidOrExpired
	}

	// 4. Check this store hasn't already redeemed this code.
	_, storeRedErr := s.repo.GetRedemptionByStore(ctx, s.db, pc.ID, in.StoreID)
	if storeRedErr == nil {
		// Row found — already applied.
		if metrics.Subscription != nil {
			metrics.Subscription.PromoAppliedTotal.
				WithLabelValues(string(in.Plan), in.Currency, string(RejectReasonMaxPerEmail)).Inc()
		}
		return ApplyOutput{RejectReason: RejectReasonMaxPerEmail}, ErrInvalidOrExpired
	}
	if !errors.As(storeRedErr, new(*apperrors.Error)) {
		return ApplyOutput{}, fmt.Errorf("promo: apply: check store redemption: %w", storeRedErr)
	}

	// 4b. Pre-flight the trial extension, BEFORE anything is written.
	//
	// The refusal has to happen here rather than at the Extend call below,
	// because by then the ledger row exists and max_per_email is 1: a
	// merchant refused after the row is written has spent their one
	// redemption on nothing. Extend re-checks under its row lock, so this is
	// the merchant's answer, not the guarantee.
	extendDays, newTrialEnd, err := s.planTrialExtension(pc, in)
	if err != nil {
		if errors.Is(err, ErrInvalidOrExpired) {
			if metrics.Subscription != nil {
				metrics.Subscription.PromoAppliedTotal.
					WithLabelValues(string(in.Plan), in.Currency, string(RejectReasonTrialNotExtendable)).Inc()
			}
			return ApplyOutput{RejectReason: RejectReasonTrialNotExtendable}, err
		}
		return ApplyOutput{}, err
	}

	// 5. Attach coupon in Stripe (if client available and the code has one).
	//
	// A console-defined code need not have a Stripe Coupon: a
	// trial-extension-only code never does, and the console omits the coupon
	// id for a code not minted in the current Stripe mode (#726). There is
	// nothing to attach in that case — attaching an empty coupon id would be
	// a Stripe API error, and inventing one would be worse.
	couponID := ""
	if pc.StripeCouponID != nil {
		couponID = *pc.StripeCouponID
	}
	if s.stripe != nil && in.StripeSubscriptionID != "" && couponID != "" {
		if err := billingstripe.AddSubscriptionDiscount(ctx, s.stripe, in.StripeSubscriptionID, couponID); err != nil {
			return ApplyOutput{}, fmt.Errorf("promo: apply: stripe add subscription discount: %w", err)
		}
		s.logger.Info("promo: coupon added to stripe subscription discounts",
			"store_id", in.StoreID,
			"coupon_id", couponID,
			"stripe_sub_id", in.StripeSubscriptionID)
	} else if couponID == "" {
		s.logger.Info("promo: code carries no stripe coupon — nothing to attach",
			"store_id", in.StoreID,
			"promo_code_id", pc.ID)
	} else {
		s.logger.Warn("promo: stripe client nil or no subscription id — skipping Stripe coupon attach",
			"store_id", in.StoreID)
	}

	// 6. Record redemption row.
	red := &Redemption{
		// Generated here rather than left to the column default, because
		// the trial extension's idempotency key is derived from it (#620)
		// and a key cannot be built from an id the database has not
		// returned yet.
		ID:             uuid.New(),
		PromoCodeID:    pc.ID,
		StoreID:        in.StoreID,
		SubscriptionID: in.SubscriptionID,
		Email:          normaliseEmail(in.MerchantEmail),
		RedeemedAt:     time.Now().UTC(),
	}
	if err := s.repo.CreateRedemption(ctx, s.db, red); err != nil {
		// Best-effort rollback: remove what we just added, and only when we
		// actually added something — a code with no coupon added nothing.
		// RemoveSubscriptionDiscount takes out that coupon's discount alone,
		// so an unrelated coupon the subscription already carried survives.
		if s.stripe != nil && in.StripeSubscriptionID != "" && couponID != "" {
			_ = billingstripe.RemoveSubscriptionDiscount(ctx, s.stripe, in.StripeSubscriptionID, couponID)
		}
		return ApplyOutput{}, fmt.Errorf("promo: apply: record redemption: %w", err)
	}

	// 7. Extend the trial, keyed on the ledger row just written (#620).
	//
	// After the row, not before, so the key exists and a retry converges on
	// the same Stripe idempotency key rather than extending twice.
	appliedEnd := time.Time{}
	if extendDays > 0 {
		if _, err := s.trial.Extend(ctx, s.db, in.StoreID, newTrialEnd, time.Now().UTC(),
			"promo_redeem:"+red.ID.String()); err != nil {

			// The ONE failure that must not be undone. Stripe has already
			// moved the merchant's billing date and only the local write
			// failed; deleting the ledger row would erase the only local
			// record that the redemption happened, and the retry it invites
			// is refused by Extend anyway (ErrTrialEndNotAfterStripe). A
			// human reconciles this — see trial.ErrStripeAppliedLocalWriteFailed.
			if errors.Is(err, trial.ErrStripeAppliedLocalWriteFailed) {
				s.logger.Error("promo: trial extension moved stripe but not the local row — redemption kept for reconciliation",
					"store_id", in.StoreID, "promo_code_id", pc.ID,
					"redemption_id", red.ID, "err", err)
				return ApplyOutput{}, fmt.Errorf("promo: apply: extend trial: %w", err)
			}

			// Everything else: put the merchant back where they started, so
			// the code is not spent on an extension that never happened.
			if delErr := s.repo.DeleteRedemptionByStore(ctx, s.db, pc.ID, in.StoreID); delErr != nil {
				s.logger.Error("promo: could not roll back redemption after a failed trial extension — the code is now spent",
					"store_id", in.StoreID, "promo_code_id", pc.ID, "err", delErr)
			}
			if s.stripe != nil && in.StripeSubscriptionID != "" && couponID != "" {
				_ = billingstripe.RemoveSubscriptionDiscount(ctx, s.stripe, in.StripeSubscriptionID, couponID)
			}
			return ApplyOutput{}, fmt.Errorf("promo: apply: extend trial: %w", err)
		}
		appliedEnd = newTrialEnd
		s.logger.Info("promo: trial extended",
			"store_id", in.StoreID, "promo_code_id", pc.ID,
			"days", extendDays, "trial_ends_at", newTrialEnd)
	}

	if metrics.Subscription != nil {
		metrics.Subscription.PromoAppliedTotal.
			WithLabelValues(string(in.Plan), in.Currency, "applied").Inc()
	}

	return terms(ApplyOutput{
		PromoCodeID:    pc.ID,
		StripeCouponID: couponID,
		EffectiveMinor: result.EffectiveMinor,
		TrialEndsAt:    appliedEnd,
	}, pc), nil
}

// planTrialExtension answers "how many days does this code add, and to what
// date", or explains why it cannot.
//
// It returns (0, zero, nil) for a code that extends no trial — the common
// case, and the one every existing call site is in.
//
// The two failure kinds are deliberately different errors, because they are
// different people's problems:
//
//   - ErrInvalidOrExpired: the merchant's subscription cannot take an
//     extension (converted, not trialing, or already lapsed). A 422 with a
//     reason, and their redemption is not spent.
//   - anything else: WE are misconfigured — no extender wired, or a caller
//     that did not pass the subscription. Reporting that as "invalid or
//     expired" would tell a merchant holding a perfectly good code to stop
//     trying, and would hide a wiring regression behind a merchant-facing
//     refusal. It fails closed and loudly instead.
func (s *Service) planTrialExtension(pc *PromoCode, in ApplyInput) (int, time.Time, error) {
	if pc.TrialExtensionDays == nil || *pc.TrialExtensionDays <= 0 {
		return 0, time.Time{}, nil
	}
	days := *pc.TrialExtensionDays

	if s.trial == nil {
		return 0, time.Time{}, fmt.Errorf(
			"promo: code %s grants a %d-day trial extension but no trial extender is wired: %w",
			pc.Code, days, ErrTrialExtensionUnavailable)
	}
	if in.Sub == nil {
		return 0, time.Time{}, fmt.Errorf(
			"promo: code %s grants a %d-day trial extension but the caller passed no subscription: %w",
			pc.Code, days, ErrTrialExtensionUnavailable)
	}

	if err := trial.Extendable(*in.Sub, time.Now().UTC()); err != nil {
		s.logger.Info("promo: trial-extension code refused — this trial cannot move",
			"store_id", in.StoreID, "promo_code_id", pc.ID, "reason", err)
		return 0, time.Time{}, ErrInvalidOrExpired
	}

	// The base is the EFFECTIVE end, never created_at + TrialDays. Extend
	// takes an absolute date, so deriving it from the signup date would
	// silently discard an extension an operator already granted (#620).
	return days, trial.EndsAt(*in.Sub).AddDate(0, 0, days), nil
}

// CancelPromo removes this code's coupon from the Stripe subscription's
// discounts and removes the local redemption record. Idempotent — if no
// redemption exists, returns nil.
func (s *Service) CancelPromo(ctx context.Context, in CancelInput) error {
	// Remove from Stripe first. Only this code's coupon goes: any other
	// discount on the subscription is written back untouched.
	if s.stripe != nil && in.StripeSubscriptionID != "" {
		couponID, err := s.cancelCouponID(ctx, in.PromoCodeID)
		if err != nil {
			return err
		}
		if couponID != "" {
			if err := billingstripe.RemoveSubscriptionDiscount(ctx, s.stripe, in.StripeSubscriptionID, couponID); err != nil {
				return fmt.Errorf("promo: cancel: stripe remove subscription discount: %w", err)
			}
			s.logger.Info("promo: coupon removed from stripe subscription discounts",
				"store_id", in.StoreID,
				"coupon_id", couponID,
				"stripe_sub_id", in.StripeSubscriptionID)
		}
	}

	// Remove local redemption record.
	if err := s.repo.DeleteRedemptionByStore(ctx, s.db, in.PromoCodeID, in.StoreID); err != nil {
		return fmt.Errorf("promo: cancel: delete redemption: %w", err)
	}
	return nil
}

// cancelCouponID returns the Stripe coupon backing promoCodeID, or "" when
// there is nothing to remove from Stripe: the code carries no coupon (a
// trial-extension-only code never does, and the console omits the id for a
// code not minted in the current Stripe mode, #726), or the promo code row
// is gone — in which case the local redemption delete still runs, keeping
// CancelPromo idempotent.
func (s *Service) cancelCouponID(ctx context.Context, promoCodeID uuid.UUID) (string, error) {
	pc, err := s.repo.GetByID(ctx, s.db, promoCodeID)
	if err != nil {
		if errors.As(err, new(*apperrors.Error)) {
			return "", nil
		}
		return "", fmt.Errorf("promo: cancel: lookup promo code: %w", err)
	}
	if pc.StripeCouponID == nil {
		return "", nil
	}
	return *pc.StripeCouponID, nil
}

// ValidateCode runs every §7.3 check for `in` and records NOTHING: no
// redemption row, no Stripe call, no metric. It answers "would this code be
// accepted for this store right now", which is a different question from
// "apply it".
//
// Two callers want that question, for different reasons. The cancel save
// offer asks before accepting the rescind, and applies straight after. The
// day-30 win-back email (#727) asks so it can decide whether to STATE the
// offer, and must not redeem: the merchant has not asked for anything yet,
// and burning the redemption here would leave them unable to use the code
// when they return — max_per_email is 1.
func (s *Service) ValidateCode(ctx context.Context, in ApplyInput) (ApplyOutput, error) {
	pc, err := s.repo.GetByCode(ctx, s.db, in.Code)
	if err != nil {
		if errors.As(err, new(*apperrors.Error)) {
			return ApplyOutput{RejectReason: RejectReasonNotFound}, ErrInvalidOrExpired
		}
		return ApplyOutput{}, fmt.Errorf("promo: validate code: lookup: %w", err)
	}

	totalRed, err := s.repo.CountRedemptions(ctx, s.db, pc.ID)
	if err != nil {
		return ApplyOutput{}, fmt.Errorf("promo: validate code: count: %w", err)
	}
	emailRed, err := s.repo.CountRedemptionsByEmail(ctx, s.db, pc.ID, normaliseEmail(in.MerchantEmail))
	if err != nil {
		return ApplyOutput{}, fmt.Errorf("promo: validate code: count email: %w", err)
	}

	result := Validate(ValidationInput{
		SubmittedCode:    in.Code,
		PromoCode:        pc,
		Now:              time.Now().UTC(),
		TotalRedemptions: totalRed,
		EmailRedemptions: emailRed,
		Plan:             in.Plan,
		Period:           in.Period,
		BasePriceMinor:   in.BasePriceMinor,
		Currency:         in.Currency,
	})
	if !result.Accepted {
		return ApplyOutput{RejectReason: result.RejectReason}, ErrInvalidOrExpired
	}

	// The same trial pre-flight ApplyPromo runs, for the same reason it is a
	// pre-flight there: "would this be accepted" and "apply it" must give the
	// same answer, or a caller states an offer the redeem path then refuses.
	// It writes nothing here — planTrialExtension only reads.
	if _, _, err := s.planTrialExtension(pc, in); err != nil {
		if errors.Is(err, ErrInvalidOrExpired) {
			return ApplyOutput{RejectReason: RejectReasonTrialNotExtendable}, err
		}
		return ApplyOutput{}, err
	}

	return terms(ApplyOutput{
		PromoCodeID:    pc.ID,
		EffectiveMinor: result.EffectiveMinor,
	}, pc), nil
}

// ValidateForSaveOffer is the cancel flow's name for ValidateCode. Kept as
// its own method because the save offer's call site and its tests read for
// the flow, not the mechanism; it adds no behaviour of its own.
func (s *Service) ValidateForSaveOffer(ctx context.Context, in ApplyInput) (ApplyOutput, error) {
	return s.ValidateCode(ctx, in)
}

func normaliseEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}
