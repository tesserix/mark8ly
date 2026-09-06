// Package admin — promo.go: HTTP handlers for promo-code application/cancellation.
// POST /admin/stores/:storeId/subscription/apply-promo
// DELETE /admin/stores/:storeId/subscription/promo
package admin

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/audit"
	"github.com/mark8ly/marketplace-api/internal/promo"
	"github.com/mark8ly/marketplace-api/internal/subscription"
	"github.com/mark8ly/marketplace-api/pkg/apperrors"
)

// PromoApplier is the interface the handler needs from promo.Service.
type PromoApplier interface {
	ApplyPromo(ctx context.Context, in promo.ApplyInput) (promo.ApplyOutput, error)
	CancelPromo(ctx context.Context, in promo.CancelInput) error
}

// PromoHandler exposes promo apply + cancel endpoints.
type PromoHandler struct {
	db      *gorm.DB
	svc     PromoApplier
	subRepo subscription.Repository
	emitter *audit.Emitter
	logger  *slog.Logger
}

// NewPromoHandler constructs a PromoHandler.
func NewPromoHandler(db *gorm.DB, svc PromoApplier, subRepo subscription.Repository, logger *slog.Logger) *PromoHandler {
	return &PromoHandler{db: db, svc: svc, subRepo: subRepo, logger: logger}
}

// WithAudit wires the audit emitter into the handler.
func (h *PromoHandler) WithAudit(e *audit.Emitter) *PromoHandler {
	h.emitter = e
	return h
}

type applyPromoRequest struct {
	Code string `json:"code" binding:"required"`
}

type applyPromoResponse struct {
	StripeCouponID string `json:"stripe_coupon_id"`
	EffectiveMinor int64  `json:"effective_minor"`
	// Currency is the store's billing currency, echoed so a client can format
	// EffectiveMinor. It is not derivable client-side: the GET subscription
	// DTO does not carry billing_currency, and the admin schema defaults the
	// missing field to USD — which would render an INR price as dollars.
	Currency string `json:"currency"`
	// PercentOffBps and MaxDurationMonths describe the offer that was just
	// applied, so the confirmation can state its size and how long it runs
	// instead of only the new price. Both are 0 when the row does not say:
	// 0 months is "no bound stated", never "zero months". A client that
	// cannot describe the terms must say nothing about them rather than
	// guess — the same rule the win-back email follows (#727).
	PercentOffBps     int `json:"percent_off_bps"`
	MaxDurationMonths int `json:"max_duration_months"`
}

// applyPromoErrorResponse is the 422 body for a refused code.
//
// Error stays the coarse, unchanging code. Reason is the machine-readable
// rejection reason, already collapsed by promo.PublicReasonFor so that
// not_found and expired are indistinguishable here — see public_reason.go for
// why that pair alone is merged. Message remains the safe fallback sentence
// for a client that does not recognise the reason.
type applyPromoErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message"`
	Reason  string `json:"reason"`
}

// ApplyPromo handles POST /admin/stores/:storeId/subscription/apply-promo.
func (h *PromoHandler) ApplyPromo(c *gin.Context) {
	storeID, err := uuid.Parse(c.Param("storeId"))
	if err != nil {
		RespondErr(c, apperrors.ValidationFailed("storeId", "invalid uuid"), h.logger)
		return
	}
	tenantID, err := uuid.Parse(c.GetString("tenant_id"))
	if err != nil {
		RespondErr(c, apperrors.ValidationFailed("tenant_id", "invalid uuid"), h.logger)
		return
	}

	var req applyPromoRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		RespondErr(c, apperrors.ValidationFailed("body", err.Error()), h.logger)
		return
	}

	// Fetch current subscription to get plan/period/currency/stripe sub id.
	sub, err := h.subRepo.GetByStoreID(c.Request.Context(), h.db, tenantID, storeID)
	if err != nil {
		RespondErr(c, err, h.logger)
		return
	}
	if sub == nil {
		RespondErr(c, apperrors.NotFound("subscription"), h.logger)
		return
	}

	merchantEmail, ok := h.merchantEmail(c, sub)
	if !ok {
		RespondErr(c, apperrors.ValidationFailed("email",
			"this store has no billing email, so per-email redemption limits cannot be checked"), h.logger)
		return
	}

	actor := "user:" + c.GetString("user_id")

	out, applyErr := h.svc.ApplyPromo(c.Request.Context(), promo.ApplyInput{
		TenantID:       tenantID,
		StoreID:        storeID,
		SubscriptionID: sub.ID,
		Code:           req.Code,
		MerchantEmail:  merchantEmail,
		Plan:           sub.Plan,
		Period:         sub.SubscriptionPeriod,
		BasePriceMinor: promo.BasePriceMinorFor(
			sub.Plan, sub.SubscriptionPeriod, sub.PriceTier, stringVal(sub.BillingCurrency)),
		Currency:             stringVal(sub.BillingCurrency),
		StripeSubscriptionID: stringVal(sub.StripeSubscriptionID),
		Actor:                actor,
	})

	// Emit audit regardless of success/failure (§23.1).
	if h.emitter != nil {
		rejectReason := ""
		if out.RejectReason != "" {
			rejectReason = string(out.RejectReason)
		}
		h.emitter.EmitPromoApplied(c, audit.PromoApplied{
			TenantID:     tenantID,
			StoreID:      storeID,
			Code:         req.Code,
			Actor:        actor,
			RejectReason: rejectReason,
			Accepted:     applyErr == nil,
		})
	}

	if applyErr != nil {
		if errors.Is(applyErr, promo.ErrInvalidOrExpired) {
			c.AbortWithStatusJSON(http.StatusUnprocessableEntity, applyPromoErrorResponse{
				Error:   "promo_invalid_or_expired",
				Message: "the promo code is invalid or has expired",
				Reason:  string(promo.PublicReasonFor(out.RejectReason)),
			})
			return
		}
		RespondErr(c, applyErr, h.logger)
		return
	}

	c.JSON(http.StatusOK, applyPromoResponse{
		StripeCouponID:    out.StripeCouponID,
		EffectiveMinor:    out.EffectiveMinor,
		Currency:          stringVal(sub.BillingCurrency),
		PercentOffBps:     out.PercentOffBps,
		MaxDurationMonths: out.MaxDurationMonths,
	})
}

// merchantEmail returns the address the per-email redemption cap is counted
// against, and whether one was found.
//
// It is deliberately NOT taken from the request body, which is where this
// handler used to read it. A client that chooses the address chooses whose
// cap it spends: submitting someone else's address writes a redemption row
// under it, and max_per_email being 1 for the campaign codes, that merchant's
// own attempt is then refused max_per_email_reached. A merchant supplying
// throwaway addresses would also clear their own cap at will.
//
// The subscription's billing email comes first because the other two callers
// of promo.ApplyPromo already use it — cancel.applySaveOfferDiscount and the
// day-30 win-back's offerFor both pass sub.Email. The cap only means anything
// if all three agree on which address they mean, and disagreement here would
// be worse than a wrong number: the win-back email validates against
// sub.Email before it states the offer, so a redemption booked against the
// session address would let the email promise a code this endpoint then
// refuses.
//
// The session address is the fallback for a row whose email column is unset
// — a store that never reached Stripe customer creation. It cannot fall back
// further to the empty string: "" is a single shared bucket that every
// address-less store would count redemptions in, so one store's redemption
// would exhaust the cap for all of them.
func (h *PromoHandler) merchantEmail(c *gin.Context, sub *subscription.StoreSubscription) (string, bool) {
	if email := strings.TrimSpace(stringVal(sub.Email)); email != "" {
		return email, true
	}
	if email := strings.TrimSpace(c.GetString("user_email")); email != "" {
		return email, true
	}
	return "", false
}

type cancelPromoRequest struct {
	PromoCodeID string `json:"promo_code_id" binding:"required"`
}

// CancelPromo handles DELETE /admin/stores/:storeId/subscription/promo.
func (h *PromoHandler) CancelPromo(c *gin.Context) {
	storeID, err := uuid.Parse(c.Param("storeId"))
	if err != nil {
		RespondErr(c, apperrors.ValidationFailed("storeId", "invalid uuid"), h.logger)
		return
	}
	tenantID, err := uuid.Parse(c.GetString("tenant_id"))
	if err != nil {
		RespondErr(c, apperrors.ValidationFailed("tenant_id", "invalid uuid"), h.logger)
		return
	}

	var req cancelPromoRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		RespondErr(c, apperrors.ValidationFailed("body", err.Error()), h.logger)
		return
	}

	promoCodeID, err := uuid.Parse(req.PromoCodeID)
	if err != nil {
		RespondErr(c, apperrors.ValidationFailed("promo_code_id", "invalid uuid"), h.logger)
		return
	}

	sub, err := h.subRepo.GetByStoreID(c.Request.Context(), h.db, tenantID, storeID)
	if err != nil {
		RespondErr(c, err, h.logger)
		return
	}

	actor := "user:" + c.GetString("user_id")

	if err := h.svc.CancelPromo(c.Request.Context(), promo.CancelInput{
		TenantID:             tenantID,
		StoreID:              storeID,
		PromoCodeID:          promoCodeID,
		StripeSubscriptionID: stringVal(sub.StripeSubscriptionID),
		Actor:                actor,
	}); err != nil {
		RespondErr(c, err, h.logger)
		return
	}

	if h.emitter != nil {
		h.emitter.EmitPromoCancelled(c, audit.PromoCancelled{
			TenantID:    tenantID,
			StoreID:     storeID,
			PromoCodeID: promoCodeID,
			Actor:       actor,
		})
	}

	c.Status(http.StatusNoContent)
}

func stringVal(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
