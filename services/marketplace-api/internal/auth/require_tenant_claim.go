package auth

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// RequireTenantClaim aborts with 404 unless the authenticated caller has a
// tenant bound to their identity.
//
// This is the companion to GIPBearerAuth's deliberate decision to NOT reject
// a token that lacks a tenant_id claim. That change fixed a login bounce
// loop (a 401 made the mobile client tear down a perfectly valid session and
// redirect to /login), but on its own it would fail OPEN: routes that read
// tenant_id without an authorization guard would suddenly be reachable by an
// authenticated user with no tenant at all, carrying an empty tenant scope.
//
// Most mobile admin routes are already covered by
// authz.RequireTenantRelation, which performs the real FGA check. This guard
// exists for the groups that are not — platform-support (not store-scoped)
// and push-tokens — and is mounted alongside bearerAuth at group level so
// that any route added later is protected by default rather than by
// remembering to add a check.
//
// 404 rather than 403 matches authz.RequireTenantRelation's existing
// convention of not leaking existence, and critically it is NOT 401 — the
// mobile client only signs the user out on 401, which is the behaviour this
// whole change set is removing.
func RequireTenantClaim() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.GetString("tenant_id") == "" {
			c.AbortWithStatusJSON(http.StatusNotFound, gin.H{
				"error":   "not_found",
				"message": "no store is linked to this account",
			})
			return
		}
		c.Next()
	}
}
