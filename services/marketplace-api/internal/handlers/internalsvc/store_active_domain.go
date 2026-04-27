package internalsvc

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// StoreActiveDomainResponse is the wire shape returned by the
// /internal/store-active-domain/:slug endpoint.
//
// `custom_domain` is empty when the store has no DomainStatusActive row,
// non-empty when it does. Callers (admin + storefront middleware) read
// the field and, when non-empty, 301 their request to that domain so
// the platform `*.mark8ly.com` URL is permanently retired for the
// store.
type StoreActiveDomainResponse struct {
	Slug         string `json:"slug"`
	CustomDomain string `json:"custom_domain"`
}

// StoreActiveDomainHandler answers GET /internal/store-active-domain/:slug.
//
// Resolves a slug → store, then joins to the latest active row in
// custom_domains. The endpoint is on marketplace-api because that's
// where custom_domains lives; platform-api owns slugs but doesn't see
// the custom-domain table.
//
//	200 OK   slug exists, body = StoreActiveDomainResponse
//	         (custom_domain may be empty)
//	404      slug not recognised — caller 404s the wildcard request.
//	500      transient DB error — caller fails open and renders the
//	         platform subdomain rather than a hard 404.
type StoreActiveDomainHandler struct {
	db *gorm.DB
}

// NewStoreActiveDomainHandler constructs a handler bound to the
// marketplace-api DB.
func NewStoreActiveDomainHandler(db *gorm.DB) *StoreActiveDomainHandler {
	return &StoreActiveDomainHandler{db: db}
}

// Get is the Gin handler. Path: /internal/store-active-domain/:slug.
func (h *StoreActiveDomainHandler) Get(c *gin.Context) {
	slug := strings.ToLower(strings.TrimSpace(c.Param("slug")))
	if slug == "" {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "missing_slug"})
		return
	}

	// LEFT JOIN so a slug with no active custom domain still returns 200
	// with an empty domain field. The middleware uses the empty value
	// to mean "platform subdomain owns this store, render normally".
	//
	// We explicitly filter on status = 'active' so a pending /
	// verifying / error row doesn't accidentally retire the platform
	// subdomain before the merchant's DNS + SSL provisioning finishes.
	var row struct {
		StoreSlug    string
		CustomDomain *string
	}
	err := h.db.WithContext(c.Request.Context()).Raw(`
		SELECT s.slug AS store_slug,
		       cd.domain AS custom_domain
		  FROM stores s
		  LEFT JOIN custom_domains cd
		    ON cd.store_id = s.id AND cd.status = 'active'
		 WHERE s.slug = ?
		 ORDER BY cd.updated_at DESC NULLS LAST
		 LIMIT 1
	`, slug).Row().Scan(&row.StoreSlug, &row.CustomDomain)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) || isNoRowsError(err) {
			c.AbortWithStatusJSON(http.StatusNotFound, gin.H{"error": "slug_not_found"})
			return
		}
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "lookup_failed"})
		return
	}

	resp := StoreActiveDomainResponse{Slug: row.StoreSlug}
	if row.CustomDomain != nil {
		resp.CustomDomain = *row.CustomDomain
	}
	c.JSON(http.StatusOK, resp)
}

// Register mounts the endpoint onto the supplied /internal route group.
//
// Intentionally NOT gated by X-Internal-Auth — the response is just
// `slug → maybe-custom-domain`, both already publicly observable
// (the slug appears in the storefront URL bar; the active custom
// domain is the storefront's canonical host). Matches the existing
// platform-api `/internal/stores/by-slug/:slug` pattern, which the
// admin middleware also calls without an auth header. Network access
// is restricted by k8s NetworkPolicy.
//
// The `internalSecret` parameter is accepted for call-site symmetry
// with the other internalsvc handlers but ignored.
func (h *StoreActiveDomainHandler) Register(group *gin.RouterGroup, _internalSecret string) {
	group.GET("/store-active-domain/:slug", h.Get)
}
