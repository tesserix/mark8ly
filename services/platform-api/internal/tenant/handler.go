package tenant

import (
	"net/http"

	"github.com/gin-gonic/gin"

	apperrors "github.com/mark8ly/platform-api/pkg/errors"
)

// Handler is the HTTP layer for tenant endpoints.
//
// Tenant creation is NOT exposed here — tenants are created exclusively
// through the onboarding completion flow. This handler exposes only reads
// and the slug-availability check.
type Handler struct {
	svc *Service
}

// NewHandler constructs a Handler.
func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

// Register mounts tenant routes onto the given gin.RouterGroup.
//
// Public routes:
//   - GET /slug-available?slug=...  used by onboarding wizard
//   - GET /tenants/by-owner?uid=... used by the returning-user sign-in
//                                   server action to map a GIP UID to its
//                                   workspace_tenant before calling auth-bff
//
// Internal routes (callable by other services / auth-bff):
//   - GET /internal/tenants/:id     used by auth-bff to look up a tenant
//                                   when minting a session post-auto-login
func (h *Handler) Register(public *gin.RouterGroup, internal *gin.RouterGroup) {
	t := public.Group("/tenants")
	{
		t.GET("/slug-available", h.checkSlugAvailable)
		t.GET("/by-owner", h.getTenantByOwner)
	}

	int := internal.Group("/tenants")
	{
		int.GET("/:id", h.getTenant)
	}
}

func (h *Handler) checkSlugAvailable(c *gin.Context) {
	slug := c.Query("slug")
	available, err := h.svc.IsSlugAvailable(c.Request.Context(), slug)
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"data": gin.H{
			"slug":      slug,
			"available": available,
		},
	})
}

// getTenantByOwner serves GET /tenants/by-owner?uid=...
//
// Looks up the workspace tenant owned by the given GIP UID. Used by the
// onboarding app's sign-in server action to bridge a freshly minted GIP
// id_token to a concrete workspace_tenant before calling auth-bff
// /auth/auto-login. Returns 404 if the UID doesn't own a tenant — the
// caller surfaces this as "no store found for this account".
func (h *Handler) getTenantByOwner(c *gin.Context) {
	uid := c.Query("uid")
	t, err := h.svc.GetByOwnerUserID(c.Request.Context(), uid)
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": t})
}

func (h *Handler) getTenant(c *gin.Context) {
	t, err := h.svc.GetByID(c.Request.Context(), c.Param("id"))
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": t})
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
