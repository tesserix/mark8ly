// Package storefront — pages.go: public content page endpoints.
package storefront

import (
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/mark8ly/marketplace-api/internal/page"
	"github.com/mark8ly/marketplace-api/internal/stores"
)

// PagesHandler handles public page endpoints under
// /storefront/stores/:storeSlug/pages.
type PagesHandler struct {
	svc    *page.Service
	logger *slog.Logger
}

// NewPagesHandler constructs a storefront PagesHandler.
func NewPagesHandler(svc *page.Service, logger *slog.Logger) *PagesHandler {
	return &PagesHandler{svc: svc, logger: logger}
}

// PageSummaryResponse is the wire DTO for the list endpoint — slug + title only.
type PageSummaryResponse struct {
	Slug  string `json:"slug"`
	Title string `json:"title"`
}

// PublicPageResponse is the full wire DTO for the single-page endpoint.
type PublicPageResponse struct {
	Slug           string  `json:"slug"`
	Title          string  `json:"title"`
	Body           string  `json:"body"`
	SEOTitle       *string `json:"seo_title,omitempty"`
	SEODescription *string `json:"seo_description,omitempty"`
}

func resolveStore(c *gin.Context) (*stores.Store, bool) {
	storeVal, exists := c.Get("store")
	if !exists {
		return nil, false
	}
	store, ok := storeVal.(*stores.Store)
	return store, ok && store != nil
}

// List handles GET /storefront/stores/:storeSlug/pages.
// Returns slug + title summaries for all published pages.
func (h *PagesHandler) List(c *gin.Context) {
	store, ok := resolveStore(c)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "not_found", "message": "store not found"})
		return
	}
	storeID, err := uuid.Parse(store.ID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "not_found", "message": "store not found"})
		return
	}

	pages, err := h.svc.ListByStore(c.Request.Context(), storeID, true)
	if err != nil {
		respondInternal(c, h.logger, err)
		return
	}

	resp := make([]PageSummaryResponse, 0, len(pages))
	for _, p := range pages {
		resp = append(resp, PageSummaryResponse{Slug: p.Slug, Title: p.Title})
	}
	c.JSON(http.StatusOK, resp)
}

// Get handles GET /storefront/stores/:storeSlug/pages/:pageSlug.
// Returns the full public page or 404 if not found or unpublished.
func (h *PagesHandler) Get(c *gin.Context) {
	store, ok := resolveStore(c)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "not_found", "message": "store not found"})
		return
	}
	storeID, err := uuid.Parse(store.ID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "not_found", "message": "store not found"})
		return
	}

	slug := c.Param("pageSlug")

	p, err := h.svc.GetBySlugForStorefront(c.Request.Context(), storeID, slug)
	if err != nil {
		respondInternal(c, h.logger, err)
		return
	}
	if p == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "not_found", "message": "page not found"})
		return
	}

	c.JSON(http.StatusOK, PublicPageResponse{
		Slug:           p.Slug,
		Title:          p.Title,
		Body:           p.Body,
		SEOTitle:       p.SEOTitle,
		SEODescription: p.SEODescription,
	})
}
