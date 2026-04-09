// Package admin — categories.go: HTTP handler for the per-store
// category CRUD surface mounted at /admin/stores/:storeId/categories.
// The handler layer is a thin wire DTO ↔ category service mapper; the
// service owns validation, slug generation, outbox enqueue, and the
// refuse-on-populated delete semantics.
package admin

import (
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/mark8ly/marketplace-api/internal/category"
	"github.com/mark8ly/marketplace-api/pkg/apperrors"
)

// CategoryHandler bundles dependencies for the admin category endpoints.
type CategoryHandler struct {
	svc    *category.Service
	repo   category.Repository
	logger *slog.Logger
}

// NewCategoryHandler constructs a CategoryHandler.
func NewCategoryHandler(svc *category.Service, repo category.Repository, logger *slog.Logger) *CategoryHandler {
	return &CategoryHandler{svc: svc, repo: repo, logger: logger}
}

// List handles GET /admin/stores/:storeId/categories.
func (h *CategoryHandler) List(c *gin.Context) {
	storeID := c.Param("storeId")
	tenantID := c.GetString("tenant_id")

	cats, err := h.repo.ListByStore(c.Request.Context(), storeID, tenantID)
	if err != nil {
		RespondErr(c, err, h.logger)
		return
	}
	out := make([]AdminCategoryResponse, 0, len(cats))
	for i := range cats {
		out = append(out, ToAdminCategoryResponse(&cats[i]))
	}
	c.JSON(http.StatusOK, gin.H{"data": out})
}

// Create handles POST /admin/stores/:storeId/categories.
func (h *CategoryHandler) Create(c *gin.Context) {
	storeID := c.Param("storeId")
	tenantID := c.GetString("tenant_id")

	var req CreateCategoryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		RespondErr(c, apperrors.ValidationFailed("body", err.Error()), h.logger)
		return
	}

	svcReq := toServiceCreateCategory(req, tenantID, storeID)
	cat, err := h.svc.Create(c.Request.Context(), svcReq)
	if err != nil {
		RespondErr(c, err, h.logger)
		return
	}
	c.JSON(http.StatusCreated, ToAdminCategoryResponse(cat))
}

// Patch handles PATCH /admin/stores/:storeId/categories/:id.
func (h *CategoryHandler) Patch(c *gin.Context) {
	storeID := c.Param("storeId")
	id := c.Param("id")
	tenantID := c.GetString("tenant_id")

	var req UpdateCategoryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		RespondErr(c, apperrors.ValidationFailed("body", err.Error()), h.logger)
		return
	}
	svcReq := toServiceUpdateCategory(req, id, tenantID, storeID)
	cat, err := h.svc.Update(c.Request.Context(), svcReq)
	if err != nil {
		RespondErr(c, err, h.logger)
		return
	}
	c.JSON(http.StatusOK, ToAdminCategoryResponse(cat))
}

// Delete handles DELETE /admin/stores/:storeId/categories/:id.
func (h *CategoryHandler) Delete(c *gin.Context) {
	storeID := c.Param("storeId")
	id := c.Param("id")
	tenantID := c.GetString("tenant_id")

	if err := h.svc.Delete(c.Request.Context(), id, storeID, tenantID); err != nil {
		RespondErr(c, err, h.logger)
		return
	}
	c.Status(http.StatusNoContent)
}
