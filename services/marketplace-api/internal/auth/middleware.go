// Package auth holds marketplace-api's request authentication middleware.
//
// Marketplace-api trusts X-User-Id and X-Tenant-Id headers because Istio's
// upstream AuthorizationPolicy verifies the JWT and forwards the claims.
// In production the headers are rewritten by Istio's request authentication
// filter and cannot be set by external callers. In dev (where Istio is not
// in front of the binary), tests pass the headers directly. As a defense-
// in-depth measure, MARKETPLACE_INTERNAL_AUTH_SECRET (when set) requires a
// matching X-Internal-Auth header — accidental direct exposure (e.g.,
// port-forwarding around Istio) is rejected.
package auth

import (
	"crypto/sha256"
	"crypto/subtle"
	"net/http"

	"github.com/gin-gonic/gin"
)

// HeaderTrustAuth returns a gin middleware that reads the user/tenant
// claim headers populated upstream by Istio. internalSecret, when
// non-empty, requires X-Internal-Auth to match.
// InternalSecretAuth guards a service-to-service route with the shared
// internal secret ALONE — no X-User-Id / X-Tenant-Id, which HeaderTrustAuth
// additionally requires.
//
// That distinction is the reason this exists. HeaderTrustAuth is for internal
// calls made ON BEHALF OF a user (the CSM review route). A platform-api
// callback fired during onboarding has no user: the merchant has not signed in
// anywhere yet. Reusing HeaderTrustAuth there would 401 every legitimate call
// and read as a broken secret.
//
// An EMPTY secret disables the check, matching HeaderTrustAuth and every other
// /internal route in this service. That is deliberate rather than lax: a route
// that fails closed while the ones beside it stay open turns a missing
// deployment variable into a partial outage that looks like a bug in whichever
// feature happens to use the strict route. The /internal namespace's real
// boundary is the network policy; this header is defence in depth.
func InternalSecretAuth(internalSecret string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if internalSecret != "" && !constantTimeEqual(c.GetHeader("X-Internal-Auth"), internalSecret) {
			respondUnauthorized(c)
			return
		}
		c.Next()
	}
}

func HeaderTrustAuth(internalSecret string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if internalSecret != "" && !constantTimeEqual(c.GetHeader("X-Internal-Auth"), internalSecret) {
			respondUnauthorized(c)
			return
		}
		userID := c.GetHeader("X-User-Id")
		tenantID := c.GetHeader("X-Tenant-Id")
		if userID == "" || tenantID == "" {
			respondUnauthorized(c)
			return
		}
		c.Set("user_id", userID)
		c.Set("tenant_id", tenantID)
		// Optional. When present, audit-log emitter records it as the
		// actor; when absent, audit rows store NULL for actor_email and
		// the UI shows the user_id instead.
		if email := c.GetHeader("X-User-Email"); email != "" {
			c.Set("user_email", email)
		}
		c.Next()
	}
}

func constantTimeEqual(got, want string) bool {
	if got == "" || want == "" {
		return false
	}
	gotSum := sha256.Sum256([]byte(got))
	wantSum := sha256.Sum256([]byte(want))
	return subtle.ConstantTimeCompare(gotSum[:], wantSum[:]) == 1
}

func respondUnauthorized(c *gin.Context) {
	c.AbortWithStatusJSON(http.StatusUnauthorized, map[string]any{
		"error":   "unauthorized",
		"message": "authentication required",
	})
}
