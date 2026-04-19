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
	PromoCodeID    uuid.UUID
	StripeCouponID string
	EffectiveMinor int64
	// RejectReason is empty on success; set when the code was rejected (for audit).
	RejectReason ValidationRejectReason
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

	// 5. Attach coupon in Stripe (if client available).
	if s.stripe != nil && in.StripeSubscriptionID != "" {
		if err := billingstripe.AttachCoupon(ctx, s.stripe, in.StripeSubscriptionID, pc.StripeCouponID); err != nil {
			return ApplyOutput{}, fmt.Errorf("promo: apply: stripe attach coupon: %w", err)
		}
		s.logger.Info("promo: coupon attached to stripe subscription",
			"store_id", in.StoreID,
			"coupon_id", pc.StripeCouponID,
			"stripe_sub_id", in.StripeSubscriptionID)
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
		// Best-effort rollback: try to detach from Stripe.
		if s.stripe != nil && in.StripeSubscriptionID != "" {
			_ = billingstripe.DetachCoupon(ctx, s.stripe, in.StripeSubscriptionID)
		}
		return ApplyOutput{}, fmt.Errorf("promo: apply: record redemption: %w", err)
	}

	if metrics.Subscription != nil {
		metrics.Subscription.PromoAppliedTotal.
			WithLabelValues(string(in.Plan), in.Currency, "applied").Inc()
	}

	return ApplyOutput{
		PromoCodeID:    pc.ID,
		StripeCouponID: pc.StripeCouponID,
		EffectiveMinor: result.EffectiveMinor,
	}, nil
}

// CancelPromo detaches the coupon from the Stripe subscription and removes
// the local redemption record. Idempotent — if no redemption exists, returns nil.
func (s *Service) CancelPromo(ctx context.Context, in CancelInput) error {
	// Detach from Stripe first (idempotent on 404).
	if s.stripe != nil && in.StripeSubscriptionID != "" {
		if err := billingstripe.DetachCoupon(ctx, s.stripe, in.StripeSubscriptionID); err != nil {
			return fmt.Errorf("promo: cancel: stripe detach coupon: %w", err)
		}
		s.logger.Info("promo: coupon detached from stripe subscription",
			"store_id", in.StoreID,
			"stripe_sub_id", in.StripeSubscriptionID)
	}

	// Remove local redemption record.
	if err := s.repo.DeleteRedemptionByStore(ctx, s.db, in.PromoCodeID, in.StoreID); err != nil {
		return fmt.Errorf("promo: cancel: delete redemption: %w", err)
	}
	return nil
}

// ValidateForSaveOffer is called by P11's cancel flow to assert that a promo
// code is valid for the save-offer flow before accepting the cancellation
// rescind. It runs all §7.3 checks but does NOT record a redemption — that
// happens only if the merchant confirms the save-offer.
func (s *Service) ValidateForSaveOffer(ctx context.Context, in ApplyInput) (ApplyOutput, error) {
	pc, err := s.repo.GetByCode(ctx, s.db, in.Code)
	if err != nil {
		if errors.As(err, new(*apperrors.Error)) {
			return ApplyOutput{RejectReason: RejectReasonNotFound}, ErrInvalidOrExpired
		}
		return ApplyOutput{}, fmt.Errorf("promo: validate for save offer: lookup: %w", err)
	}

	totalRed, err := s.repo.CountRedemptions(ctx, s.db, pc.ID)
	if err != nil {
		return ApplyOutput{}, fmt.Errorf("promo: validate for save offer: count: %w", err)
	}
	emailRed, err := s.repo.CountRedemptionsByEmail(ctx, s.db, pc.ID, normaliseEmail(in.MerchantEmail))
	if err != nil {
		return ApplyOutput{}, fmt.Errorf("promo: validate for save offer: count email: %w", err)
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

	return ApplyOutput{
		PromoCodeID:    pc.ID,
		StripeCouponID: pc.StripeCouponID,
		EffectiveMinor: result.EffectiveMinor,
	}, nil
}

func normaliseEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}
