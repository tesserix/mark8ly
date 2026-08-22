// Package admin — bulk.go: HTTP handler for the bulk product actions
// endpoint POST /admin/stores/:storeId/products/bulk. Per M7d spec:
// always returns 200 with per-id results, never a global 4xx when at
// least one row succeeded. FGA is checked per product ID by the
// service layer via the injected check function.
package admin

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/mark8ly/marketplace-api/internal/authz"
	"github.com/mark8ly/marketplace-api/internal/product"
	"github.com/mark8ly/marketplace-api/pkg/apperrors"
)

// BulkHandler bundles dependencies for the bulk products endpoint.
type BulkHandler struct {
	svc        *product.Service
	authzCheck authz.Client
	logger     *slog.Logger
}

// NewBulkHandler constructs a BulkHandler.
func NewBulkHandler(svc *product.Service, authzCheck authz.Client, logger *slog.Logger) *BulkHandler {
	return &BulkHandler{svc: svc, authzCheck: authzCheck, logger: logger}
}

// BulkProductRequest is the wire body for POST /products/bulk.
type BulkProductRequest struct {
	Action     string      `json:"action" binding:"required"`
	ProductIDs []string    `json:"product_ids" binding:"required,min=1"`
	Params     *BulkParams `json:"params,omitempty"`
}

// BulkParams carries action-specific parameters.
type BulkParams struct {
	CategoryIDs []string `json:"category_ids,omitempty"`
}

// Bulk handles POST /admin/stores/:storeId/products/bulk.
func (h *BulkHandler) Bulk(c *gin.Context) {
	storeID := c.Param("storeId")
	tenantID := c.GetString("tenant_id")
	userID := c.GetString("user_id")

	var req BulkProductRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		RespondErr(c, apperrors.ValidationFailed("body", err.Error()), h.logger)
		return
	}

	action := product.BulkAction(req.Action)
	if !product.ValidBulkActions[action] {
		RespondErr(c, apperrors.ValidationFailed("action",
			"must be one of: archive, unarchive, publish, unpublish, delete, assign_category"), h.logger)
		return
	}

	if len(req.ProductIDs) > product.MaxBulkIDs {
		c.AbortWithStatusJSON(http.StatusUnprocessableEntity, gin.H{
			"error":   "bulk_cap_exceeded",
			"message": "cannot process more than 100 products in a single bulk request",
		})
		return
	}

	if action == product.BulkActionAssignCategory {
		if req.Params == nil || len(req.Params.CategoryIDs) == 0 {
			RespondErr(c, apperrors.ValidationFailed("params.category_ids",
				"category_ids required for assign_category action"), h.logger)
			return
		}
	}

	var categoryIDs []string
	if req.Params != nil {
		categoryIDs = req.Params.CategoryIDs
	}

	svcReq := product.BulkRequest{
		StoreID:     storeID,
		TenantID:    tenantID,
		UserID:      userID,
		Action:      action,
		ProductIDs:  req.ProductIDs,
		CategoryIDs: categoryIDs,
	}

	// Build the per-id FGA check function. The bulk endpoint requires
	// admin-level permission (same as single-product mutations). The
	// check is against the tenant, not the individual product — matching
	// the existing RequireTenantRelation pattern but done per-id so
	// partial failures surface correctly.
	fgaCheck := func(ctx context.Context, uid, _ string) (bool, error) {
		requiredRole := requiredRoleForAction(action)
		return h.authzCheck.Check(ctx, uid, string(requiredRole), tenantID)
	}

	results := h.svc.Bulk(c.Request.Context(), svcReq, fgaCheck)

	c.JSON(http.StatusOK, gin.H{"results": results})
}

// requiredRoleForAction maps bulk actions to the minimum FGA role.
func requiredRoleForAction(action product.BulkAction) authz.Role {
	switch action {
	case product.BulkActionDelete:
		return authz.RoleOwner
	default:
		return authz.RoleAdmin
	}
}
