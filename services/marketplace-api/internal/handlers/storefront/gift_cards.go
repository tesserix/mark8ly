package storefront

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/mark8ly/marketplace-api/internal/giftcard"
	"github.com/mark8ly/marketplace-api/internal/stores"
	"github.com/mark8ly/marketplace-api/pkg/apperrors"
)

// CheckBalanceRequest is the wire DTO for POST /gift-cards/check-balance.
type CheckBalanceRequest struct {
	Code string `json:"code" binding:"required"`
}

// GiftCardStorefrontHandler handles storefront gift card endpoints.
type GiftCardStorefrontHandler struct {
	svc    *giftcard.Service
	logger *slog.Logger
}

// NewGiftCardStorefrontHandler constructs a GiftCardStorefrontHandler.
func NewGiftCardStorefrontHandler(svc *giftcard.Service, logger *slog.Logger) *GiftCardStorefrontHandler {
	return &GiftCardStorefrontHandler{svc: svc, logger: logger}
}

// CheckBalance handles POST /storefront/stores/:storeSlug/gift-cards/check-balance.
func (h *GiftCardStorefrontHandler) CheckBalance(c *gin.Context) {
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
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
			"error": "validation_failed", "message": "invalid store",
		})
		return
	}

	var req CheckBalanceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
			"error": "validation_failed", "message": err.Error(),
		})
		return
	}

	result, err := h.svc.CheckBalance(c.Request.Context(), storeID, req.Code)
	if err != nil {
		// Amendment LOW FIX 9: use errors.As instead of manual type assertion.
		var ae *apperrors.Error
		if errors.As(err, &ae) {
			switch ae.Code {
			case apperrors.CodeGiftCardNotFound, apperrors.CodeNotFound:
				c.AbortWithStatusJSON(http.StatusNotFound, gin.H{
					"error": "gift_card_not_found", "message": "gift card not found",
				})
				return
			case apperrors.CodeGiftCardExpired:
				c.AbortWithStatusJSON(http.StatusGone, gin.H{
					"error": "gift_card_expired", "message": "this gift card has expired",
				})
				return
			}
		}
		if h.logger != nil {
			h.logger.Error("gift card check balance error", "err", err.Error())
		}
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
			"error": "internal", "message": "internal server error",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": result})
}
