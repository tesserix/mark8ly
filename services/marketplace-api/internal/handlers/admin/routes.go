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
	ProductHandler        *ProductHandler
	CategoryHandler       *CategoryHandler
	VariantHandler        *VariantHandler
	MediaHandler          *MediaHandler
	OrdersHandler         *OrdersHandler
	ReturnsHandler        *ReturnsHandler
	AbandonedCartsHandler *AbandonedCartsHandler
	StoresHandler            *StoresHandler
	BulkHandler              *BulkHandler
	CSVImportsHandler        *CSVImportsHandler
	PaymentSettingsHandler   *PaymentSettingsHandler
	ShippingSettingsHandler  *ShippingSettingsHandler
	TaxSettingsHandler       *TaxSettingsHandler
	CouponHandler            *CouponHandler
	GiftCardHandler          *GiftCardHandler
	LoyaltyHandler           *LoyaltyHandler
	CampaignHandler          *CampaignHandler
	SegmentHandler           *SegmentHandler
	StoresMiddleware         gin.HandlerFunc // from stores.StoreMiddleware
	AuthzMiddleware          *authz.Middleware
	InternalSecret           string
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

	// Tenant-wide admin routes — outside of /stores/:storeId because they
	// enumerate across stores, not within a single one.
	if deps.StoresHandler != nil {
		adminRoot := router.Group("/admin", authMW)
		adminRoot.GET("/stores",
			deps.AuthzMiddleware.RequireTenantRelation(authz.RoleStaff),
			deps.StoresHandler.List)
	}

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

			// Bulk actions — role check is per-id inside the handler.
			if deps.BulkHandler != nil {
				products.POST("/bulk", deps.AuthzMiddleware.RequireTenantRelation(authz.RoleStaff), deps.BulkHandler.Bulk)
			}

			// CSV export — streaming download of all products as CSV.
			if deps.CSVImportsHandler != nil {
				products.GET("/export.csv",
					deps.AuthzMiddleware.RequireTenantRelation(authz.CSVExportRole),
					deps.CSVImportsHandler.Export)
			}
		}

		// CSV imports — submit, list, status, cancel, download errors.
		if deps.CSVImportsHandler != nil {
			csvImports := storeRoute.Group("/csv-imports")
			{
				csvImports.POST("",
					deps.AuthzMiddleware.RequireTenantRelation(authz.CSVImportRole),
					deps.CSVImportsHandler.Submit)
				csvImports.GET("",
					deps.AuthzMiddleware.RequireTenantRelation(authz.CSVImportViewRole),
					deps.CSVImportsHandler.List)
				csvImports.GET("/:id",
					deps.AuthzMiddleware.RequireTenantRelation(authz.CSVImportViewRole),
					deps.CSVImportsHandler.Status)
				csvImports.POST("/:id/cancel",
					deps.AuthzMiddleware.RequireTenantRelation(authz.CSVImportRole),
					deps.CSVImportsHandler.Cancel)
				csvImports.GET("/:id/errors",
					deps.AuthzMiddleware.RequireTenantRelation(authz.CSVImportViewRole),
					deps.CSVImportsHandler.DownloadErrors)
			}
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

		// Orders — role policy from internal/authz/orders_roles.go.
		if deps.OrdersHandler != nil {
			orders := storeRoute.Group("/orders")
			{
				orders.GET("",
					deps.AuthzMiddleware.RequireTenantRelation(authz.OrdersViewRole),
					deps.OrdersHandler.List)
				orders.POST("",
					deps.AuthzMiddleware.RequireTenantRelation(authz.OrdersEditRole),
					deps.OrdersHandler.Create)
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

		// Returns — Request lives under /orders/:id/returns (same :id
		// parameter name as the orders subtree to satisfy gin's per-level
		// param-name uniqueness rule). State-change actions live under
		// /returns/:id/<verb>.
		if deps.ReturnsHandler != nil {
			storeRoute.POST("/orders/:id/returns",
				deps.AuthzMiddleware.RequireTenantRelation(authz.ReturnsEditRole),
				deps.ReturnsHandler.Request)
			returns := storeRoute.Group("/returns")
			{
				returns.GET("",
					deps.AuthzMiddleware.RequireTenantRelation(authz.ReturnsViewRole),
					deps.ReturnsHandler.List)
				returns.GET("/:id",
					deps.AuthzMiddleware.RequireTenantRelation(authz.ReturnsViewRole),
					deps.ReturnsHandler.Get)
				returns.POST("/:id/approve",
					deps.AuthzMiddleware.RequireTenantRelation(authz.ReturnsEditRole),
					deps.ReturnsHandler.Approve)
				returns.POST("/:id/reject",
					deps.AuthzMiddleware.RequireTenantRelation(authz.ReturnsEditRole),
					deps.ReturnsHandler.Reject)
				returns.POST("/:id/received",
					deps.AuthzMiddleware.RequireTenantRelation(authz.ReturnsEditRole),
					deps.ReturnsHandler.MarkReceived)
				returns.POST("/:id/refunded",
					deps.AuthzMiddleware.RequireTenantRelation(authz.ReturnsEditRole),
					deps.ReturnsHandler.MarkRefunded)
			}
		}

		// Settings — payment, shipping, tax configuration.
		if deps.PaymentSettingsHandler != nil {
			ps := storeRoute.Group("/settings/payments")
			{
				ps.GET("",
					deps.AuthzMiddleware.RequireTenantRelation(authz.RoleAdmin),
					deps.PaymentSettingsHandler.List)
				ps.PUT("/:provider",
					deps.AuthzMiddleware.RequireTenantRelation(authz.RoleOwner),
					deps.PaymentSettingsHandler.Upsert)
				ps.DELETE("/:provider",
					deps.AuthzMiddleware.RequireTenantRelation(authz.RoleOwner),
					deps.PaymentSettingsHandler.Delete)
				ps.POST("/:provider/test",
					deps.AuthzMiddleware.RequireTenantRelation(authz.RoleAdmin),
					deps.PaymentSettingsHandler.TestConnection)
			}
		}
		if deps.ShippingSettingsHandler != nil {
			ss := storeRoute.Group("/settings/shipping")
			{
				ss.GET("",
					deps.AuthzMiddleware.RequireTenantRelation(authz.RoleAdmin),
					deps.ShippingSettingsHandler.List)
				ss.PUT("/:provider",
					deps.AuthzMiddleware.RequireTenantRelation(authz.RoleOwner),
					deps.ShippingSettingsHandler.Upsert)
				ss.DELETE("/:provider",
					deps.AuthzMiddleware.RequireTenantRelation(authz.RoleOwner),
					deps.ShippingSettingsHandler.Delete)
			}
		}
		if deps.TaxSettingsHandler != nil {
			ts := storeRoute.Group("/settings/tax")
			{
				ts.GET("",
					deps.AuthzMiddleware.RequireTenantRelation(authz.RoleAdmin),
					deps.TaxSettingsHandler.Get)
				ts.PUT("/taxjar",
					deps.AuthzMiddleware.RequireTenantRelation(authz.RoleOwner),
					deps.TaxSettingsHandler.UpsertTaxJar)
			}
		}

		// Coupons — Marketing M1.
		if deps.CouponHandler != nil {
			coupons := storeRoute.Group("/coupons")
			{
				coupons.GET("",
					deps.AuthzMiddleware.RequireTenantRelation(authz.RoleStaff),
					deps.CouponHandler.List)
				coupons.POST("",
					deps.AuthzMiddleware.RequireTenantRelation(authz.RoleAdmin),
					deps.CouponHandler.Create)
				coupons.GET("/:id",
					deps.AuthzMiddleware.RequireTenantRelation(authz.RoleStaff),
					deps.CouponHandler.Get)
				coupons.PATCH("/:id",
					deps.AuthzMiddleware.RequireTenantRelation(authz.RoleAdmin),
					deps.CouponHandler.Patch)
				coupons.DELETE("/:id",
					deps.AuthzMiddleware.RequireTenantRelation(authz.RoleAdmin),
					deps.CouponHandler.Delete)
			}
		}

		// Gift cards — Marketing M2.
		if deps.GiftCardHandler != nil {
			gc := storeRoute.Group("/gift-cards")
			{
				gc.GET("",
					deps.AuthzMiddleware.RequireTenantRelation(authz.GiftCardsViewRole),
					deps.GiftCardHandler.List)
				gc.POST("",
					deps.AuthzMiddleware.RequireTenantRelation(authz.GiftCardsEditRole),
					deps.GiftCardHandler.Issue)
				gc.GET("/:id",
					deps.AuthzMiddleware.RequireTenantRelation(authz.GiftCardsViewRole),
					deps.GiftCardHandler.Get)
			}
		}

		// Loyalty program — Marketing M3.
		if deps.LoyaltyHandler != nil {
			loyaltyGroup := storeRoute.Group("/loyalty")
			{
				loyaltyGroup.GET("/program",
					deps.AuthzMiddleware.RequireTenantRelation(authz.LoyaltyViewRole),
					deps.LoyaltyHandler.GetProgram)
				loyaltyGroup.PUT("/program",
					deps.AuthzMiddleware.RequireTenantRelation(authz.LoyaltyEditRole),
					deps.LoyaltyHandler.UpdateProgram)
				members := loyaltyGroup.Group("/members")
				{
					members.GET("",
						deps.AuthzMiddleware.RequireTenantRelation(authz.LoyaltyViewRole),
						deps.LoyaltyHandler.ListMembers)
					members.GET("/:id",
						deps.AuthzMiddleware.RequireTenantRelation(authz.LoyaltyViewRole),
						deps.LoyaltyHandler.GetMember)
					members.POST("/:id/adjust",
						deps.AuthzMiddleware.RequireTenantRelation(authz.LoyaltyEditRole),
						deps.LoyaltyHandler.AdjustPoints)
				}
				loyaltyGroup.GET("/referrals",
					deps.AuthzMiddleware.RequireTenantRelation(authz.LoyaltyViewRole),
					deps.LoyaltyHandler.ListReferrals)
			}
		}

		// Abandoned carts.
		if deps.AbandonedCartsHandler != nil {
			carts := storeRoute.Group("/abandoned-carts")
			{
				carts.GET("",
					deps.AuthzMiddleware.RequireTenantRelation(authz.AbandonedCartsViewRole),
					deps.AbandonedCartsHandler.List)
				carts.GET("/:id",
					deps.AuthzMiddleware.RequireTenantRelation(authz.AbandonedCartsViewRole),
					deps.AbandonedCartsHandler.Get)
				carts.POST("/:id/recovery-email",
					deps.AuthzMiddleware.RequireTenantRelation(authz.AbandonedCartsEditRole),
					deps.AbandonedCartsHandler.TriggerRecoveryEmail)
			}
		}

		// Campaigns — Marketing M4.
		if deps.CampaignHandler != nil {
			campaigns := storeRoute.Group("/campaigns")
			{
				campaigns.GET("",
					deps.AuthzMiddleware.RequireTenantRelation(authz.CampaignsViewRole),
					deps.CampaignHandler.List)
				campaigns.POST("",
					deps.AuthzMiddleware.RequireTenantRelation(authz.CampaignsEditRole),
					deps.CampaignHandler.Create)
				campaigns.GET("/:id",
					deps.AuthzMiddleware.RequireTenantRelation(authz.CampaignsViewRole),
					deps.CampaignHandler.Get)
				campaigns.PATCH("/:id",
					deps.AuthzMiddleware.RequireTenantRelation(authz.CampaignsEditRole),
					deps.CampaignHandler.Patch)
				campaigns.DELETE("/:id",
					deps.AuthzMiddleware.RequireTenantRelation(authz.CampaignsEditRole),
					deps.CampaignHandler.Delete)
				campaigns.POST("/:id/send",
					deps.AuthzMiddleware.RequireTenantRelation(authz.CampaignsEditRole),
					deps.CampaignHandler.Send)
				campaigns.POST("/:id/schedule",
					deps.AuthzMiddleware.RequireTenantRelation(authz.CampaignsEditRole),
					deps.CampaignHandler.Schedule)
				campaigns.POST("/:id/pause",
					deps.AuthzMiddleware.RequireTenantRelation(authz.CampaignsEditRole),
					deps.CampaignHandler.Pause)
				campaigns.POST("/:id/resume",
					deps.AuthzMiddleware.RequireTenantRelation(authz.CampaignsEditRole),
					deps.CampaignHandler.Resume)
			}
		}

		// Segments — Marketing M4.
		if deps.SegmentHandler != nil {
			segments := storeRoute.Group("/segments")
			{
				segments.GET("",
					deps.AuthzMiddleware.RequireTenantRelation(authz.CampaignsViewRole),
					deps.SegmentHandler.List)
				segments.POST("",
					deps.AuthzMiddleware.RequireTenantRelation(authz.CampaignsEditRole),
					deps.SegmentHandler.Create)
				segments.DELETE("/:id",
					deps.AuthzMiddleware.RequireTenantRelation(authz.CampaignsEditRole),
					deps.SegmentHandler.Delete)
			}
		}
	}
}
