package storefront

import (
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"

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

// MyGiftCards handles GET /storefront/stores/:storeSlug/gift-cards/mine.
// Returns cards where the authenticated customer is the purchaser or
// the recipient. Requires RequireCustomerAuth middleware on the route.
func (h *GiftCardStorefrontHandler) MyGiftCards(c *gin.Context) {
	storeVal, _ := c.Get("store")
	store, ok := storeVal.(*stores.Store)
	if !ok || store == nil {
		respondNotFound(c)
		return
	}
	storeID, err := uuid.Parse(store.ID)
	if err != nil {
		respondNotFound(c)
		return
	}
	emailVal, _ := c.Get(CustomerEmailKey)
	email, _ := emailVal.(string)
	if email == "" {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
			"error": "unauthorized", "message": "sign-in required",
		})
		return
	}
	cards, err := h.svc.ListByCustomerEmail(c.Request.Context(), storeID, email)
	if err != nil {
		if h.logger != nil {
			h.logger.Error("gift cards mine error", "err", err.Error())
		}
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
			"error": "internal", "message": "internal server error",
		})
		return
	}
	out := make([]gin.H, 0, len(cards))
	for _, gc := range cards {
		out = append(out, myGiftCardDTO(gc))
	}
	c.JSON(http.StatusOK, gin.H{"data": out})
}

// maskedCodeDisplay is what we render instead of a real code. Same shape
// as FormatCodeDisplay so the monospace layout doesn't jump.
const maskedCodeDisplay = "••••-••••-••••-••••-••••-••••-••"

// myGiftCardDTO builds one row of the My Account → Gift cards response.
//
// The code is only ever disclosed for an `active` card. A `pending` card
// has not been paid for yet, and this endpoint is reachable by anyone who
// can put their own address in `purchaser_email` / `recipient_email` on
// the public purchase endpoint — so emitting its code would hand out a
// redeemable secret for a card nobody has paid for. The buyer still sees
// the row (and its status), just not the secret.
//
// This is defence in depth: DebitInTx refuses non-active cards regardless.
func myGiftCardDTO(gc giftcard.GiftCard) gin.H {
	code := maskedCodeDisplay
	if gc.Status == giftcard.StatusActive {
		code = giftcard.FormatCodeDisplay(gc.Code)
	}
	return gin.H{
		"id":                       gc.ID.String(),
		"code_display":             code,
		"initial_balance":          gc.InitialBalance.String(),
		"current_balance":          gc.CurrentBalance.String(),
		"currency_code":            gc.CurrencyCode,
		"status":                   string(gc.Status),
		"recipient_email":          gc.RecipientEmail,
		"purchased_by_email":       gc.PurchasedByEmail,
		"purchased_via_storefront": gc.PurchasedViaStorefront,
		"expires_at":               gc.ExpiresAt,
		"created_at":               gc.CreatedAt,
	}
}

// PurchaseGiftCardRequest is the wire body for POST /gift-cards/purchase.
// The storefront page collects these fields; currency is derived server-
// side from the store (never trust the client).
type PurchaseGiftCardRequest struct {
	Amount         decimal.Decimal `json:"amount" binding:"required"`
	SenderName     *string         `json:"sender_name"`
	SenderEmail    *string         `json:"sender_email"`
	RecipientName  *string         `json:"recipient_name"`
	RecipientEmail *string         `json:"recipient_email" binding:"required,email"`
	Message        *string         `json:"message"`
	ExpiresAt      *string         `json:"expires_at"`
	// The buyer paying for the card — required for Stripe receipt email
	// and fraud / reconciliation.
	PurchaserName  string `json:"purchaser_name"`
	PurchaserEmail string `json:"purchaser_email" binding:"required,email"`
	// Where Stripe should redirect after payment. Client supplies these
	// (relative to the storefront origin) so the flow stays per-store.
	SuccessURL string `json:"success_url" binding:"required"`
	CancelURL  string `json:"cancel_url"  binding:"required"`
}

// Purchase handles POST /storefront/stores/:storeSlug/gift-cards/purchase.
// Creates a pending gift card + Stripe Checkout Session; customer is
// redirected to the returned URL to pay. Webhook activates the card.
func (h *GiftCardStorefrontHandler) Purchase(c *gin.Context) {
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
	tenantID, err := uuid.Parse(store.TenantID)
	if err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
			"error": "validation_failed", "message": "invalid tenant",
		})
		return
	}

	var req PurchaseGiftCardRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
			"error": "validation_failed", "message": err.Error(),
		})
		return
	}

	var expiresAt *time.Time
	if req.ExpiresAt != nil && *req.ExpiresAt != "" {
		t, err := time.Parse("2006-01-02", *req.ExpiresAt)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
				"error":   "validation_failed",
				"message": "expires_at must be YYYY-MM-DD",
			})
			return
		}
		// End-of-day so merchant-side expiry reads as "valid through X".
		eod := t.Add(24*time.Hour - time.Second)
		expiresAt = &eod
	}

	result, err := h.svc.Purchase(c.Request.Context(), giftcard.PurchaseInput{
		TenantID:        tenantID,
		StoreID:         storeID,
		InitialBalance:  req.Amount,
		CurrencyCode:    store.CurrencyCode,
		SenderName:      req.SenderName,
		SenderEmail:     req.SenderEmail,
		RecipientName:   req.RecipientName,
		RecipientEmail:  req.RecipientEmail,
		Message:         req.Message,
		ExpiresAt:       expiresAt,
		PurchaserName:   req.PurchaserName,
		PurchaserEmail:  req.PurchaserEmail,
		PaymentProvider: "stripe",
		SuccessURL:      req.SuccessURL,
		CancelURL:       req.CancelURL,
	})
	if err != nil {
		var ae *apperrors.Error
		if errors.As(err, &ae) && ae.Code == apperrors.CodeValidationFailed {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
				"error": "validation_failed", "message": ae.Message,
			})
			return
		}
		if h.logger != nil {
			h.logger.Error("gift card purchase error", "err", err.Error())
		}
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
			"error":   "internal",
			"message": "could not start gift card purchase",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data": gin.H{
			"gift_card_id": result.GiftCardID.String(),
			"checkout_url": result.CheckoutURL,
			"session_id":   result.CheckoutSessionID,
			"provider":     result.Provider,
		},
	})
}
