package promo

import (
	"context"
	"strings"
	"time"

	"github.com/mark8ly/marketplace-api/internal/subscription"
)

// signup.go — redeeming a promo code at onboarding (#620).
//
// The merchant types a code before they have a plan, a price or a card. That
// makes signup a NARROWER surface than the billing page, not merely an earlier
// one, and the narrowing is what this file exists to state.

// SignupInput asks about, or redeems, a code for a merchant who has just
// signed up.
//
// No plan, period or base price: at signup the plan is always trial, which has
// no Stripe Price and no floor entry, so those are derived here rather than
// asked of a caller that could get them wrong.
type SignupInput struct {
	// Code is what the merchant typed. Empty is normal and means "no code".
	Code string
	// MerchantEmail is the address the per-email cap counts against. It is
	// the one abuse control available at signup, and it is available because
	// onboarding has already verified the address.
	MerchantEmail string
	// Currency is the store's ISO 4217 billing currency.
	Currency string
	// Sub is the subscription row to extend. Nil for validation, required for
	// redemption — RedeemAtSignup fails closed without it.
	Sub *subscription.StoreSubscription
}

// SignupOffer is what a code grants a merchant at signup.
type SignupOffer struct {
	// TrialExtensionDays is the number of days added to the trial, or 0 when
	// no code was supplied or none was granted.
	TrialExtensionDays int
	// RejectReason is empty on success. It is the INTERNAL reason; run it
	// through PublicReasonFor before it leaves the server.
	RejectReason ValidationRejectReason
}

// applyInput builds the ApplyInput a signup redemption uses.
//
// Plan is trial and Period is monthly because that is what a store_subscriptions
// row looks like the moment onboarding creates it (#827): plan=trial,
// status=signup, no period chosen. BasePriceMinor is 0 for the same reason —
// trial has no catalog price — and CheckFloor passes for a plan with no floor
// entry, so a zero here is not a discount that undercuts anything.
func (in SignupInput) applyInput() ApplyInput {
	out := ApplyInput{
		Code:           strings.TrimSpace(in.Code),
		MerchantEmail:  in.MerchantEmail,
		Plan:           subscription.PlanTrial,
		Period:         subscription.PeriodMonthly,
		BasePriceMinor: 0,
		Currency:       strings.ToLower(strings.TrimSpace(in.Currency)),
		Actor:          "system:onboarding",
		Sub:            in.Sub,
	}
	if in.Sub != nil {
		out.TenantID = in.Sub.TenantID
		out.StoreID = in.Sub.StoreID
		out.SubscriptionID = in.Sub.ID
		return out
	}

	// No subscription yet — this is the validating caller, asking before the
	// row exists. Answer against the row onboarding is ABOUT to create (#827):
	// plan=trial, status=signup, created now, never extended.
	//
	// Not a fudge to satisfy a nil check. "Would this code work?" asked at
	// signup means "would it work against the subscription I am about to
	// get", and that row's shape is known exactly. Passing nil instead would
	// fail closed as a caller bug, and inventing a different shape here would
	// let validate and redeem answer differently.
	out.Sub = &subscription.StoreSubscription{
		Plan:      subscription.PlanTrial,
		Status:    subscription.StatusSignup,
		CreatedAt: time.Now().UTC(),
	}
	return out
}

// deliverableAtSignup reports whether the WHOLE of a code's benefit can be
// delivered to a merchant who has just signed up.
//
// A discount cannot be. There is no Stripe subscription yet for a coupon to
// attach to, so ApplyPromo would log "no subscription id — skipping Stripe
// coupon attach", write the ledger row, and report success: the merchant's one
// redemption spent on the half we could not honour, with nothing to show for
// it and no way to claim it later.
//
// So a code carrying any discount is refused here and pointed at the billing
// page, where it works. Refusing is the conservative direction: the merchant
// still holds a usable code.
//
// ONE definition, called by both ValidateForSignup and RedeemAtSignup. If the
// two could disagree, the onboarding field would accept a code that signup
// then silently ate — which is the exact failure this rule prevents.
func deliverableAtSignup(pc *PromoCode) bool {
	return pc.DiscountType == nil && pc.DiscountValue == nil
}

// ValidateForSignup answers "would this code be accepted at signup, and what
// would it grant", and records NOTHING — no redemption row, no Stripe call.
//
// It is what the onboarding field calls as the merchant types, so the offer
// can be shown before they commit. Spending the code here would be fatal:
// max_per_email is 1, so a redemption written at typing time is a code the
// merchant can never actually use.
func (s *Service) ValidateForSignup(ctx context.Context, in SignupInput) (SignupOffer, error) {
	code := strings.TrimSpace(in.Code)
	if code == "" {
		return SignupOffer{}, nil
	}

	pc, err := s.repo.GetByCode(ctx, s.db, code)
	if err != nil {
		return SignupOffer{RejectReason: RejectReasonNotFound}, ErrInvalidOrExpired
	}
	if !deliverableAtSignup(pc) {
		return SignupOffer{RejectReason: RejectReasonRedeemInBilling}, ErrInvalidOrExpired
	}

	out, err := s.ValidateCode(ctx, in.applyInput())
	if err != nil {
		return SignupOffer{RejectReason: out.RejectReason}, err
	}
	return SignupOffer{TrialExtensionDays: out.TrialExtensionDays}, nil
}

// RedeemAtSignup redeems a code against a subscription that has just been
// created, and returns what it granted.
//
// An empty code is a no-op returning the zero offer, because most merchants
// type nothing and that is not a failure. Called from the onboarding
// subscription callback, immediately after the row exists — which is the
// earliest moment redemption is possible at all, since the ledger row needs a
// subscription id and trial.Extend needs a row to move.
func (s *Service) RedeemAtSignup(ctx context.Context, in SignupInput) (SignupOffer, error) {
	code := strings.TrimSpace(in.Code)
	if code == "" {
		return SignupOffer{}, nil
	}

	pc, err := s.repo.GetByCode(ctx, s.db, code)
	if err != nil {
		return SignupOffer{RejectReason: RejectReasonNotFound}, ErrInvalidOrExpired
	}
	if !deliverableAtSignup(pc) {
		return SignupOffer{RejectReason: RejectReasonRedeemInBilling}, ErrInvalidOrExpired
	}

	out, err := s.ApplyPromo(ctx, in.applyInput())
	if err != nil {
		return SignupOffer{RejectReason: out.RejectReason}, err
	}
	return SignupOffer{TrialExtensionDays: out.TrialExtensionDays}, nil
}
