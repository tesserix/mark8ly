package main

import (
	"context"

	"github.com/gin-gonic/gin"

	"github.com/mark8ly/marketplace-api/internal/audit"
	"github.com/mark8ly/marketplace-api/internal/promo"
	"github.com/mark8ly/marketplace-api/internal/subscription"
)

// signupPromoAdapter joins internal/promo to internal/subscription's
// SignupPromoRedeemer.
//
// It exists because the dependency can only point one way: internal/promo
// already imports internal/subscription for its plan and status constants, so
// subscription cannot import promo back. main.go imports both, which makes it
// the only place the two shapes can meet.
//
// The adapter is also where the INTERNAL reject reason becomes the PUBLIC one.
// That conversion has to happen exactly once, at the boundary the value
// crosses on its way to platform-api and then to a merchant — promo's own
// reasons distinguish not_found from expired, and PublicReasonFor merges them
// so a caller cannot tell a real code from a guessed one (#620).
type signupPromoAdapter struct{ svc *promo.Service }

func (a signupPromoAdapter) RedeemAtSignup(ctx context.Context,
	in subscription.SignupPromoInput) (subscription.SignupPromoOffer, error) {
	offer, err := a.svc.RedeemAtSignup(ctx, promo.SignupInput{
		Code:          in.Code,
		MerchantEmail: in.MerchantEmail,
		Currency:      in.Currency,
		Sub:           in.Sub,
	})
	return out(offer), err
}

func (a signupPromoAdapter) ValidateForSignup(ctx context.Context,
	in subscription.SignupPromoInput) (subscription.SignupPromoOffer, error) {
	offer, err := a.svc.ValidateForSignup(ctx, promo.SignupInput{
		Code:          in.Code,
		MerchantEmail: in.MerchantEmail,
		Currency:      in.Currency,
		Sub:           in.Sub,
	})
	return out(offer), err
}

// out converts a promo offer for the wire, collapsing the reject reason to the
// public set. An empty internal reason stays empty rather than becoming
// PublicReasonFor's default — "" means "not a validation failure at all", and
// mapping it to invalid_or_expired would report a refusal on a success.
func out(offer promo.SignupOffer) subscription.SignupPromoOffer {
	reason := ""
	if offer.RejectReason != "" {
		reason = string(promo.PublicReasonFor(offer.RejectReason))
	}
	return subscription.SignupPromoOffer{
		TrialExtensionDays: offer.TrialExtensionDays,
		RejectReason:       reason,
	}
}

// signupPromoAuditAdapter joins internal/audit to subscription's
// SignupPromoAuditor, for the same one-way-import reason as above.
type signupPromoAuditAdapter struct{ emitter *audit.Emitter }

func (a signupPromoAuditAdapter) RecordSignupPromo(c *gin.Context,
	sub *subscription.StoreSubscription, code string, offer subscription.SignupPromoOffer) {
	a.emitter.EmitPromoApplied(c, audit.PromoApplied{
		TenantID:           sub.TenantID,
		StoreID:            sub.StoreID,
		Code:               code,
		Actor:              "system:onboarding",
		RejectReason:       offer.RejectReason,
		Accepted:           offer.RejectReason == "",
		TrialExtensionDays: offer.TrialExtensionDays,
	})
}
