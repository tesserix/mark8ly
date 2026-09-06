package cancel

import (
	"context"

	"github.com/mark8ly/marketplace-api/internal/promo"
	"github.com/mark8ly/marketplace-api/internal/subscription"
)

// SaveOfferPromoCode is the promo code the cancellation save offer redeems.
//
// A promo_codes row whose code is exactly this string is what switches the
// save-offer discount on. mark8ly does not mint codes: rows arrive from the
// tesserix-home promo catalog through internal/billing/consolepromo (#726),
// so authoring this code is a console action. Until the row exists this path
// is a deliberate no-op — the reversal still happens and the merchant is not
// told about a discount that was not applied (#701).
//
// promo_codes now floors code length at 4 characters (migration 000131,
// relaxed from 12 for console campaign codes such as "LAUNCH50"), so this
// constant is comfortably provisionable at either bound.
const SaveOfferPromoCode = "SAVEOFFER20OFF6MONTHS"

const (
	saveOfferMsgDiscountApplied = "Save offer accepted. A 20% discount applies to your next billing cycle."
	saveOfferMsgReversalOnly    = "Save offer accepted. Your subscription stays active and will not be cancelled."
)

// PromoApplier is the narrow slice of promo.Service the save offer needs.
// Declared here (rather than depending on *promo.Service) so the dependency is
// optional and stubbable; promo does not import this package, so there is no
// import cycle.
type PromoApplier interface {
	ValidateForSaveOffer(ctx context.Context, in promo.ApplyInput) (promo.ApplyOutput, error)
	ApplyPromo(ctx context.Context, in promo.ApplyInput) (promo.ApplyOutput, error)
}

// WithPromo returns a copy of the Service that attempts the save-offer discount
// through p. A Service without one behaves exactly as before, minus the false
// discount claim. Passing a nil applier leaves the Service unchanged.
func (s *Service) WithPromo(p PromoApplier) *Service {
	if s == nil || p == nil {
		return s
	}
	cp := *s
	cp.promo = p
	return &cp
}

// SaveOfferMessage returns the merchant-facing message for an accepted save
// offer. It claims the discount only when one was actually applied — the
// merchant un-cancels in reliance on this sentence, so it must never describe a
// discount that does not exist (#701).
func SaveOfferMessage(discountApplied bool) string {
	if discountApplied {
		return saveOfferMsgDiscountApplied
	}
	return saveOfferMsgReversalOnly
}

// saveOfferOutput builds the response for a completed save-offer reversal. The
// status is active either way: whether the discount applied has no bearing on
// the reversal, which has already been committed by the time this is called.
func saveOfferOutput(discountApplied bool) Output {
	return Output{
		Status:       string(subscription.StatusActive),
		SaveOfferMsg: SaveOfferMessage(discountApplied),
	}
}

// applySaveOfferDiscount attempts to attach the save-offer discount and reports
// whether it was actually applied.
//
// It deliberately returns no error. The merchant asked to un-cancel and that
// has already succeeded; a discount that cannot be applied must never fail the
// reversal or surface an error to them. Every failure path is logged and
// reported as "not applied", which the message then reflects honestly.
func (s *Service) applySaveOfferDiscount(ctx context.Context, in Input, sub *subscription.StoreSubscription) bool {
	if s.promo == nil {
		s.logger.Info("cancel: save offer accepted without a promo service — no discount applied",
			"store_id", in.StoreID, "tenant_id", in.TenantID)
		return false
	}
	if sub == nil {
		s.logger.Warn("cancel: save offer accepted with no subscription loaded — no discount applied",
			"store_id", in.StoreID, "tenant_id", in.TenantID)
		return false
	}

	applyIn := promo.ApplyInput{
		TenantID:       in.TenantID,
		StoreID:        in.StoreID,
		SubscriptionID: sub.ID,
		Code:           SaveOfferPromoCode,
		MerchantEmail:  derefString(sub.Email),
		Plan:           sub.Plan,
		Period:         sub.SubscriptionPeriod,
		// The §7.4 absolute floor is computed FROM this price, so a zero
		// here rejected every priced plan out of hand — the save-offer
		// discount could never apply to a paying merchant. See
		// promo.BasePriceMinorFor.
		BasePriceMinor: promo.BasePriceMinorFor(
			sub.Plan, sub.SubscriptionPeriod, sub.PriceTier, derefString(sub.BillingCurrency)),
		Currency:             derefString(sub.BillingCurrency),
		StripeSubscriptionID: derefString(sub.StripeSubscriptionID),
		Actor:                in.Actor,
	}

	if _, err := s.promo.ValidateForSaveOffer(ctx, applyIn); err != nil {
		s.logger.Info("cancel: save-offer discount not available — reversal stands, no discount claimed",
			"store_id", in.StoreID, "tenant_id", in.TenantID, "code", SaveOfferPromoCode, "err", err)
		return false
	}

	if _, err := s.promo.ApplyPromo(ctx, applyIn); err != nil {
		s.logger.Warn("cancel: save-offer discount failed to apply — reversal stands, no discount claimed",
			"store_id", in.StoreID, "tenant_id", in.TenantID, "code", SaveOfferPromoCode, "err", err)
		return false
	}

	s.logger.Info("cancel: save-offer discount applied",
		"store_id", in.StoreID, "tenant_id", in.TenantID, "code", SaveOfferPromoCode)
	return true
}

func derefString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
