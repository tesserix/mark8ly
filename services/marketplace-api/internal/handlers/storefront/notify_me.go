// Package storefront — notify_me.go: NotifyMeHandler manages product
// notification subscriptions (back_in_stock, price_drop) for mobile
// storefront customers.
package storefront

import (
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
)

// NotifyMeHandler serves notify-me subscription endpoints.
type NotifyMeHandler struct {
	logger *slog.Logger
}

// NewNotifyMeHandler constructs a NotifyMeHandler.
func NewNotifyMeHandler(logger *slog.Logger) *NotifyMeHandler {
	return &NotifyMeHandler{logger: logger}
}

// Subscribe handles POST /mobile/storefront/stores/:storeSlug/products/:handle/notify-me.
// Stub — will be implemented in Task 2.5.
func (h *NotifyMeHandler) Subscribe(c *gin.Context) {
	c.JSON(http.StatusNotImplemented, gin.H{
		"error":   "not_implemented",
		"message": "Subscribe is not yet implemented",
	})
}

// Unsubscribe handles DELETE /mobile/storefront/stores/:storeSlug/products/:handle/notify-me/:notifyType.
// Stub — will be implemented in Task 2.5.
func (h *NotifyMeHandler) Unsubscribe(c *gin.Context) {
	c.JSON(http.StatusNotImplemented, gin.H{
		"error":   "not_implemented",
		"message": "Unsubscribe is not yet implemented",
	})
}
