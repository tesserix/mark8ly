// Package admin — promo.go: HTTP handlers for promo-code application/cancellation.
// POST /admin/stores/:storeId/subscription/apply-promo
// DELETE /admin/stores/:storeId/subscription/promo
package admin

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

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
	// Email is the merchant email for per-email rate-limit and redemption tracking.
	Email string `json:"email" binding:"required"`
}

type applyPromoResponse struct {
	StripeCouponID string `json:"stripe_coupon_id"`
	EffectiveMinor int64  `json:"effective_minor"`
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

	actor := "user:" + c.GetString("user_id")
	var subID uuid.UUID
	if sub != nil {
		subID = sub.ID
	}

	out, applyErr := h.svc.ApplyPromo(c.Request.Context(), promo.ApplyInput{
		TenantID:             tenantID,
		StoreID:              storeID,
		SubscriptionID:       subID,
		Code:                 req.Code,
		MerchantEmail:        req.Email,
		Plan:                 sub.Plan,
		Period:               sub.SubscriptionPeriod,
		BasePriceMinor:       0, // populated from billing_currency/plan lookup in future; floor uses plan+currency
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
			c.AbortWithStatusJSON(http.StatusUnprocessableEntity, gin.H{
				"error":   "promo_invalid_or_expired",
				"message": "the promo code is invalid or has expired",
			})
			return
		}
		RespondErr(c, applyErr, h.logger)
		return
	}

	c.JSON(http.StatusOK, applyPromoResponse{
		StripeCouponID: out.StripeCouponID,
		EffectiveMinor: out.EffectiveMinor,
	})
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
