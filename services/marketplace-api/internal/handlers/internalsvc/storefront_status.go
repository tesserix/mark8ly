package internalsvc

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// StorefrontStatusResponse is the contract consumed by the Cloudflare Worker.
// Flat and small — KV caches it as JSON for 15 minutes.
type StorefrontStatusResponse struct {
	Status   string                  `json:"status"`
	Plan     string                  `json:"plan"`
	Branding StorefrontBrandingBlock `json:"branding"`
	// CurrentPeriodEnd is included so the Worker can sanity-check grace-window
	// edge cases. Empty when the subscription has never billed.
	CurrentPeriodEnd string `json:"current_period_end,omitempty"`
}

// StorefrontBrandingBlock is the minimal branding payload the closed page
// template needs. Logo URL and support email may be empty — the template
// degrades gracefully.
type StorefrontBrandingBlock struct {
	Name         string `json:"name"`
	LogoURL      string `json:"logo_url"`
	SupportEmail string `json:"support_email"`
}

// StorefrontStatusHandler answers GET /internal/storefront-status/:host.
// Resolves host → store via stores.slug (for *.mark8ly.com subdomains) or
// custom_domains.domain (for merchant-owned domains).
type StorefrontStatusHandler struct {
	db *gorm.DB
}

// NewStorefrontStatusHandler constructs a handler bound to the marketplace DB.
func NewStorefrontStatusHandler(db *gorm.DB) *StorefrontStatusHandler {
	return &StorefrontStatusHandler{db: db}
}

const platformDomainSuffix = ".mark8ly.com"

// Get is the Gin handler. Path: /internal/storefront-status/:host.
//
//	200 OK   — storefront resolved; body = StorefrontStatusResponse.
//	404      — host not recognised; Worker treats as pass-through.
//	500      — transient DB error; Worker treats as pass-through (fail-open).
func (h *StorefrontStatusHandler) Get(c *gin.Context) {
	host := strings.ToLower(strings.TrimSpace(c.Param("host")))
	if host == "" {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "missing_host"})
		return
	}

	storeID, ok := h.resolveStoreID(c, host)
	if !ok {
		c.AbortWithStatusJSON(http.StatusNotFound, gin.H{"error": "not_found"})
		return
	}

	var row struct {
		Status           string
		Plan             string
		Name             string
		LogoURL          string
		SupportEmail     string
		CurrentPeriodEnd *string
	}
	err := h.db.WithContext(c.Request.Context()).Raw(`
		SELECT ss.status, ss.plan,
		       s.name,
		       COALESCE(sb.logo_url, '')      AS logo_url,
		       COALESCE(sb.support_email, '') AS support_email,
		       ss.current_period_end::text    AS current_period_end
		  FROM stores s
		  JOIN store_subscriptions ss ON ss.store_id = s.id
		  LEFT JOIN store_branding sb ON sb.store_id = s.id
		 WHERE s.id = ?
		 LIMIT 1
	`, storeID).Row().Scan(&row.Status, &row.Plan, &row.Name, &row.LogoURL, &row.SupportEmail, &row.CurrentPeriodEnd)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.AbortWithStatusJSON(http.StatusNotFound, gin.H{"error": "not_found"})
			return
		}
		// No subscription yet (signup → first invoice race) is a 404 too.
		if isNoRowsError(err) {
			c.AbortWithStatusJSON(http.StatusNotFound, gin.H{"error": "not_found"})
			return
		}
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "lookup_failed"})
		return
	}

	resp := StorefrontStatusResponse{
		Status: row.Status,
		Plan:   row.Plan,
		Branding: StorefrontBrandingBlock{
			Name:         row.Name,
			LogoURL:      row.LogoURL,
			SupportEmail: row.SupportEmail,
		},
	}
	if row.CurrentPeriodEnd != nil {
		resp.CurrentPeriodEnd = *row.CurrentPeriodEnd
	}
	c.JSON(http.StatusOK, resp)
}

// resolveStoreID returns (store_id, true) when host maps to a known store.
// Resolution order:
//  1. *.mark8ly.com → strip suffix, look up by stores.slug.
//  2. otherwise → look up custom_domains.domain.
//
// We deliberately don't fall back from step 1 to step 2 — a missing slug
// match is a 404, not a custom-domain probe.
func (h *StorefrontStatusHandler) resolveStoreID(c *gin.Context, host string) (string, bool) {
	if strings.HasSuffix(host, platformDomainSuffix) {
		slug := strings.TrimSuffix(host, platformDomainSuffix)
		if slug == "" || strings.Contains(slug, ".") {
			return "", false
		}
		var id string
		err := h.db.WithContext(c.Request.Context()).
			Raw(`SELECT id::text FROM stores WHERE slug = ? LIMIT 1`, slug).
			Row().Scan(&id)
		if err != nil || id == "" {
			return "", false
		}
		return id, true
	}

	var id string
	err := h.db.WithContext(c.Request.Context()).
		Raw(`SELECT store_id::text FROM custom_domains WHERE domain = ? AND status = 'active' LIMIT 1`, host).
		Row().Scan(&id)
	if err != nil || id == "" {
		return "", false
	}
	return id, true
}

// isNoRowsError detects the "no rows returned" flavour of database/sql /
// GORM. We treat this as not-found rather than 500 because a host that
// resolves to a store-without-subscription is a data-shape oddity (signup
// race), not a transient failure.
func isNoRowsError(err error) bool {
	return err != nil && strings.Contains(err.Error(), "no rows in result set")
}

// Register mounts the endpoint onto the supplied /internal route group with
// the standard X-Internal-Auth gate.
func (h *StorefrontStatusHandler) Register(group *gin.RouterGroup, internalSecret string) {
	group.GET("/storefront-status/:host", RequireInternalAuth(internalSecret), h.Get)
}
