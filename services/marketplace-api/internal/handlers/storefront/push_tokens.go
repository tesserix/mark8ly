// Package storefront — push_tokens.go: Mobile push token registration and
// deletion for storefront customers. Requires customer auth context.
package storefront

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/mark8ly/marketplace-api/internal/pushtoken"
	"github.com/mark8ly/marketplace-api/internal/stores"
)

// PushTokenHandler serves push token endpoints for mobile storefront customers.
type PushTokenHandler struct {
	repo   *pushtoken.Repository
	logger *slog.Logger
}

// NewPushTokenHandler constructs a PushTokenHandler.
func NewPushTokenHandler(repo *pushtoken.Repository, logger *slog.Logger) *PushTokenHandler {
	return &PushTokenHandler{repo: repo, logger: logger}
}

// Register handles POST /mobile/storefront/stores/:storeSlug/push-tokens.
// Binds JSON, gets customer_id from context (set by auth middleware),
// gets store from context, upserts token.
func (h *PushTokenHandler) Register(c *gin.Context) {
	var req pushtoken.RegisterTokenInput
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "bad_request",
			"message": err.Error(),
		})
		return
	}

	customerProfileID := c.GetString(CustomerProfileIDKey)
	if customerProfileID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error":   "unauthorized",
			"message": "Authentication required",
		})
		return
	}

	customerID, err := uuid.Parse(customerProfileID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "bad_request",
			"message": "invalid customer context",
		})
		return
	}

	storeVal, _ := c.Get("store")
	store, _ := storeVal.(*stores.Store)
	if store == nil {
		respondNotFound(c)
		return
	}

	tenantID, _ := uuid.Parse(store.TenantID)

	token := &pushtoken.StorefrontPushToken{
		TenantID:   tenantID,
		StoreSlug:  store.Slug,
		CustomerID: customerID,
		DeviceID:   req.DeviceID,
		Token:      req.Token,
		Platform:   req.Platform,
		UpdatedAt:  time.Now(),
	}

	if err := h.repo.Upsert(token); err != nil {
		h.logger.Error("storefront push token upsert failed",
			"error", err,
			"customer_id", customerID,
			"store_slug", store.Slug,
		)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "internal",
			"message": "failed to register push token",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"id":      token.ID,
		"message": "registered",
	})
}

// Delete handles DELETE /mobile/storefront/stores/:storeSlug/push-tokens/:tokenId.
// Deletes by ID with customer ownership check.
func (h *PushTokenHandler) Delete(c *gin.Context) {
	customerProfileID := c.GetString(CustomerProfileIDKey)
	if customerProfileID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error":   "unauthorized",
			"message": "Authentication required",
		})
		return
	}

	customerID, err := uuid.Parse(customerProfileID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "bad_request",
			"message": "invalid customer context",
		})
		return
	}

	tokenID, err := uuid.Parse(c.Param("tokenId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "bad_request",
			"message": "invalid token ID",
		})
		return
	}

	if err := h.repo.DeleteByID(customerID, tokenID); err != nil {
		h.logger.Error("storefront push token delete failed",
			"error", err,
			"customer_id", customerID,
			"token_id", tokenID,
		)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "internal",
			"message": "failed to delete push token",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "deleted"})
}
