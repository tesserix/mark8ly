// Package admin — arbitrage_appeal.go: handler for merchant-initiated
// geo-pricing arbitrage appeals (§18.8.1).
package admin

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/mark8ly/marketplace-api/internal/arbitrage"
	"github.com/mark8ly/marketplace-api/internal/authz"
)

// ArbitrageAppealHandler handles POST /admin/stores/:storeId/arbitrage-appeal.
type ArbitrageAppealHandler struct {
	svc *arbitrage.AppealService
}

// NewArbitrageAppealHandler constructs an ArbitrageAppealHandler.
func NewArbitrageAppealHandler(svc *arbitrage.AppealService) *ArbitrageAppealHandler {
	return &ArbitrageAppealHandler{svc: svc}
}

// appealBody is the JSON request body for the appeal endpoint.
type appealBody struct {
	// Jurisdiction is the ISO-3166-1 alpha-2 code the merchant claims to
	// operate from. Required; validated as a known 2-letter alpha code.
	Jurisdiction string `json:"jurisdiction" binding:"required"`
	// Justification is optional free text explaining the mismatch (max 1000 chars).
	Justification string `json:"justification"`
	// DocumentURL is an optional gs:// URI to a supporting document uploaded
	// via the existing media endpoint. The appeal endpoint is stateless —
	// upload the document first, then submit its URL here.
	DocumentURL string `json:"document_url"`
}

// Submit handles POST /admin/stores/:storeId/arbitrage-appeal.
//
// Returns:
//   - 200: appeal submitted; audit row updated.
//   - 400: missing or invalid jurisdiction code.
//   - 404: no open arbitrage flag for the store.
//   - 403: caller does not have MPCanManageSubscription permission (enforced by middleware).
func (h *ArbitrageAppealHandler) Submit(c *gin.Context) {
	storeID, err := uuid.Parse(c.Param("storeId"))
	if err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
			"error":   "arbitrage_appeal_invalid_jurisdiction",
			"message": "invalid store id",
		})
		return
	}

	tenantIDStr := c.GetString("tenant_id")
	tenantID, err := uuid.Parse(tenantIDStr)
	if err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
			"error":   "invalid_tenant",
			"message": "missing or invalid tenant_id",
		})
		return
	}

	userIDStr := c.GetString("user_id")
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
			"error":   "invalid_user",
			"message": "missing or invalid user_id",
		})
		return
	}

	var body appealBody
	if err := c.ShouldBindJSON(&body); err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
			"error":   "arbitrage_appeal_invalid_jurisdiction",
			"message": "jurisdiction is required",
		})
		return
	}

	if !arbitrage.IsKnownCountry(body.Jurisdiction) {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
			"error":   "arbitrage_appeal_invalid_jurisdiction",
			"message": "jurisdiction must be a valid ISO-3166-1 alpha-2 code",
		})
		return
	}

	submitErr := h.svc.Submit(c.Request.Context(), arbitrage.AppealInput{
		TenantID:      tenantID,
		StoreID:       storeID,
		Jurisdiction:  body.Jurisdiction,
		Justification: body.Justification,
		DocumentURL:   body.DocumentURL,
		ActorUserID:   userID,
	})
	switch {
	case errors.Is(submitErr, arbitrage.ErrNoOpenFlag):
		c.AbortWithStatusJSON(http.StatusNotFound, gin.H{
			"error":   "arbitrage_appeal_no_open_flag",
			"message": "no open arbitrage flag found for this store",
		})
	case submitErr != nil:
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
			"error":   "arbitrage_appeal_failed",
			"message": "internal error processing appeal",
		})
	default:
		c.JSON(http.StatusOK, gin.H{
			"data":  gin.H{"status": "submitted"},
			"error": nil,
		})
	}
}

// RegisterArbitrageAppeal mounts the appeal endpoint on the given store
// sub-router. The appeal requires SubscriptionEditRole (owner) — only the
// store owner can submit a jurisdiction challenge.
func RegisterArbitrageAppeal(storeRoute gin.IRouter, h *ArbitrageAppealHandler, fgaMw *authz.Middleware) {
	storeRoute.POST("/arbitrage-appeal",
		fgaMw.RequireTenantRelation(authz.SubscriptionEditRole),
		h.Submit,
	)
}
