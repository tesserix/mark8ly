// Package storefront — mobile_stubs.go: Stub methods for mobile storefront
// routes that reference handler methods not yet implemented. These return
// 501 Not Implemented and will be replaced with real implementations in
// Task 2.5.
package storefront

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// ListOrders handles GET /mobile/storefront/stores/:storeSlug/orders.
// Stub — will be implemented in Task 2.5.
func (h *OrderDetailHandler) ListOrders(c *gin.Context) {
	c.JSON(http.StatusNotImplemented, gin.H{
		"error":   "not_implemented",
		"message": "ListOrders is not yet implemented",
	})
}

// Register handles POST /mobile/storefront/stores/:storeSlug/account/register.
// Stub — will be implemented in Task 2.5.
func (h *CustomerAccountHandler) Register(c *gin.Context) {
	c.JSON(http.StatusNotImplemented, gin.H{
		"error":   "not_implemented",
		"message": "Register is not yet implemented",
	})
}

// CheckEmail handles GET /mobile/storefront/stores/:storeSlug/account/check-email.
// Stub — will be implemented in Task 2.5.
func (h *CustomerAccountHandler) CheckEmail(c *gin.Context) {
	c.JSON(http.StatusNotImplemented, gin.H{
		"error":   "not_implemented",
		"message": "CheckEmail is not yet implemented",
	})
}

// ListMyReviews handles GET /mobile/storefront/stores/:storeSlug/my-reviews.
// Stub — will be implemented in Task 2.5.
func (h *ReviewsHandler) ListMyReviews(c *gin.Context) {
	c.JSON(http.StatusNotImplemented, gin.H{
		"error":   "not_implemented",
		"message": "ListMyReviews is not yet implemented",
	})
}
