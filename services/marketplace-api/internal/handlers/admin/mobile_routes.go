package admin

import (
	"github.com/gin-gonic/gin"

	"github.com/mark8ly/marketplace-api/internal/auth"
	"github.com/mark8ly/marketplace-api/internal/authz"
)

// MobileDeps extends Deps with the mobile-specific GIP token verifier.
type MobileDeps struct {
	Deps
	TokenVerifier    auth.TokenVerifier
	PushTokenHandler *PushTokenHandler
	// PlatformSupportHandler bridges merchant→platform support chat to
	// otto's platform tenant. Nil when otto isn't wired.
	PlatformSupportHandler *PlatformSupportHandler
}

// RegisterAdminMobile mounts the mobile admin route group. Uses GIPBearerAuth
// instead of HeaderTrustAuth. Same handlers, same authz, different auth.
// Includes per-user rate limiting since these routes are public-internet-facing.
func RegisterAdminMobile(router *gin.RouterGroup, deps MobileDeps) {
	if deps.TokenVerifier == nil {
		return // mobile routes disabled when no GIP config
	}

	bearerAuth := auth.GIPBearerAuth(deps.TokenVerifier)
	rateLimiter := auth.NewPerUserRateLimiter(60, 10) // 60 req/min, burst 10

	// Platform support chat — merchant admin → Tesserix platform team.
	// Not store-scoped: it rides the admin's tenant from the bearer token,
	// so any authenticated merchant admin can open a platform chat.
	if deps.PlatformSupportHandler != nil {
		ps := router.Group("/mobile/admin/platform-support", bearerAuth, rateLimiter)
		deps.PlatformSupportHandler.Register(ps)
	}

	// Tenant-wide routes
	if deps.StoresHandler != nil {
		mobileRoot := router.Group("/mobile/admin", bearerAuth, rateLimiter)
		mobileRoot.GET("/stores",
			deps.AuthzMiddleware.RequireTenantRelation(authz.RoleStaff),
			deps.StoresHandler.List)
	}

	storeRoute := router.Group("/mobile/admin/stores/:storeId", bearerAuth, rateLimiter, deps.StoresMiddleware)
	{
		// Dashboard
		if deps.DashboardHandler != nil {
			storeRoute.GET("/dashboard",
				deps.AuthzMiddleware.RequireTenantRelation(authz.RoleStaff),
				deps.DashboardHandler.Get)
		}

		// Products (full CRUD)
		products := storeRoute.Group("/products")
		{
			products.GET("", deps.AuthzMiddleware.RequireTenantRelation(authz.RoleStaff), deps.ProductHandler.List)
			products.POST("", deps.AuthzMiddleware.RequireTenantRelation(authz.RoleAdmin), deps.ProductHandler.Create)
			products.GET("/:id", deps.AuthzMiddleware.RequireTenantRelation(authz.RoleStaff), deps.ProductHandler.Get)
			products.PATCH("/:id", deps.AuthzMiddleware.RequireTenantRelation(authz.RoleAdmin), deps.ProductHandler.Patch)
		}

		// Product media
		if deps.MediaHandler != nil {
			mediaGroup := products.Group("/:id/media")
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
			}
		}

		// Product variants
		if deps.VariantHandler != nil {
			storeRoute.PATCH("/products/:id/variants/:variantId",
				deps.AuthzMiddleware.RequireTenantRelation(authz.RoleAdmin),
				deps.VariantHandler.Patch)
		}

		// Product categories
		if deps.CategoryHandler != nil {
			cats := storeRoute.Group("/categories")
			{
				cats.GET("", deps.AuthzMiddleware.RequireTenantRelation(authz.RoleStaff), deps.CategoryHandler.List)
				// Create mirrors the web admin route (RoleAdmin) so merchants can
				// add a category from the mobile picker. Reuses the same handler,
				// DTO, and service as POST /api/v1/admin/stores/:storeId/categories.
				cats.POST("", deps.AuthzMiddleware.RequireTenantRelation(authz.RoleAdmin), deps.CategoryHandler.Create)
			}
		}

		// Orders (list, detail, actions)
		if deps.OrdersHandler != nil {
			orders := storeRoute.Group("/orders")
			{
				orders.GET("",
					deps.AuthzMiddleware.RequireTenantRelation(authz.OrdersViewRole),
					deps.OrdersHandler.List)
				orders.GET("/:id",
					deps.AuthzMiddleware.RequireTenantRelation(authz.OrdersViewRole),
					deps.OrdersHandler.Get)
				orders.POST("/:id/confirm",
					deps.AuthzMiddleware.RequireTenantRelation(authz.OrdersEditRole),
					deps.OrdersHandler.Confirm)
				orders.POST("/:id/fulfill",
					deps.AuthzMiddleware.RequireTenantRelation(authz.OrdersEditRole),
					deps.OrdersHandler.MarkFulfilled)
				orders.POST("/:id/cancel",
					deps.AuthzMiddleware.RequireTenantRelation(authz.OrdersEditRole),
					deps.OrdersHandler.Cancel)
				orders.POST("/:id/refund",
					deps.AuthzMiddleware.RequireTenantRelation(authz.OrdersRefundRole),
					deps.OrdersHandler.Refund)
			}
		}

		// Customers (list, detail, block/unblock)
		if deps.CustomersHandler != nil {
			customers := storeRoute.Group("/customers")
			{
				customers.GET("",
					deps.AuthzMiddleware.RequireTenantRelation(authz.CustomersViewRole),
					deps.CustomersHandler.List)
				customers.GET("/:id",
					deps.AuthzMiddleware.RequireTenantRelation(authz.CustomersViewRole),
					deps.CustomersHandler.Get)
				customers.POST("/:id/block",
					deps.AuthzMiddleware.RequireTenantRelation(authz.CustomersEditRole),
					deps.CustomersHandler.Block)
				customers.POST("/:id/unblock",
					deps.AuthzMiddleware.RequireTenantRelation(authz.CustomersEditRole),
					deps.CustomersHandler.Unblock)
			}
		}

		// Reviews (moderation) — mirrors web routes.go:590-611. Staff view,
		// Admin mutate. Handler/DTO/service are shared with web; this only
		// exposes the same routes on the mobile group (same split as categories).
		if deps.ReviewsHandler != nil {
			reviews := storeRoute.Group("/reviews")
			{
				reviews.GET("",
					deps.AuthzMiddleware.RequireTenantRelation(authz.ReviewsViewRole),
					deps.ReviewsHandler.List)
				reviews.GET("/:id",
					deps.AuthzMiddleware.RequireTenantRelation(authz.ReviewsViewRole),
					deps.ReviewsHandler.Get)
				reviews.POST("/:id/approve",
					deps.AuthzMiddleware.RequireTenantRelation(authz.ReviewsEditRole),
					deps.ReviewsHandler.Approve)
				reviews.POST("/:id/reject",
					deps.AuthzMiddleware.RequireTenantRelation(authz.ReviewsEditRole),
					deps.ReviewsHandler.Reject)
				reviews.POST("/:id/featured",
					deps.AuthzMiddleware.RequireTenantRelation(authz.ReviewsEditRole),
					deps.ReviewsHandler.ToggleFeatured)
				reviews.POST("/:id/reply",
					deps.AuthzMiddleware.RequireTenantRelation(authz.ReviewsEditRole),
					deps.ReviewsHandler.Reply)
			}
		}

		// Push tokens
		if deps.PushTokenHandler != nil {
			pushTokens := storeRoute.Group("/push-tokens")
			{
				pushTokens.POST("", deps.PushTokenHandler.Register)
				pushTokens.DELETE("/:tokenId", deps.PushTokenHandler.Delete)
			}
		}

		// Notifications
		if deps.NotificationsHandler != nil {
			notifs := storeRoute.Group("/notifications")
			{
				notifs.GET("",
					deps.AuthzMiddleware.RequireTenantRelation(authz.NotificationsViewRole),
					deps.NotificationsHandler.List)
				notifs.PATCH("/read-all",
					deps.AuthzMiddleware.RequireTenantRelation(authz.NotificationsEditRole),
					deps.NotificationsHandler.MarkAllRead)
			}
		}
	}
}
