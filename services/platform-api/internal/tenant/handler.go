package tenant

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

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
//     server action to map a GIP UID to its
//     workspace_tenant before calling auth-bff
//
// Internal routes (callable by other services / auth-bff):
//   - GET /internal/tenants/:id     used by auth-bff to look up a tenant
//     when minting a session post-auto-login
func (h *Handler) Register(public *gin.RouterGroup, internal *gin.RouterGroup) {
	t := public.Group("/tenants")
	{
		// Phase Q: slug-available moved to the store package
		// (store.Handler.checkSlugAvailable); a tenant doesn't
		// own a slug anymore.
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

// RegisterDirectory mounts the platform-wide tenant directory (#277) on the
// supplied group. The CALLER is responsible for gating it — main.go wraps it
// in middleware.RequireInternalAuthStrict, because these routes return every
// tenant on the platform and must not be reachable on an unconfigured deploy.
//
// Deliberately separate from Register: those routes are caller-scoped or
// id-scoped and keep the permissive auth branch.
func (h *Handler) RegisterDirectory(g *gin.RouterGroup) {
	t := g.Group("/tenants")
	{
		t.GET("", h.listDirectory)
		// /detail, not /:id — the internal group already serves GET
		// /tenants/:id with a different shape, and the admin BFF calls it.
		t.GET("/:id/detail", h.getTenantDetail)
		// Static sibling of :id. Verified against gin 1.12 with main.go's
		// real two-group shape: no router-build panic, and /tenants/:id,
		// /:id/detail and /:id/members all still resolve to their own
		// handlers. Path chosen over a filter on the directory list because
		// that list matches with ILIKE '%q%' — a substring match would
		// report the wrong lead converted.
		t.GET("/by-owner-email", h.getTenantByOwnerEmail)
	}
}

// getTenantByOwnerEmail serves GET /internal/tenants/by-owner-email?email=...
// (#279). Used by the marketplace-api console lookup that maps a lead's
// email to the tenant it converted into. A missing or unowned email comes
// back 404 — the repository already normalises (trim + lowercase) and
// reports apperrors.NotFound, which respondError maps here. The 400 the
// console contract expects for a missing email belongs to marketplace-api,
// which knows it's serving a console caller; this hop only knows "found"
// or "not found".
func (h *Handler) getTenantByOwnerEmail(c *gin.Context) {
	t, err := h.svc.GetByOwnerEmail(c.Request.Context(), c.Query("email"))
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": t})
}

// RegisterLifecycle mounts the operator-facing tenant suspend/unsuspend
// routes (#287) on the supplied group. Mounted on strictInternal only:
// these act on the whole estate and are not scoped by anything the caller
// had to know, so an unconfigured deploy must answer 503 rather than serve
// them. Deliberately separate from the existing PATCH in Register — that
// path runs a merchant-authorized fga.Check a platform operator has no
// relation in, and carries no status field.
func (h *Handler) RegisterLifecycle(g *gin.RouterGroup) {
	t := g.Group("/tenants")
	{
		t.POST("/:id/suspend", h.suspendTenant)
		t.POST("/:id/unsuspend", h.unsuspendTenant)
	}
}

// suspendTenant serves POST /internal/tenants/:id/suspend (#287).
func (h *Handler) suspendTenant(c *gin.Context) {
	h.lifecycle(c, h.svc.Suspend)
}

// unsuspendTenant serves POST /internal/tenants/:id/unsuspend (#287).
func (h *Handler) unsuspendTenant(c *gin.Context) {
	h.lifecycle(c, h.svc.Unsuspend)
}

// lifecycle is shared by suspendTenant and unsuspendTenant: identical
// response shaping and error mapping, one implementation so the two
// routes cannot drift. A no-op (Changed=false) is still a 200 — #287's
// acceptance requires that suspending an already-suspended tenant
// returns current state, not an error.
func (h *Handler) lifecycle(c *gin.Context, op func(context.Context, string) (*SuspendResult, error)) {
	id := c.Param("id")
	// A malformed id is a caller error, not a server error. Unvalidated it
	// reaches Postgres, which raises 22P02 and surfaces as a 500 that pages
	// someone (#343). marketplace-api's console-facing half already rejects
	// it with the same code, so the two halves of one operation agree.
	if _, err := uuid.Parse(id); err != nil {
		respondError(c, apperrors.BadRequest("invalid_tenant_id", "id must be a UUID"))
		return
	}
	res, err := op(c.Request.Context(), id)
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": gin.H{
		"tenant_id":       id,
		"status":          res.Status,
		"stores_affected": res.StoresAffected,
		"changed":         res.Changed,
	}})
}

func (h *Handler) listDirectory(c *gin.Context) {
	f := parseDirectoryFilter(c)

	res, err := h.svc.ListDirectory(c.Request.Context(), f)
	if err != nil {
		respondError(c, err)
		return
	}
	if res.Tenants == nil {
		res.Tenants = []Tenant{}
	}

	c.JSON(http.StatusOK, gin.H{
		"data": res.Tenants,
		"pagination": gin.H{
			"page":  max(f.Page, 1),
			"limit": f.Limit,
			"total": res.Total,
		},
	})
}

func (h *Handler) getTenantDetail(c *gin.Context) {
	t, err := h.svc.GetWithStores(c.Request.Context(), c.Param("id"))
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": t})
}

// parseDirectoryFilter never returns an error. A missing or malformed
// parameter takes the default; an oversized limit clamps. The console is
// entitled to ask for too much, and a ceiling here is the backstop.
func parseDirectoryFilter(c *gin.Context) DirectoryFilter {
	f := DirectoryFilter{
		Q:      strings.TrimSpace(c.Query("q")),
		Status: strings.TrimSpace(c.Query("status")),
		Page:   1,
		Limit:  DefaultDirectoryPageSize,
	}
	if v := strings.TrimSpace(c.Query("page")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			f.Page = n
		}
	}
	if v := strings.TrimSpace(c.Query("limit")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			f.Limit = min(n, MaxDirectoryPageSize)
		}
	}
	if t, ok := parseRFC3339(c.Query("created_from")); ok {
		f.CreatedFrom = t
	}
	if t, ok := parseRFC3339(c.Query("created_to")); ok {
		f.CreatedTo = t
	}
	if v := strings.TrimSpace(c.Query("ids")); v != "" {
		ids := make([]string, 0, strings.Count(v, ",")+1)
		for _, part := range strings.Split(v, ",") {
			part = strings.TrimSpace(part)
			if part != "" {
				ids = append(ids, part)
			}
		}
		// If nothing remains after trimming, treat "ids" as absent — an
		// empty slice must never reach DirectoryFilter.IDs (see the
		// len() guard in applyDirectoryFilter).
		if len(ids) > 0 {
			f.IDs = ids
		}
	}
	return f
}

func parseRFC3339(v string) (time.Time, bool) {
	v = strings.TrimSpace(v)
	if v == "" {
		return time.Time{}, false
	}
	t, err := time.Parse(time.RFC3339, v)
	if err != nil {
		return time.Time{}, false
	}
	return t, true
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
