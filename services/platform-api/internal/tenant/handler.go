package tenant

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/mark8ly/platform-api/internal/authz"
	apperrors "github.com/mark8ly/platform-api/pkg/errors"
)

// Handler is the HTTP layer for tenant endpoints.
//
// Tenant creation is NOT exposed here — tenants are created exclusively
// through the onboarding completion flow. This handler exposes reads,
// the slug-availability check, role-aware updates, and a "what role does
// the current user have on this tenant" lookup the admin BFF uses to
// gate its UI.
type Handler struct {
	svc *Service
	fga authz.Client // may be nil in dev if OpenFGA failed to initialise
}

// NewHandler constructs a Handler.
//
// fga may be nil — the startup path logs a warning and continues when
// OpenFGA isn't reachable so dev iteration isn't blocked. All code
// paths that touch fga must nil-check before use; see updateTenant and
// getMe below.
func NewHandler(svc *Service, fga authz.Client) *Handler {
	return &Handler{svc: svc, fga: fga}
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

	// Phase P multi-tenant membership. Lives under /api/v1/users
	// rather than /tenants because the resource is "tenants this
	// user belongs to", not "a tenant". The admin BFF calls this
	// with the uid from the session cookie.
	u := public.Group("/users")
	{
		u.GET("/me/tenants", h.listMyTenants)
	}

	int := internal.Group("/tenants")
	{
		int.GET("/:id", h.getTenant)
		int.PATCH("/:id", h.updateTenant)
		// Phase O: admin BFF calls this once per request to discover
		// the caller's role on the workspace tenant. uid comes from
		// the session cookie validated by admin middleware — the
		// admin BFF never trusts browser-supplied uids.
		int.GET("/:id/me", h.getMe)
	}
}

// updateTenantRequest is the body shape for PATCH /internal/tenants/:id.
//
// All fields are optional pointers so callers can PATCH a single field
// without needing to resend the full tenant row. Matches tenant.UpdateInput.
// The uid field carries the caller's GIP UID (from the admin BFF's
// validated session cookie) so platform-api can run an FGA check
// without standing up its own auth middleware.
type updateTenantRequest struct {
	Name *string `json:"name"`
	UID  string  `json:"uid"`
}

// updateTenant handles PATCH /internal/tenants/:id.
//
// Authorization flow (Phase O):
//
//  1. Admin BFF validates the session cookie and extracts the GIP UID.
//  2. Admin BFF POSTs the PATCH with {uid, name, ...} in the body.
//  3. Handler runs fga.Check(uid, "can_edit_settings", tenantID).
//  4. If denied, returns 403; if allowed, delegates to svc.Update.
//
// The uid is in the body rather than a header because Gin/middleware
// gets confusing with trailing-slash/empty-string edge cases on
// internal routes. A body field is explicit and easy to unit test.
//
// If the fga client is nil (dev without OpenFGA), the check is
// SKIPPED with a warning — we do not fail closed because that would
// block local settings edits whenever fga-seed is flaky. Never deploy
// to prod with fga nil.
func (h *Handler) updateTenant(c *gin.Context) {
	var req updateTenantRequest
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
		allowed, err := h.fga.Check(c.Request.Context(), req.UID, "can_edit_settings", tenantID)
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
	t, err := h.svc.Update(c.Request.Context(), tenantID, UpdateInput{Name: req.Name})
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": t})
}

// getMe handles GET /internal/tenants/:id/me?uid=...
//
// Returns the caller's role on the tenant, or 404 if they have no
// role at all. The admin BFF calls this once per authenticated
// request to forward the role into server components via the same
// `x-session-*` header convention Phase J uses for user/tenant.
//
// Response shape: { data: { role: "owner"|"admin"|"staff"|"viewer" } }
// 404 body: { error: "no_role", message: ... }
//
// If fga is nil (dev without OpenFGA), we fall back to returning
// role=owner. That keeps the admin UI usable during local iteration
// — the tenant-scoped PATCH check above also skips in the same
// condition, so the degraded behaviour is consistent.
func (h *Handler) getMe(c *gin.Context) {
	uid := c.Query("uid")
	if uid == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "missing_uid",
			"message": "uid query parameter is required",
		})
		return
	}
	tenantID := c.Param("id")
	if h.fga == nil {
		c.JSON(http.StatusOK, gin.H{"data": gin.H{"role": string(authz.RoleOwner)}})
		return
	}
	role, err := h.fga.GetRole(c.Request.Context(), uid, tenantID)
	if err != nil {
		respondError(c, apperrors.Internal("authz_check_failed", "authorization check failed"))
		return
	}
	if role == "" {
		c.JSON(http.StatusNotFound, gin.H{
			"error":   "no_role",
			"message": "user has no role on this tenant",
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": gin.H{"role": string(role)}})
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

// listMyTenants serves GET /api/v1/users/me/tenants?uid=...
//
// Phase P: returns every tenant the user has any role on. Used by the
// admin tenant switcher and the multi-tenant sign-in flow. Public
// route — the admin BFF is the only caller today and forwards the
// uid from the validated session cookie.
//
// Empty uid returns 400. Empty result set returns an empty list, not
// a 404 — callers differentiate "no such user" from "user with zero
// tenants" by looking at the `data` array length.
func (h *Handler) listMyTenants(c *gin.Context) {
	uid := c.Query("uid")
	memberships, err := h.svc.ListMemberTenants(c.Request.Context(), uid)
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": memberships})
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
