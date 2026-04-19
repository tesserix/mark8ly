// Package public implements unauthenticated / pre-auth public endpoints.
// Currently this covers the per-tenant SSO login/callback/logout flows.
// Routes are registered by RegisterPublic onto a caller-supplied
// *gin.RouterGroup — no auth middleware is applied at this layer; individual
// handlers validate identity through the SSO protocol itself.
package public

import (
	"github.com/gin-gonic/gin"
)

// PublicDeps groups every dependency the public route registrar needs.
// Constructed in cmd/marketplace-api/main.go.
type PublicDeps struct {
	// SSOLoginHandler handles /sso/:tenantSlug/{login,callback,logout}.
	// May be nil when SSO feature is not wired (e.g. test builds).
	SSOLoginHandler *SSOLoginHandler
}

// RegisterPublic mounts all public (unauthenticated) routes onto the given
// router group. Callers typically pass the root group so routes are served
// at the top level without an /api/v1 prefix.
func RegisterPublic(router *gin.RouterGroup, deps PublicDeps) {
	if deps.SSOLoginHandler != nil {
		sso := router.Group("/sso/:tenantSlug")
		sso.GET("/login", deps.SSOLoginHandler.Login)
		sso.POST("/callback", deps.SSOLoginHandler.Callback)
		sso.POST("/logout", deps.SSOLoginHandler.Logout)
	}
}
