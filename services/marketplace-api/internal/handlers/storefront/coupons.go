// Package storefront — coupons.go: storefront coupon validation endpoint.
// POST /storefront/stores/:storeSlug/coupons/validate
package storefront

import (
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/mark8ly/marketplace-api/internal/coupon"
	"github.com/mark8ly/marketplace-api/internal/stores"
	"github.com/mark8ly/marketplace-api/pkg/apperrors"
)

// CouponValidateHandler handles storefront coupon validation.
type CouponValidateHandler struct {
	svc    *coupon.Service
	logger *slog.Logger
}

// NewCouponValidateHandler constructs a CouponValidateHandler.
func NewCouponValidateHandler(svc *coupon.Service, logger *slog.Logger) *CouponValidateHandler {
	return &CouponValidateHandler{svc: svc, logger: logger}
}

// ValidateCouponRequest is the JSON body for the validate endpoint.
type ValidateCouponRequest struct {
	Code          string          `json:"code"           binding:"required"`
	CustomerEmail string          `json:"customer_email" binding:"omitempty,email"`
	Subtotal      decimal.Decimal `json:"subtotal"       binding:"required"`
}

// ValidateCouponResponse is the JSON response for a successful validation.
type ValidateCouponResponse struct {
	CouponID       string          `json:"coupon_id"`
	Code           string          `json:"code"`
	Type           string          `json:"type"`
	Value          decimal.Decimal `json:"value"`
	DiscountAmount decimal.Decimal `json:"discount_amount"`
	FreeShipping   bool            `json:"free_shipping"`
	Title          string          `json:"title"`
}

// Validate handles POST /storefront/stores/:storeSlug/coupons/validate.
func (h *CouponValidateHandler) Validate(c *gin.Context) {
	storeVal, ok := c.Get("store")
	if !ok {
		respondNotFound(c)
		return
	}
	store, ok := storeVal.(*stores.Store)
	if !ok || store == nil {
		respondNotFound(c)
		return
	}

	storeID, err := uuid.Parse(store.ID)
	if err != nil {
		h.respondErr(c, apperrors.ValidationFailed("store.id", "invalid uuid"))
		return
	}

	tenantID, err := uuid.Parse(store.TenantID)
	if err != nil {
		h.respondErr(c, apperrors.ValidationFailed("store.tenant_id", "invalid uuid"))
		return
	}

	var req ValidateCouponRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.respondErr(c, apperrors.ValidationFailed("body", err.Error()))
		return
	}

	result, err := h.svc.Validate(c.Request.Context(), coupon.ValidateInput{
		TenantID:      tenantID,
		StoreID:       storeID,
		Code:          req.Code,
		CustomerEmail: req.CustomerEmail,
		Subtotal:      req.Subtotal,
	})
	if err != nil {
		h.respondErr(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data": ValidateCouponResponse{
			CouponID:       result.CouponID.String(),
			Code:           result.Code,
			Type:           string(result.Type),
			Value:          result.Value,
			DiscountAmount: result.DiscountAmount,
			FreeShipping:   result.FreeShipping,
			Title:          result.Title,
		},
	})
}

// respondErr mirrors the checkout_ext.go error response pattern.
func (h *CouponValidateHandler) respondErr(c *gin.Context, err error) {
	var ae *apperrors.Error
	if asErr, ok := err.(*apperrors.Error); ok {
		ae = asErr
	}
	if ae != nil {
		switch ae.Code {
		case apperrors.CodeValidationFailed:
			c.AbortWithStatusJSON(http.StatusBadRequest, map[string]any{
				"error":   string(ae.Code),
				"message": ae.Message,
			})
			return
		case apperrors.CodeCouponNotFound:
			c.AbortWithStatusJSON(http.StatusNotFound, map[string]any{
				"error":   string(ae.Code),
				"message": ae.Message,
			})
			return
		case apperrors.CodeCouponExpired,
			apperrors.CodeCouponUsageLimitReached,
			apperrors.CodeCouponInvalid,
			apperrors.CodeCouponMinPurchaseNotMet:
			c.AbortWithStatusJSON(http.StatusUnprocessableEntity, map[string]any{
				"error":   string(ae.Code),
				"message": ae.Message,
			})
			return
		}
	}
	if h.logger != nil {
		h.logger.Error("coupon validate: unhandled error", "err", err.Error())
	}
	c.AbortWithStatusJSON(http.StatusInternalServerError, map[string]any{
		"error":   "internal",
		"message": "internal server error",
	})
}
