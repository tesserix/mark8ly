package admin

import (
	"github.com/gin-gonic/gin"

	"github.com/mark8ly/marketplace-api/internal/authz"
	"github.com/mark8ly/marketplace-api/internal/billing/appaddon"
)

// MountAppCredentialsRoutes mounts the two white-label credential
// upload endpoints onto an already-authenticated store-scoped route
// group (`/admin/stores/:storeId`). Called from main.go alongside the
// existing admin route registration — kept as a separate function so
// the new wiring doesn't require extending the Deps struct that other
// sessions may be modifying concurrently.
//
// Tenant scoping in appcreds.Service is not authorisation: without the role
// gate any member, viewer included, could upload signing keys.
func MountAppCredentialsRoutes(storeRoute *gin.RouterGroup, h *AppCredentialsHandler, fgaMw *authz.Middleware) {
	if h == nil {
		return
	}
	g := storeRoute.Group("/app-credentials", fgaMw.RequireTenantRelation(authz.AppCredentialsEditRole))
	g.POST("/apple", h.PostApple)
	g.POST("/google", h.PostGoogle)
}

// MountAppAddOnRoutes mounts the add-on purchase endpoint onto the
// already-authenticated store-scoped route group.
func MountAppAddOnRoutes(storeRoute *gin.RouterGroup, h *appaddon.Handler, fgaMw *authz.Middleware) {
	if h == nil {
		return
	}
	storeRoute.POST("/subscription/add-on/white-label-app",
		fgaMw.RequireTenantRelation(authz.SubscriptionEditRole), h.Purchase)
}
