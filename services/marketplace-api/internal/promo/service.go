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

// Service is the promo-code application service.
type Service struct {
	db     *gorm.DB
	repo   Repository
	stripe *billingstripe.Client
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

	if metrics.Subscription != nil {
		metrics.Subscription.PromoAppliedTotal.
			WithLabelValues(string(in.Plan), in.Currency, "applied").Inc()
	}

	return terms(ApplyOutput{
		PromoCodeID:    pc.ID,
		StripeCouponID: couponID,
		EffectiveMinor: result.EffectiveMinor,
	}, pc), nil
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
