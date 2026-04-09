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
	"net/http"

	"github.com/gin-gonic/gin"
)

// HeaderTrustAuth returns a gin middleware that reads the user/tenant
// claim headers populated upstream by Istio. internalSecret, when
// non-empty, requires X-Internal-Auth to match.
func HeaderTrustAuth(internalSecret string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if internalSecret != "" && c.GetHeader("X-Internal-Auth") != internalSecret {
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
		c.Next()
	}
}

func respondUnauthorized(c *gin.Context) {
	c.AbortWithStatusJSON(http.StatusUnauthorized, map[string]any{
		"error":   "unauthorized",
		"message": "authentication required",
	})
}
