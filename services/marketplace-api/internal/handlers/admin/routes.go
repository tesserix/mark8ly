// Package admin — routes.go: RegisterAdmin wires the M5a products subset
// of the admin API onto a caller-supplied *gin.RouterGroup. The caller
// owns the API version prefix (typically "/api/v1") so this file stays
// oblivious to versioning decisions.
package admin

import (
	"github.com/gin-gonic/gin"

	"github.com/mark8ly/marketplace-api/internal/auth"
	"github.com/mark8ly/marketplace-api/internal/authz"
	"github.com/mark8ly/marketplace-api/internal/billing/migration"
	"github.com/mark8ly/marketplace-api/internal/plangate"
	"github.com/mark8ly/marketplace-api/internal/subscription/cancel"
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
	SettingsMetaHandler      *SettingsMetaHandler
	CouponHandler            *CouponHandler
	GiftCardHandler          *GiftCardHandler
	LoyaltyHandler           *LoyaltyHandler
	CampaignHandler          *CampaignHandler
	SegmentHandler           *SegmentHandler
	CustomersHandler         *CustomersHandler
	ReviewsHandler           *ReviewsHandler
	AccountHandler           *AccountHandler
	DomainsHandler           *DomainsHandler
	SubscriptionHandler      *SubscriptionHandler
	ChangePlanHandler        *ChangePlanHandler
	CompleteActionHandler    *CompleteActionHandler
	TrialBillingHandler      *TrialBillingHandler
	// P10 — promo-code engine (§7) + 14-day cooling-off refund (§8).
	PromoHandler  *PromoHandler
	RefundHandler *RefundHandler
	// P11 — merchant-initiated cancellation + save-offer (§15).
	CancelHandler *cancel.Handler
	MigrationFastPathHandler   *migration.Handler
	AuditLogsHandler         *AuditLogsHandler
	NotificationsHandler     *NotificationsHandler
	DashboardHandler         *DashboardHandler
	TicketsHandler           *TicketsHandler
	ShipmentsHandler         *ShipmentsHandler
	BrandingHandler          *BrandingHandler
	PagesHandler             *PagesHandler
	PlanResolver             *plangate.PlanResolver
	StoresMiddleware         gin.HandlerFunc // from stores.StoreMiddleware
	SubscriptionStatusLoader gin.HandlerFunc // optional; runs after StoresMiddleware
	SubscriptionReadOnlyGate gin.HandlerFunc // optional; runs after StatusLoader — returns 402 on read-only states
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

	// Account — S1. Lives outside /stores/:storeId because it's user-scoped.
	if deps.AccountHandler != nil {
		account := router.Group("/admin/account", authMW)
		{
			account.GET("",
				deps.AuthzMiddleware.RequireTenantRelation(authz.AccountViewRole),
				deps.AccountHandler.GetProfile)
			account.PATCH("",
				deps.AuthzMiddleware.RequireTenantRelation(authz.AccountEditRole),
				deps.AccountHandler.UpdateProfile)
			account.DELETE("",
				deps.AuthzMiddleware.RequireTenantRelation(authz.AccountDeleteRole),
				deps.AccountHandler.DeleteAccount)
			account.POST("/avatar/upload-url",
				deps.AuthzMiddleware.RequireTenantRelation(authz.AccountEditRole),
				deps.AccountHandler.AvatarUploadURL)
			account.POST("/mfa/enable",
				deps.AuthzMiddleware.RequireTenantRelation(authz.AccountEditRole),
				deps.AccountHandler.EnableMFA)
			account.POST("/mfa/verify",
				deps.AuthzMiddleware.RequireTenantRelation(authz.AccountEditRole),
				deps.AccountHandler.VerifyMFA)
			account.POST("/mfa/disable",
				deps.AuthzMiddleware.RequireTenantRelation(authz.AccountEditRole),
				deps.AccountHandler.DisableMFA)
			account.GET("/sessions",
				deps.AuthzMiddleware.RequireTenantRelation(authz.AccountViewRole),
				deps.AccountHandler.ListSessions)
			account.DELETE("/sessions/:id",
				deps.AuthzMiddleware.RequireTenantRelation(authz.AccountEditRole),
				deps.AccountHandler.RevokeSession)
		}
	}

	storeMW := []gin.HandlerFunc{authMW, deps.StoresMiddleware}
	if deps.SubscriptionStatusLoader != nil {
		storeMW = append(storeMW, deps.SubscriptionStatusLoader)
	}
	if deps.SubscriptionReadOnlyGate != nil {
		storeMW = append(storeMW, deps.SubscriptionReadOnlyGate)
	}
	storeRoute := router.Group("/admin/stores/:storeId", storeMW...)
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

				// Customer-facing document emails. Auto-fire on Confirm
				// (invoice) and on shipment delivery (receipt); these
				// endpoints let the merchant resend on demand.
				orders.POST("/:id/invoice/email",
					deps.AuthzMiddleware.RequireTenantRelation(authz.OrdersEditRole),
					deps.OrdersHandler.EmailInvoice)
				orders.POST("/:id/receipt/email",
					deps.AuthzMiddleware.RequireTenantRelation(authz.OrdersEditRole),
					deps.OrdersHandler.EmailReceipt)

				// Shipments — shipping label creation + retrieval.
				if deps.ShipmentsHandler != nil {
					orders.POST("/:id/shipments",
						deps.AuthzMiddleware.RequireTenantRelation(authz.OrdersEditRole),
						deps.ShipmentsHandler.Create)
					orders.GET("/:id/shipments",
						deps.AuthzMiddleware.RequireTenantRelation(authz.OrdersViewRole),
						deps.ShipmentsHandler.GetByOrder)
					// Advance the shipment status through its tracking
					// lifecycle (in_transit → out_for_delivery → delivered).
					// Customer-facing timeline reads the resulting
					// order_events rows.
					orders.PATCH("/:id/shipments/:shipmentId/status",
						deps.AuthzMiddleware.RequireTenantRelation(authz.OrdersEditRole),
						deps.ShipmentsHandler.UpdateStatus)
				}
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
				// Pickup details can be edited after approval without
				// changing status — typical when the courier only comes
				// back with a time slot later.
				returns.PATCH("/:id/pickup",
					deps.AuthzMiddleware.RequireTenantRelation(authz.ReturnsEditRole),
					deps.ReturnsHandler.SetPickupDetails)
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
		// Metadata endpoint: returns providers allowed for the store's country,
		// so the admin UI can render one card per supported provider instead of
		// showing an empty-state dead end.
		if deps.SettingsMetaHandler != nil {
			storeRoute.GET("/settings/supported-providers",
				deps.AuthzMiddleware.RequireTenantRelation(authz.RoleAdmin),
				deps.SettingsMetaHandler.GetSupportedProviders)
		}
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

		// Customers — C2.
		if deps.CustomersHandler != nil {
			customers := storeRoute.Group("/customers")
			{
				customers.GET("",
					deps.AuthzMiddleware.RequireTenantRelation(authz.CustomersViewRole),
					deps.CustomersHandler.List)
				customers.GET("/:id",
					deps.AuthzMiddleware.RequireTenantRelation(authz.CustomersViewRole),
					deps.CustomersHandler.Get)
				customers.PATCH("/:id/tags",
					deps.AuthzMiddleware.RequireTenantRelation(authz.CustomersEditRole),
					deps.CustomersHandler.UpdateTags)
				customers.PATCH("/:id/notes",
					deps.AuthzMiddleware.RequireTenantRelation(authz.CustomersEditRole),
					deps.CustomersHandler.UpdateNotes)
				customers.POST("/:id/block",
					deps.AuthzMiddleware.RequireTenantRelation(authz.CustomersEditRole),
					deps.CustomersHandler.Block)
				customers.POST("/:id/unblock",
					deps.AuthzMiddleware.RequireTenantRelation(authz.CustomersEditRole),
					deps.CustomersHandler.Unblock)
			}
		}

		// Reviews — C3.
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

		// Custom domains — Settings S2.
		if deps.DomainsHandler != nil {
			domains := storeRoute.Group("/domains")
			{
				domains.GET("",
					deps.AuthzMiddleware.RequireTenantRelation(authz.DomainsViewRole),
					deps.DomainsHandler.List)
				domains.POST("",
					deps.AuthzMiddleware.RequireTenantRelation(authz.DomainsEditRole),
					deps.DomainsHandler.Add)
				domains.DELETE("/:id",
					deps.AuthzMiddleware.RequireTenantRelation(authz.DomainsEditRole),
					deps.DomainsHandler.Remove)
				domains.POST("/:id/verify",
					deps.AuthzMiddleware.RequireTenantRelation(authz.DomainsViewRole),
					deps.DomainsHandler.Verify)
				domains.POST("/:id/refresh-status",
					deps.AuthzMiddleware.RequireTenantRelation(authz.DomainsViewRole),
					deps.DomainsHandler.RefreshStatus)
			}
		}

		// Subscription — Settings S3.
		if deps.SubscriptionHandler != nil {
			sub := storeRoute.Group("/subscription")
			{
				sub.GET("",
					deps.AuthzMiddleware.RequireTenantRelation(authz.SubscriptionViewRole),
					deps.SubscriptionHandler.GetSubscription)
				sub.POST("/checkout",
					deps.AuthzMiddleware.RequireTenantRelation(authz.SubscriptionEditRole),
					deps.SubscriptionHandler.CreateCheckout)
				sub.POST("/portal",
					deps.AuthzMiddleware.RequireTenantRelation(authz.SubscriptionEditRole),
					deps.SubscriptionHandler.CreatePortal)

				if deps.ChangePlanHandler != nil {
					sub.POST("/change-plan",
						deps.AuthzMiddleware.RequireTenantRelation(authz.SubscriptionEditRole),
						deps.ChangePlanHandler.ChangePlan)
					sub.GET("/change-plan/preflight",
						deps.AuthzMiddleware.RequireTenantRelation(authz.SubscriptionViewRole),
						deps.ChangePlanHandler.ChangePlanPreflight)
				}

				if deps.CompleteActionHandler != nil {
					sub.GET("/complete-action",
						deps.AuthzMiddleware.RequireTenantRelation(authz.SubscriptionViewRole),
						deps.CompleteActionHandler.Redirect)
				}

				// P10 — promo-code engine (§7).
				if deps.PromoHandler != nil {
					sub.POST("/apply-promo",
						deps.AuthzMiddleware.RequireTenantRelation(authz.SubscriptionEditRole),
						deps.PromoHandler.ApplyPromo)
					sub.DELETE("/promo",
						deps.AuthzMiddleware.RequireTenantRelation(authz.SubscriptionEditRole),
						deps.PromoHandler.CancelPromo)
				}

				// P10 — 14-day cooling-off refund (§8).
				if deps.RefundHandler != nil {
					sub.POST("/refund",
						deps.AuthzMiddleware.RequireTenantRelation(authz.SubscriptionEditRole),
						deps.RefundHandler.IssueRefund)
				}

				// P11 — merchant-initiated cancellation + save-offer (§15).
				// Cancel is allowed even in read-only states (allowlist covers
				// POST /subscription/* — see readonly/allowlist.go).
				if deps.CancelHandler != nil {
					sub.POST("/cancel",
						deps.AuthzMiddleware.RequireTenantRelation(authz.SubscriptionEditRole),
						deps.CancelHandler.Cancel)
				}
			}
		}

		// Trial billing — P5 deferred-charge card-add (§5.3).
		if deps.TrialBillingHandler != nil {
			storeRoute.POST("/billing/subscription",
				deps.AuthzMiddleware.RequireTenantRelation(authz.SubscriptionEditRole),
				deps.TrialBillingHandler.Subscribe)
		}

		// Migration fast-path — P5 merchant-initiated platform migration submit.
		// TODO: /internal/csm/migration-fast-path/:id/review wiring deferred —
		// the /internal/ group and HeaderTrustAuth chain are not mounted here.
		if deps.MigrationFastPathHandler != nil {
			storeRoute.POST("/migration-fast-path/submit",
				deps.AuthzMiddleware.RequireTenantRelation(authz.SubscriptionEditRole),
				deps.MigrationFastPathHandler.Submit)
		}

		// Audit logs — Settings S4.
		if deps.AuditLogsHandler != nil {
			auditLogs := storeRoute.Group("/audit-logs")
			{
				auditLogs.GET("",
					deps.AuthzMiddleware.RequireTenantRelation(authz.AuditLogsViewRole),
					deps.AuditLogsHandler.List)
				auditLogs.GET("/export",
					deps.AuthzMiddleware.RequireTenantRelation(authz.AuditLogsViewRole),
					deps.AuditLogsHandler.ExportCSV)
			}
		}

		// Dashboard — D1 overview + range-scoped analytics tabs.
		if deps.DashboardHandler != nil {
			storeRoute.GET("/dashboard",
				deps.AuthzMiddleware.RequireTenantRelation(authz.RoleStaff),
				deps.DashboardHandler.Get)
			metrics := storeRoute.Group("/dashboard/metrics")
			metrics.Use(deps.AuthzMiddleware.RequireTenantRelation(authz.RoleStaff))
			metrics.GET("/sales", deps.DashboardHandler.GetSalesMetrics)
			metrics.GET("/orders", deps.DashboardHandler.GetOrdersMetrics)
			metrics.GET("/customers", deps.DashboardHandler.GetCustomersMetrics)
			metrics.GET("/reviews", deps.DashboardHandler.GetReviewsMetrics)
		}

		// Tickets — D2.
		if deps.TicketsHandler != nil {
			tickets := storeRoute.Group("/tickets")
			{
				tickets.GET("",
					deps.AuthzMiddleware.RequireTenantRelation(authz.TicketsViewRole),
					deps.TicketsHandler.List)
				tickets.POST("",
					deps.AuthzMiddleware.RequireTenantRelation(authz.TicketsEditRole),
					deps.TicketsHandler.Create)
				tickets.GET("/:id",
					deps.AuthzMiddleware.RequireTenantRelation(authz.TicketsViewRole),
					deps.TicketsHandler.Get)
				tickets.POST("/:id/reply",
					deps.AuthzMiddleware.RequireTenantRelation(authz.TicketsEditRole),
					deps.TicketsHandler.Reply)
				tickets.PATCH("/:id",
					deps.AuthzMiddleware.RequireTenantRelation(authz.TicketsEditRole),
					deps.TicketsHandler.UpdateStatus)
			}
		}

		// Notifications — Settings S5.
		if deps.NotificationsHandler != nil {
			notifs := storeRoute.Group("/notifications")
			{
				notifs.GET("",
					deps.AuthzMiddleware.RequireTenantRelation(authz.NotificationsViewRole),
					deps.NotificationsHandler.List)
				notifs.GET("/unread-count",
					deps.AuthzMiddleware.RequireTenantRelation(authz.NotificationsViewRole),
					deps.NotificationsHandler.GetUnreadCount)
				notifs.PATCH("/:id/read",
					deps.AuthzMiddleware.RequireTenantRelation(authz.NotificationsEditRole),
					deps.NotificationsHandler.MarkRead)
				notifs.PATCH("/read-all",
					deps.AuthzMiddleware.RequireTenantRelation(authz.NotificationsEditRole),
					deps.NotificationsHandler.MarkAllRead)
			}
			// Notification preferences — same store scope, different path.
			storeRoute.GET("/notification-preferences",
				deps.AuthzMiddleware.RequireTenantRelation(authz.NotificationsViewRole),
				deps.NotificationsHandler.GetPreferences)
			storeRoute.PATCH("/notification-preferences",
				deps.AuthzMiddleware.RequireTenantRelation(authz.NotificationsEditRole),
				deps.NotificationsHandler.UpdatePreferences)
		}

		// Branding — B1.
		if deps.BrandingHandler != nil {
			brandingGroup := storeRoute.Group("/branding")
			{
				brandingGroup.GET("",
					deps.AuthzMiddleware.RequireTenantRelation(authz.BrandingViewRole),
					deps.BrandingHandler.Get)
				brandingGroup.PUT("",
					deps.AuthzMiddleware.RequireTenantRelation(authz.BrandingEditRole),
					deps.BrandingHandler.Update)
				brandingGroup.POST("/upload-url",
					deps.AuthzMiddleware.RequireTenantRelation(authz.BrandingEditRole),
					deps.BrandingHandler.UploadURL)
			}
		}

		// Pages — FGA is gated on the tenant relation (PageViewRole/PageEditRole).
		// Per-store ownership is enforced by the handlers themselves via an
		// explicit `page.StoreID == storeID` check after fetch (see commit 3f34647).
		// This two-layer approach (FGA for tenant membership + handler scope check
		// for store) mirrors branding and other store-scoped resources in this
		// service.
		if deps.PagesHandler != nil {
			pagesGroup := storeRoute.Group("/pages")
			{
				pagesGroup.GET("",
					deps.AuthzMiddleware.RequireTenantRelation(authz.PageViewRole),
					deps.PagesHandler.List)
				pagesGroup.POST("",
					deps.AuthzMiddleware.RequireTenantRelation(authz.PageEditRole),
					deps.PagesHandler.Create)
				pagesGroup.GET("/:id",
					deps.AuthzMiddleware.RequireTenantRelation(authz.PageViewRole),
					deps.PagesHandler.Get)
				pagesGroup.PATCH("/:id",
					deps.AuthzMiddleware.RequireTenantRelation(authz.PageEditRole),
					deps.PagesHandler.Update)
				pagesGroup.DELETE("/:id",
					deps.AuthzMiddleware.RequireTenantRelation(authz.PageEditRole),
					deps.PagesHandler.Delete)
			}
		}
	}
}
