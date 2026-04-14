// Package admin — pages.go: HTTP handler for store content pages.
package admin

import (
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/mark8ly/marketplace-api/internal/page"
	"github.com/mark8ly/marketplace-api/pkg/apperrors"
)

// PagesHandler handles /admin/stores/:storeId/pages endpoints.
type PagesHandler struct {
	svc    *page.Service
	logger *slog.Logger
}

// NewPagesHandler constructs a PagesHandler.
func NewPagesHandler(svc *page.Service, logger *slog.Logger) *PagesHandler {
	return &PagesHandler{svc: svc, logger: logger}
}

// PageResponse is the wire DTO for a store content page.
type PageResponse struct {
	ID             string  `json:"id"`
	StoreID        string  `json:"store_id"`
	Slug           string  `json:"slug"`
	Title          string  `json:"title"`
	Body           string  `json:"body"`
	SEOTitle       *string `json:"seo_title,omitempty"`
	SEODescription *string `json:"seo_description,omitempty"`
	Published      bool    `json:"published"`
	SortOrder      int     `json:"sort_order"`
	CreatedAt      string  `json:"created_at"`
	UpdatedAt      string  `json:"updated_at"`
}

func toPageResponse(p page.Page) PageResponse {
	return PageResponse{
		ID:             p.ID.String(),
		StoreID:        p.StoreID.String(),
		Slug:           p.Slug,
		Title:          p.Title,
		Body:           p.Body,
		SEOTitle:       p.SEOTitle,
		SEODescription: p.SEODescription,
		Published:      p.Published,
		SortOrder:      p.SortOrder,
		CreatedAt:      p.CreatedAt.Format("2006-01-02T15:04:05Z"),
		UpdatedAt:      p.UpdatedAt.Format("2006-01-02T15:04:05Z"),
	}
}

// CreatePageRequest is the request body for POST .../pages.
type CreatePageRequest struct {
	Slug           string  `json:"slug"`
	Title          string  `json:"title"`
	Body           string  `json:"body"`
	SEOTitle       *string `json:"seo_title"`
	SEODescription *string `json:"seo_description"`
	Published      *bool   `json:"published"`
	SortOrder      int     `json:"sort_order"`
}

// UpdatePageRequest is the patch body for PATCH .../pages/:id.
type UpdatePageRequest struct {
	Slug           *string `json:"slug"`
	Title          *string `json:"title"`
	Body           *string `json:"body"`
	SEOTitle       *string `json:"seo_title"`
	SEODescription *string `json:"seo_description"`
	Published      *bool   `json:"published"`
	SortOrder      *int    `json:"sort_order"`
}

// List handles GET /admin/stores/:storeId/pages.
// Returns all pages for the store including drafts.
func (h *PagesHandler) List(c *gin.Context) {
	storeID, err := uuid.Parse(c.Param("storeId"))
	if err != nil {
		RespondErr(c, apperrors.ValidationFailed("storeId", "invalid uuid"), h.logger)
		return
	}

	pages, err := h.svc.ListByStore(c.Request.Context(), storeID, false)
	if err != nil {
		RespondErr(c, err, h.logger)
		return
	}

	resp := make([]PageResponse, 0, len(pages))
	for _, p := range pages {
		resp = append(resp, toPageResponse(p))
	}
	c.JSON(http.StatusOK, resp)
}

// Get handles GET /admin/stores/:storeId/pages/:id.
func (h *PagesHandler) Get(c *gin.Context) {
	storeID, err := uuid.Parse(c.Param("storeId"))
	if err != nil {
		RespondErr(c, apperrors.ValidationFailed("storeId", "invalid uuid"), h.logger)
		return
	}
	_ = storeID // ownership enforced by StoreMiddleware + tenant scoping in repo

	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		RespondErr(c, apperrors.ValidationFailed("id", "invalid uuid"), h.logger)
		return
	}

	p, err := h.svc.GetByID(c.Request.Context(), id)
	if err != nil {
		RespondErr(c, err, h.logger)
		return
	}
	if p == nil {
		RespondErr(c, apperrors.NotFound("page"), h.logger)
		return
	}

	c.JSON(http.StatusOK, toPageResponse(*p))
}

// Create handles POST /admin/stores/:storeId/pages.
func (h *PagesHandler) Create(c *gin.Context) {
	storeID, err := uuid.Parse(c.Param("storeId"))
	if err != nil {
		RespondErr(c, apperrors.ValidationFailed("storeId", "invalid uuid"), h.logger)
		return
	}
	tenantID, err := uuid.Parse(c.GetString("tenant_id"))
	if err != nil {
		RespondErr(c, apperrors.ValidationFailed("tenant_id", "invalid uuid"), h.logger)
		return
	}

	var req CreatePageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		RespondErr(c, apperrors.ValidationFailed("body", "invalid request body"), h.logger)
		return
	}

	p, err := h.svc.Create(c.Request.Context(), page.CreateInput{
		TenantID:       tenantID,
		StoreID:        storeID,
		Slug:           req.Slug,
		Title:          req.Title,
		Body:           req.Body,
		SEOTitle:       req.SEOTitle,
		SEODescription: req.SEODescription,
		Published:      req.Published,
		SortOrder:      req.SortOrder,
	})
	if err != nil {
		RespondErr(c, err, h.logger)
		return
	}

	c.JSON(http.StatusCreated, toPageResponse(*p))
}

// Update handles PATCH /admin/stores/:storeId/pages/:id.
func (h *PagesHandler) Update(c *gin.Context) {
	_, err := uuid.Parse(c.Param("storeId"))
	if err != nil {
		RespondErr(c, apperrors.ValidationFailed("storeId", "invalid uuid"), h.logger)
		return
	}

	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		RespondErr(c, apperrors.ValidationFailed("id", "invalid uuid"), h.logger)
		return
	}

	var req UpdatePageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		RespondErr(c, apperrors.ValidationFailed("body", "invalid request body"), h.logger)
		return
	}

	p, err := h.svc.Update(c.Request.Context(), id, page.UpdateInput{
		Slug:           req.Slug,
		Title:          req.Title,
		Body:           req.Body,
		SEOTitle:       req.SEOTitle,
		SEODescription: req.SEODescription,
		Published:      req.Published,
		SortOrder:      req.SortOrder,
	})
	if err != nil {
		RespondErr(c, err, h.logger)
		return
	}
	if p == nil {
		RespondErr(c, apperrors.NotFound("page"), h.logger)
		return
	}

	c.JSON(http.StatusOK, toPageResponse(*p))
}

// Delete handles DELETE /admin/stores/:storeId/pages/:id.
func (h *PagesHandler) Delete(c *gin.Context) {
	_, err := uuid.Parse(c.Param("storeId"))
	if err != nil {
		RespondErr(c, apperrors.ValidationFailed("storeId", "invalid uuid"), h.logger)
		return
	}

	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		RespondErr(c, apperrors.ValidationFailed("id", "invalid uuid"), h.logger)
		return
	}

	if err := h.svc.Delete(c.Request.Context(), id); err != nil {
		RespondErr(c, err, h.logger)
		return
	}

	c.Status(http.StatusNoContent)
}
