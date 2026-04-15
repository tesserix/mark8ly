// Package testroutes hosts handlers used only by the e2e/visual test
// suites — never enabled in production. Wiring is gated by the
// MARKETPLACE_API_ENABLE_TEST_ROUTES env var in main.go so a forgotten
// route can never accidentally leak into a deployed environment.
//
// All routes mount under /api/v1/test/.
package testroutes

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/mark8ly/marketplace-api/internal/branding"
	"github.com/mark8ly/marketplace-api/internal/stores"
)

// BrandingSeeder seeds branding (layout_variant + homepage_content) for
// a store identified by slug. Powers the storefront visual-regression
// suite which needs to flip layouts and content between snapshots.
type BrandingSeeder struct {
	stores   stores.Repository
	branding *branding.Service
	logger   *slog.Logger
}

func NewBrandingSeeder(s stores.Repository, b *branding.Service, log *slog.Logger) *BrandingSeeder {
	return &BrandingSeeder{stores: s, branding: b, logger: log}
}

type seedBrandingRequest struct {
	Slug            string          `json:"slug"`
	LayoutVariant   *string         `json:"layout_variant"`
	HomepageContent json.RawMessage `json:"homepage_content"`
}

// Seed handles POST /api/v1/test/branding.
func (h *BrandingSeeder) Seed(c *gin.Context) {
	var req seedBrandingRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_json", "message": err.Error()})
		return
	}
	if req.Slug == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing_slug", "message": "slug required"})
		return
	}

	store, err := h.stores.GetBySlug(c.Request.Context(), req.Slug)
	if err != nil {
		if errors.Is(err, stores.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "store_not_found", "message": "no store with that slug"})
			return
		}
		h.logger.Error("test seed: lookup store", "err", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal"})
		return
	}

	storeID, err := uuid.Parse(store.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "store_id_parse"})
		return
	}
	tenantID, err := uuid.Parse(store.TenantID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "tenant_id_parse"})
		return
	}

	in := branding.UpdateInput{
		TenantID:      tenantID,
		StoreID:       storeID,
		LayoutVariant: req.LayoutVariant,
	}
	// Treat presence of the homepage_content key as "set this blob",
	// even if it's an empty object — empty == merchant cleared it.
	if len(req.HomepageContent) > 0 {
		raw := json.RawMessage(req.HomepageContent)
		in.HomepageContent = &raw
	}

	updated, err := h.branding.Update(c.Request.Context(), in)
	if err != nil {
		h.logger.Warn("test seed: branding update failed", "err", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "update_failed", "message": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"store_id":         updated.StoreID.String(),
		"layout_variant":   updated.LayoutVariant,
		"homepage_content": json.RawMessage(updated.HomepageContent),
	})
}

// Register mounts the test routes under the given group.
func (h *BrandingSeeder) Register(r *gin.RouterGroup) {
	r.POST("/branding", h.Seed)
}
