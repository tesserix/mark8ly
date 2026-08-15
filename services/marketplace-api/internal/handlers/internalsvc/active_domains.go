package internalsvc

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// ActiveDomainsResponse is the wire shape returned by /internal/active-domains.
type ActiveDomainsResponse struct {
	Domains []string `json:"domains"`
}

// ActiveDomainsHandler answers GET /internal/active-domains with every verified
// merchant custom domain.
//
// Exists for the OpenPanel CORS reconciler: OpenPanel rejects an event whose
// Origin is absent from the project's cors list, and no wildcard can cover a
// merchant's own domain, so the list has to be enumerated periodically.
type ActiveDomainsHandler struct {
	db *gorm.DB
}

// NewActiveDomainsHandler constructs a handler bound to the marketplace-api DB.
func NewActiveDomainsHandler(db *gorm.DB) *ActiveDomainsHandler {
	return &ActiveDomainsHandler{db: db}
}

// Get is the Gin handler. Path: /internal/active-domains.
func (h *ActiveDomainsHandler) Get(c *gin.Context) {
	var domains []string
	// Only 'active' — a verifying row has no working TLS yet, so allowing its
	// origin would just widen the CORS list to a host nobody can reach.
	err := h.db.WithContext(c.Request.Context()).Raw(`
		SELECT domain
		  FROM custom_domains
		 WHERE status = 'active' AND domain <> ''
		 ORDER BY domain
	`).Scan(&domains).Error
	if err != nil {
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "lookup_failed"})
		return
	}

	if domains == nil {
		domains = []string{}
	}
	c.JSON(http.StatusOK, ActiveDomainsResponse{Domains: domains})
}

// Register mounts the endpoint onto the supplied /internal route group.
//
// Gated by X-Internal-Auth: a single slug→domain lookup is publicly observable,
// but the full enumeration of onboarded merchants is not.
func (h *ActiveDomainsHandler) Register(group *gin.RouterGroup, internalSecret string) {
	group.GET("/active-domains", RequireInternalAuth(internalSecret), h.Get)
}
