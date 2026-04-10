// Package admin — abandoned_carts.go: HTTP handler for the admin
// abandoned-carts subset mounted under /api/v1/admin/stores/:storeId/abandoned-carts.
package admin

import (
	"log/slog"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/mark8ly/marketplace-api/internal/order"
	"github.com/mark8ly/marketplace-api/pkg/apperrors"
)

// AbandonedCartsHandler bundles dependencies for the admin abandoned-cart endpoints.
type AbandonedCartsHandler struct {
	svc    *order.AbandonedCartService
	logger *slog.Logger
}

// NewAbandonedCartsHandler constructs an AbandonedCartsHandler.
func NewAbandonedCartsHandler(svc *order.AbandonedCartService, logger *slog.Logger) *AbandonedCartsHandler {
	return &AbandonedCartsHandler{svc: svc, logger: logger}
}

// List handles GET /admin/stores/:storeId/abandoned-carts.
func (h *AbandonedCartsHandler) List(c *gin.Context) {
	storeID, err := uuid.Parse(c.Param("storeId"))
	if err != nil {
		RespondErr(c, apperrors.ValidationFailed("storeId", "must be a uuid"), h.logger)
		return
	}
	limit := 50
	if l := c.Query("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 && n <= 200 {
			limit = n
		}
	}
	rows, err := h.svc.List(c.Request.Context(), storeID, limit)
	if err != nil {
		RespondErr(c, err, h.logger)
		return
	}
	out := make([]AdminAbandonedCartResponse, 0, len(rows))
	for i := range rows {
		out = append(out, ToAdminAbandonedCartResponse(&rows[i]))
	}
	c.JSON(http.StatusOK, gin.H{"data": out})
}

// Get handles GET /admin/stores/:storeId/abandoned-carts/:id.
func (h *AbandonedCartsHandler) Get(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		RespondErr(c, apperrors.ValidationFailed("id", "must be a uuid"), h.logger)
		return
	}
	cart, err := h.svc.Get(c.Request.Context(), id)
	if err != nil {
		RespondErr(c, err, h.logger)
		return
	}
	if cart.StoreID.String() != c.Param("storeId") {
		RespondErr(c, apperrors.NotFound("abandoned_cart"), h.logger)
		return
	}
	c.JSON(http.StatusOK, ToAdminAbandonedCartResponse(cart))
}

// TriggerRecoveryEmail handles POST /admin/stores/:storeId/abandoned-carts/:id/recovery-email.
func (h *AbandonedCartsHandler) TriggerRecoveryEmail(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		RespondErr(c, apperrors.ValidationFailed("id", "must be a uuid"), h.logger)
		return
	}
	if err := h.svc.TriggerRecoveryEmail(c.Request.Context(), id); err != nil {
		RespondErr(c, err, h.logger)
		return
	}
	cart, err := h.svc.Get(c.Request.Context(), id)
	if err != nil {
		RespondErr(c, err, h.logger)
		return
	}
	c.JSON(http.StatusOK, ToAdminAbandonedCartResponse(cart))
}
