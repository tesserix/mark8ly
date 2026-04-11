package store

import (
	"encoding/json"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/mark8ly/platform-api/internal/authz"
	apperrors "github.com/mark8ly/platform-api/pkg/errors"
)

// Handler is the HTTP layer for store endpoints.
//
// Phase Q: store update is FGA-gated on `can_edit_store_settings`
// which resolves transitively to tenant-level owners/admins via the
// `from parent` inheritance in the DSL. The admin BFF forwards the
// caller's uid in the request body so the handler can run the
// check without standing up its own auth middleware — same pattern
// as the tenant handler post-Phase-O.
type Handler struct {
	svc *Service
	fga authz.Client
}

func NewHandler(svc *Service, fga authz.Client) *Handler {
	return &Handler{svc: svc, fga: fga}
}

// Register mounts store routes onto the given gin.RouterGroups.
//
// Public routes:
//
//	GET /stores/slug-available?slug=...  used by onboarding wizard
//	                                     and the future add-store UI
//
// Internal routes (trusted in-cluster callers, i.e. admin BFF):
//
//	GET   /internal/stores/:id                 — fetch a single store
//	GET   /internal/tenants/:tenant_id/stores  — list a tenant's stores
//	PATCH /internal/stores/:id                 — edit a store (name)
func (h *Handler) Register(public *gin.RouterGroup, internal *gin.RouterGroup) {
	pub := public.Group("/stores")
	{
		pub.GET("/slug-available", h.checkSlugAvailable)
		// Phase S — public storefront lookup. The customer-facing
		// storefront app resolves {slug}.mark8ly.com to a store row
		// via this endpoint. Returns 404 for unknown or non-active
		// (suspended/archived) stores.
		pub.GET("/by-slug/:slug", h.getPublicStoreBySlug)
	}

	int := internal.Group("/stores")
	{
		int.GET("/:id", h.getStore)
		int.PATCH("/:id", h.updateStore)
		// Internal by-slug lookup — used by the admin middleware to
		// resolve {slug}-admin.mark8ly.com to a tenant_id for session
		// auto-switching. Returns tenant_id which the public endpoint
		// intentionally hides.
		int.GET("/by-slug/:slug", h.getInternalStoreBySlug)
	}

	// Listing + creating stores is addressed under the tenant they
	// belong to so the route lines up with REST nesting expectations.
	internal.GET("/tenants/:id/stores", h.listByTenant)
	internal.POST("/tenants/:id/stores", h.createForTenant)
}

// createStoreRequest is the body shape for POST /internal/tenants/:id/stores.
type createStoreRequest struct {
	Slug         string `json:"slug"`
	Name         string `json:"name"`
	CountryCode  string `json:"country_code"`
	CurrencyCode string `json:"currency_code"`
	Timezone     string `json:"timezone"`
	// UID is the calling user's GIP uid. Phase Q.2 gates store
	// creation on the tenant-level can_manage_stores permission.
	UID string `json:"uid"`
}

func (h *Handler) createForTenant(c *gin.Context) {
	var req createStoreRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "invalid_body",
			"message": "request body is not valid JSON",
		})
		return
	}
	tenantID := c.Param("id")

	if h.fga != nil {
		if req.UID == "" {
			c.JSON(http.StatusForbidden, gin.H{
				"error":   "missing_uid",
				"message": "authorization requires uid",
			})
			return
		}
		allowed, err := h.fga.Check(c.Request.Context(), req.UID, "can_manage_stores", tenantID)
		if err != nil {
			respondError(c, apperrors.Internal("authz_check_failed", "authorization check failed"))
			return
		}
		if !allowed {
			c.JSON(http.StatusForbidden, gin.H{
				"error":   "forbidden",
				"message": "you do not have permission to create stores",
			})
			return
		}
	}

	row, err := h.svc.Create(c.Request.Context(), CreateInput{
		TenantID:     tenantID,
		Slug:         req.Slug,
		Name:         req.Name,
		CountryCode:  req.CountryCode,
		CurrencyCode: req.CurrencyCode,
		Timezone:     req.Timezone,
	})
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"data": row})
}

// publicStore is the shape returned by the storefront lookup. Strips
// internal fields (tenant_id, logo_url nullability is preserved)
// and exposes only what a customer-facing page needs.
type publicStore struct {
	ID              string          `json:"id"`
	Slug            string          `json:"slug"`
	Name            string          `json:"name"`
	CountryCode     string          `json:"country_code"`
	CurrencyCode    string          `json:"currency_code"`
	Timezone        string          `json:"timezone"`
	LogoURL         string          `json:"logo_url,omitempty"`
	StorefrontTheme json.RawMessage `json:"storefront_theme"`
}

func (h *Handler) getPublicStoreBySlug(c *gin.Context) {
	s, err := h.svc.GetBySlug(c.Request.Context(), c.Param("slug"))
	if err != nil {
		respondError(c, err)
		return
	}
	if s.Status != StatusActive {
		c.JSON(http.StatusNotFound, gin.H{
			"error":   "store_not_found",
			"message": "this store is not currently available",
		})
		return
	}
	out := publicStore{
		ID:              s.ID,
		Slug:            s.Slug,
		Name:            s.Name,
		CountryCode:     s.CountryCode,
		CurrencyCode:    s.CurrencyCode,
		Timezone:        s.Timezone,
		StorefrontTheme: s.StorefrontTheme,
	}
	if s.LogoURL != nil {
		out.LogoURL = *s.LogoURL
	}
	c.JSON(http.StatusOK, gin.H{"data": out})
}

func (h *Handler) checkSlugAvailable(c *gin.Context) {
	slug := c.Query("slug")
	available, err := h.svc.IsSlugAvailable(c.Request.Context(), slug)
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"data": gin.H{"slug": slug, "available": available},
	})
}

func (h *Handler) getStore(c *gin.Context) {
	s, err := h.svc.GetByID(c.Request.Context(), c.Param("id"))
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": s})
}

// getInternalStoreBySlug returns the full store row (including tenant_id)
// for a given slug. Internal-only: used by the admin middleware to resolve
// {slug}-admin.mark8ly.com to the owning tenant for session auto-switching.
func (h *Handler) getInternalStoreBySlug(c *gin.Context) {
	s, err := h.svc.GetBySlug(c.Request.Context(), c.Param("slug"))
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": s})
}

func (h *Handler) listByTenant(c *gin.Context) {
	rows, err := h.svc.ListByTenant(c.Request.Context(), c.Param("id"))
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": rows})
}

type updateStoreRequest struct {
	Name            *string         `json:"name"`
	StorefrontTheme json.RawMessage `json:"storefront_theme"`
	// UID carries the calling user's GIP uid so we can run the FGA
	// can_edit_store_settings check. Same pattern the tenant handler
	// uses since Phase O.
	UID string `json:"uid"`
}

func (h *Handler) updateStore(c *gin.Context) {
	var req updateStoreRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "invalid_body",
			"message": "request body is not valid JSON",
		})
		return
	}
	storeID := c.Param("id")

	if h.fga != nil {
		if req.UID == "" {
			c.JSON(http.StatusForbidden, gin.H{
				"error":   "missing_uid",
				"message": "authorization requires uid",
			})
			return
		}
		allowed, err := h.fga.CheckObject(c.Request.Context(), req.UID, "can_edit_store_settings", "store", storeID)
		if err != nil {
			respondError(c, apperrors.Internal("authz_check_failed", "authorization check failed"))
			return
		}
		if !allowed {
			c.JSON(http.StatusForbidden, gin.H{
				"error":   "forbidden",
				"message": "you do not have permission to edit this store's settings",
			})
			return
		}
	}

	s, err := h.svc.Update(c.Request.Context(), storeID, UpdateInput{
		Name:            req.Name,
		StorefrontTheme: req.StorefrontTheme,
	})
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": s})
}

func respondError(c *gin.Context, err error) {
	if ae, ok := apperrors.As(err); ok {
		c.JSON(ae.Status, gin.H{"error": ae.Code, "message": ae.Message})
		return
	}
	c.JSON(http.StatusInternalServerError, gin.H{
		"error":   "internal_error",
		"message": "an unexpected error occurred",
	})
}
