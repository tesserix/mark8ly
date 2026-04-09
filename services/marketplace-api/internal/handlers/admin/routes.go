// Package admin — routes.go: RegisterAdmin wires the M5a products subset
// of the admin API onto a caller-supplied *gin.RouterGroup. The caller
// owns the API version prefix (typically "/api/v1") so this file stays
// oblivious to versioning decisions.
package admin

import (
	"github.com/gin-gonic/gin"

	"github.com/mark8ly/marketplace-api/internal/auth"
	"github.com/mark8ly/marketplace-api/internal/authz"
)

// Deps groups every dependency the admin route registrar needs.
// Constructed in cmd/marketplace-api/main.go.
type Deps struct {
	ProductHandler   *ProductHandler
	CategoryHandler  *CategoryHandler
	VariantHandler   *VariantHandler
	MediaHandler     *MediaHandler
	StoresMiddleware gin.HandlerFunc // from stores.StoreMiddleware
	AuthzMiddleware  *authz.Middleware
	InternalSecret   string
}

// RegisterAdmin mounts the admin route group on the given router. The
// group is rooted at /admin/stores/:storeId and the middleware chain is:
//
//	HeaderTrustAuth → StoreMiddleware → RequireTenantRelation
//
// The auth middleware sets user_id + tenant_id on the gin context from
// internal-trust headers; the store middleware validates store ownership
// against the projection and populates "store" on the context;
// RequireTenantRelation runs the FGA Check per spec §13.1.1.
func RegisterAdmin(router *gin.RouterGroup, deps Deps) {
	authMW := auth.HeaderTrustAuth(deps.InternalSecret)

	storeRoute := router.Group("/admin/stores/:storeId", authMW, deps.StoresMiddleware)
	{
		products := storeRoute.Group("/products")
		{
			products.GET("", deps.AuthzMiddleware.RequireTenantRelation(authz.RoleStaff), deps.ProductHandler.List)
			products.POST("", deps.AuthzMiddleware.RequireTenantRelation(authz.RoleAdmin), deps.ProductHandler.Create)
			products.GET("/:id", deps.AuthzMiddleware.RequireTenantRelation(authz.RoleStaff), deps.ProductHandler.Get)
			products.PATCH("/:id", deps.AuthzMiddleware.RequireTenantRelation(authz.RoleAdmin), deps.ProductHandler.Patch)
			products.DELETE("/:id", deps.AuthzMiddleware.RequireTenantRelation(authz.RoleOwner), deps.ProductHandler.Delete)
			products.POST("/:id/copy", deps.AuthzMiddleware.RequireTenantRelation(authz.RoleAdmin), deps.ProductHandler.Copy)
		}
		categories := storeRoute.Group("/categories")
		{
			categories.GET("", deps.AuthzMiddleware.RequireTenantRelation(authz.RoleStaff), deps.CategoryHandler.List)
			categories.POST("", deps.AuthzMiddleware.RequireTenantRelation(authz.RoleAdmin), deps.CategoryHandler.Create)
			categories.PATCH("/:id", deps.AuthzMiddleware.RequireTenantRelation(authz.RoleAdmin), deps.CategoryHandler.Patch)
			categories.DELETE("/:id", deps.AuthzMiddleware.RequireTenantRelation(authz.RoleAdmin), deps.CategoryHandler.Delete)
		}

		storeRoute.PATCH("/products/:id/variants/:variantId",
			deps.AuthzMiddleware.RequireTenantRelation(authz.RoleAdmin),
			deps.VariantHandler.Patch)

		mediaGroup := storeRoute.Group("/products/:id/media")
		{
			mediaGroup.POST("/upload-url",
				deps.AuthzMiddleware.RequireTenantRelation(authz.RoleAdmin),
				deps.MediaHandler.UploadURL)
			mediaGroup.POST("",
				deps.AuthzMiddleware.RequireTenantRelation(authz.RoleAdmin),
				deps.MediaHandler.Create)
			mediaGroup.PATCH("/:mediaId",
				deps.AuthzMiddleware.RequireTenantRelation(authz.RoleAdmin),
				deps.MediaHandler.Patch)
			mediaGroup.DELETE("/:mediaId",
				deps.AuthzMiddleware.RequireTenantRelation(authz.RoleAdmin),
				deps.MediaHandler.Delete)
			mediaGroup.POST("/:mediaId/recrop",
				deps.AuthzMiddleware.RequireTenantRelation(authz.RoleAdmin),
				deps.MediaHandler.Recrop)
		}
	}
}
