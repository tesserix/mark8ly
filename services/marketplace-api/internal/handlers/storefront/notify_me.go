// Package storefront — notify_me.go: NotifyMeHandler manages product
// notification subscriptions (back_in_stock, price_drop) for mobile
// storefront customers.
package storefront

import (
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/mark8ly/marketplace-api/internal/notifyme"
	"github.com/mark8ly/marketplace-api/internal/stores"
)

// NotifyMeHandler serves notify-me subscription endpoints.
type NotifyMeHandler struct {
	repo   *notifyme.Repository
	logger *slog.Logger
}

// NewNotifyMeHandler constructs a NotifyMeHandler.
func NewNotifyMeHandler(repo *notifyme.Repository, logger *slog.Logger) *NotifyMeHandler {
	return &NotifyMeHandler{repo: repo, logger: logger}
}

// Subscribe handles POST /mobile/storefront/stores/:storeSlug/products/:handle/notify-me.
// Upserts a notification subscription for the authenticated customer.
func (h *NotifyMeHandler) Subscribe(c *gin.Context) {
	profile := mustGetCustomerProfile(c)
	if profile == nil {
		return
	}

	storeVal, _ := c.Get("store")
	store, _ := storeVal.(*stores.Store)
	if store == nil {
		respondNotFound(c)
		return
	}

	var req notifyme.SubscribeInput
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "validation_error",
			"message": err.Error(),
		})
		return
	}

	productID, err := uuid.Parse(req.ProductID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "validation_error",
			"message": "invalid product_id",
		})
		return
	}

	tenantID, _ := uuid.Parse(store.TenantID)

	sub := &notifyme.ProductNotifySubscription{
		TenantID:   tenantID,
		StoreSlug:  store.Slug,
		CustomerID: profile.ID,
		ProductID:  productID,
		NotifyType: req.NotifyType,
	}

	if err := h.repo.Upsert(sub); err != nil {
		h.logger.Error("notify-me subscribe failed",
			"error", err,
			"customer_id", profile.ID,
			"product_id", req.ProductID,
		)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "internal",
			"message": "failed to subscribe",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"id":      sub.ID,
		"message": "subscribed",
	})
}

// Unsubscribe handles DELETE /mobile/storefront/stores/:storeSlug/products/:handle/notify-me/:notifyType.
// Removes the notification subscription for the authenticated customer.
func (h *NotifyMeHandler) Unsubscribe(c *gin.Context) {
	profile := mustGetCustomerProfile(c)
	if profile == nil {
		return
	}

	notifyType := c.Param("notifyType")
	if notifyType != notifyme.TypeBackInStock && notifyType != notifyme.TypePriceDrop {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "validation_error",
			"message": "notify_type must be back_in_stock or price_drop",
		})
		return
	}

	// The product_id needs to come from somewhere — in the mobile route,
	// :handle is the product handle, not the UUID. We accept product_id
	// as a query parameter for the delete endpoint.
	productIDStr := c.Query("product_id")
	if productIDStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "validation_error",
			"message": "product_id query parameter is required",
		})
		return
	}

	productID, err := uuid.Parse(productIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "validation_error",
			"message": "invalid product_id",
		})
		return
	}

	if err := h.repo.Delete(profile.ID, productID, notifyType); err != nil {
		h.logger.Error("notify-me unsubscribe failed",
			"error", err,
			"customer_id", profile.ID,
			"product_id", productID,
		)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "internal",
			"message": "failed to unsubscribe",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "unsubscribed"})
}
