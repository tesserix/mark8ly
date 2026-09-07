package admin

import (
	"log/slog"

	"github.com/gin-gonic/gin"

	"github.com/mark8ly/marketplace-api/internal/auth"
	"github.com/mark8ly/marketplace-api/internal/authz"
	"github.com/mark8ly/marketplace-api/internal/ratelimit"
)

// MobileDeps extends Deps with the mobile-specific bearer token verifier.
type MobileDeps struct {
	Deps
	// TokenVerifier is the Zitadel bearer verifier built by main.go's
	// wiring (#786 collapsed the GIP alternative away). Nil disables the
	// entire mobile admin group — see RegisterAdminMobile. It is used for
	// signature verification ONLY: it is never a source of tenancy, no
	// matter what a token asserts. See TenantMembershipChecker.
	TokenVerifier auth.TokenVerifier
	// MyTenantsHandler serves the ONE mobile admin route mounted outside
	// the tenant gate — see its doc comment for why that is required
	// rather than an oversight. Nil leaves the route unmounted, which
	// disables Zitadel tenant discovery on mobile.
	MyTenantsHandler *MobileMyTenantsHandler
	// LoginHandler serves the app's public sign-in route — the only mobile
	// route mounted with NO bearer auth at all, because it is how a client
	// obtains one. Nil leaves it unmounted.
	LoginHandler *MobileLoginHandler
	// IDPHandler serves "Continue with Google" (#686 item 1). Mounted in
	// the same unauthenticated, IP-limited group as LoginHandler, for the
	// same reason: these routes are how a client obtains a token. Nil
	// leaves them unmounted and the app falls back to its Firebase path.
	IDPHandler *MobileIDPHandler
	// TenantMembershipChecker backs auth.TenantFromRequest (#524 phase 4):
	// it resolves the caller's X-Acting-Tenant-Id header via an FGA
	// membership check. Since #786 this is the SOLE source of tenancy on
	// the mobile admin group — Zitadel tokens carry no tenant_id claim at
	// all, and auth.BearerAuth would ignore one if they did.
	// TenantFromRequest tolerates a nil checker (it guards before calling
	// it), so leaving this nil is safe — it simply means no request ever
	// resolves a tenant and every tenant-gated route 404s.
	TenantMembershipChecker auth.TenantMembershipChecker
	// TenantMembershipLogger is passed to auth.TenantFromRequest for its
	// (fail-closed, never-abort) FGA-error logging. May be nil.
	TenantMembershipLogger *slog.Logger
	PushTokenHandler       *PushTokenHandler
	// PlatformSupportHandler bridges merchant→platform support chat to
	// otto's platform tenant. Nil when otto isn't wired.
	PlatformSupportHandler *PlatformSupportHandler
	// TeamHandler proxies tenant team management to platform-api. Nil when
	// the platform client isn't configured (MARKETPLACE_PLATFORM_API_URL empty).
	TeamHandler *TeamHandler
	// MobileAccountHandler proxies the mobile "delete my account" flow to
	// platform-api (Apple App Store requires an in-app deletion path). Nil
	// when the platform client isn't configured, same gate as TeamHandler.
	MobileAccountHandler *MobileAccountHandler
}

// RegisterAdminMobile mounts the mobile admin route group. Uses auth.BearerAuth
// instead of HeaderTrustAuth. Same handlers, same authz, different auth.
// Includes per-user rate limiting since these routes are public-internet-facing.
func RegisterAdminMobile(router *gin.RouterGroup, deps MobileDeps) {
	if deps.TokenVerifier == nil {
		return // mobile routes disabled when Zitadel isn't configured
	}

	// bearerAuth authenticates only: it sets user_id and never tenant_id.
	// Since #786 there is exactly one tenancy writer in this chain —
	// TenantFromRequest, below — so the mutual-exclusion bookkeeping the
	// GIP/Zitadel split needed is gone. Do not reintroduce a second
	// writer: an unvalidated claim racing an FGA-validated result for the
	// same context key is the bug #524 phase 4 removed.
	bearerAuth := auth.BearerAuth(deps.TokenVerifier)
	rateLimiter := auth.NewPerUserRateLimiter(60, 10) // 60 req/min, burst 10
	// Fails closed for callers with no tenant bound to their identity.
	// BearerAuth intentionally lets a validly-signed token through even
	// when the caller has no tenant bound yet (a 401 there made the mobile
	// client sign the user out and bounce to /login), so this guard
	// supplies the authorization half — as 404, never 401. Mounted at
	// group level so routes without an explicit RequireTenantRelation are
	// still protected.
	requireTenant := auth.RequireBoundTenant()

	// tenantMW mirrors routes.go's tenantMW: auth, the FGA acting-tenant
	// resolver, then the bound-tenant guard, then TenantGate (#287, F1) so
	// a suspended tenant is refused on every non-store-scoped mobile group
	// too — this is the fifth admin route group the design's four-group
	// count missed. TenantGate is a nil-safe method value (see
	// Deps.TenantGate's doc), so appending it unconditionally is safe
	// whether or not the gate is wired.
	//
	// auth.TenantFromRequest is now mounted unconditionally (#786): with
	// GIP gone, no token reaching this group can carry a tenant claim, so
	// the FGA-validated X-Acting-Tenant-Id header is the only path to a
	// tenant that exists. It MUST sit here, between bearerAuth and
	// requireTenant, and nowhere else:
	//   - before bearerAuth there is no user_id yet for its FGA
	//     membership check to use;
	//   - after requireTenant, requireTenant has already 404'd any
	//     request TenantFromRequest would otherwise have rescued by
	//     resolving tenant_id from X-Acting-Tenant-Id.
	// It is safe to mount with a nil TenantMembershipChecker/logger: it
	// never aborts, and it only ever touches the checker when the header
	// is present — see its doc comment in
	// internal/auth/tenant_from_request.go.
	tenantMW := []gin.HandlerFunc{
		bearerAuth,
		auth.TenantFromRequest(deps.TenantMembershipChecker, deps.TenantMembershipLogger),
		requireTenant,
	}
	if deps.TenantGate != nil {
		tenantMW = append(tenantMW, deps.TenantGate)
	}
	tenantMW = append(tenantMW, rateLimiter)

	// Platform support chat — merchant admin → Tesserix platform team.
	// Not store-scoped: it rides the admin's tenant from the bearer token,
	// so any authenticated merchant admin can open a platform chat.
	if deps.PlatformSupportHandler != nil {
		ps := router.Group("/mobile/admin/platform-support", tenantMW...)
		deps.PlatformSupportHandler.Register(ps)
	}

	// Account deletion — proxies to platform-api. Not store-scoped: it rides
	// the caller's tenant + UID from the bearer token, so any tenant member
	// can delete their own account. No FGA/RequireTenantRelation gate here
	// on purpose — platform-api is authoritative on owner-vs-staff teardown,
	// and Apple requires the deletion path to work for staff too.
	if deps.MobileAccountHandler != nil {
		acct := router.Group("/mobile/admin/account", tenantMW...)
		acct.DELETE("", deps.MobileAccountHandler.Delete)
	}

	// Sign-in — mounted with NO bearer auth, because it is what produces a
	// bearer token. IP rate limiting stands in for the per-user limiter
	// used everywhere else: there is no user yet, and this is the one
	// mobile route an unauthenticated caller can reach, so it is the only
	// credential-stuffing surface in this group.
	if deps.LoginHandler != nil {
		a := router.Group("/mobile/admin/auth", ratelimit.PerIP(0.5, 10))
		a.POST("/login", deps.LoginHandler.Login)
		// Completing the emailed challenge. Also unauthenticated — it is
		// the second half of obtaining a token, not something a token
		// protects — and under the same IP limit, since guessing a
		// six-digit code is the other brute-forceable surface here.
		a.POST("/otp/verify", deps.LoginHandler.VerifyOTP)
		// Completing the authenticator-app challenge. Same group, same IP
		// limit, and for the same reason: guessing a six-digit TOTP is the
		// identical brute-forceable surface, and it is the second half of
		// obtaining a token rather than something a token protects.
		a.POST("/totp/verify", deps.LoginHandler.VerifyTOTP)
		// Resending the emailed code (#686 item 3). Same group, same IP
		// limit: it is part of obtaining a token, so no bearer can gate
		// it. Its own abuse budget lives in auth-bff's emailotp limiter —
		// a small number of codes per address per window — which this
		// route surfaces as a distinct `rate_limited` rather than hiding.
		a.POST("/otp/resend", deps.LoginHandler.ResendOTP)
	}

	// Federated sign-in (#686 item 1) — same unauthenticated group, same
	// IP limit, and for the same reason: /idp/start and /idp/finish are
	// how a client obtains a token, so no bearer auth can gate them, and
	// they are the other credential-adjacent surface an unauthenticated
	// caller can reach. Mounted as its own group only because it may be
	// wired independently of LoginHandler.
	if deps.IDPHandler != nil {
		idp := router.Group("/mobile/admin/auth/idp", ratelimit.PerIP(0.5, 10))
		idp.POST("/start", deps.IDPHandler.Start)
		idp.POST("/finish", deps.IDPHandler.Finish)
	}

	// Tenant discovery — the ONLY mobile admin route mounted WITHOUT
	// requireTenant, and deliberately so: it is how a Zitadel-authenticated
	// client learns the tenant it will then send as X-Acting-Tenant-Id.
	// Gating it would deadlock that flow (a tenant would be needed to
	// discover the tenant). Authentication still applies — bearerAuth and
	// the per-user rate limiter are both here; only the tenant gate is
	// absent. Do NOT "tidy" this onto tenantMW.
	if deps.MyTenantsHandler != nil {
		me := router.Group("/mobile/admin/me", bearerAuth, rateLimiter)
		me.GET("/tenants", deps.MyTenantsHandler.List)
	}

	// Tenant-wide routes
	if deps.StoresHandler != nil {
		mobileRoot := router.Group("/mobile/admin", tenantMW...)
		mobileRoot.GET("/stores",
			deps.AuthzMiddleware.RequireTenantRelation(authz.RoleStaff),
			deps.StoresHandler.List)
	}

	// storeRoute chain: bearerAuth -> requireTenant -> TenantGate ->
	// rateLimiter -> StoresMiddleware, matching routes.go's storeMW
	// ordering (tenantMW then StoresMiddleware). TenantGate runs and
	// aborts BEFORE StoresMiddleware ever executes, so a suspended tenant
	// never reaches the store-ownership lookup — no double-abort, just the
	// same short-circuit routes.go already relies on.
	storeRoute := router.Group("/mobile/admin/stores/:storeId", append(append([]gin.HandlerFunc{}, tenantMW...), deps.StoresMiddleware)...)
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

				// Customer-facing document emails — mirror routes.go:302-310.
				// Resend the invoice / receipt on demand (same handlers, same
				// authz as web; only the route group differs).
				orders.POST("/:id/invoice/email",
					deps.AuthzMiddleware.RequireTenantRelation(authz.OrdersEditRole),
					deps.OrdersHandler.EmailInvoice)
				orders.POST("/:id/receipt/email",
					deps.AuthzMiddleware.RequireTenantRelation(authz.OrdersEditRole),
					deps.OrdersHandler.EmailReceipt)

				// Shipments — mirror routes.go:312-355. Guarded on the
				// handler so a deployment without shipping wired stays
				// route-clean. Handler/service/DTO shared with web.
				if deps.ShipmentsHandler != nil {
					orders.POST("/:id/shipments",
						deps.AuthzMiddleware.RequireTenantRelation(authz.OrdersEditRole),
						deps.ShipmentsHandler.Create)
					orders.GET("/:id/shipments",
						deps.AuthzMiddleware.RequireTenantRelation(authz.OrdersViewRole),
						deps.ShipmentsHandler.GetByOrder)
					orders.PATCH("/:id/shipments/:shipmentId/status",
						deps.AuthzMiddleware.RequireTenantRelation(authz.OrdersEditRole),
						deps.ShipmentsHandler.UpdateStatus)
					orders.GET("/:id/shipments/:shipmentId/label",
						deps.AuthzMiddleware.RequireTenantRelation(authz.OrdersViewRole),
						deps.ShipmentsHandler.DownloadLabel)
					orders.POST("/:id/shipments/:shipmentId/label/email",
						deps.AuthzMiddleware.RequireTenantRelation(authz.OrdersEditRole),
						deps.ShipmentsHandler.EmailLabel)
					orders.POST("/:id/shipments/:shipmentId/tracking/refresh",
						deps.AuthzMiddleware.RequireTenantRelation(authz.OrdersEditRole),
						deps.ShipmentsHandler.RefreshTracking)
					orders.DELETE("/:id/shipments/:shipmentId",
						deps.AuthzMiddleware.RequireTenantRelation(authz.OrdersEditRole),
						deps.ShipmentsHandler.Delete)
					orders.POST("/:id/shipments/:shipmentId/pickup/schedule",
						deps.AuthzMiddleware.RequireTenantRelation(authz.OrdersEditRole),
						deps.ShipmentsHandler.SchedulePickup)
					// "Cancel / return shipment". Landed on the web table
					// (routes.go) after this group was copied from it — the
					// same drift that hid gift-card enable/disable. The mobile
					// client has always called it (packages/mobile-shared/
					// api/shipments.ts → ShippingPanel), so every tap 404'd at
					// the gin tree with the handler never reached.
					orders.POST("/:id/shipments/:shipmentId/cancel",
						deps.AuthzMiddleware.RequireTenantRelation(authz.OrdersEditRole),
						deps.ShipmentsHandler.CancelShipment)
				}
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

		// ── Marketing hub ── mirrors web routes.go 449-635. Handlers are
		// shared with web (all on Deps, MobileDeps embeds Deps → no main.go
		// wiring); this only re-exposes the same routes on the mobile group,
		// each guarded on its handler so a partial deployment stays clean.

		// Coupons (routes.go:449-469) — Staff view, Admin mutate.
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

		// Gift cards (routes.go:471-485).
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
				// Disable/Enable landed on the web table (routes.go) after
				// this group was copied from it, so the phone could not reach
				// them at all — gin refused the request at the tree and the
				// handler never ran. Same handlers, same GiftCardsEditRole
				// gate as Issue above; only the auth chain differs (bearer
				// rather than header-trust), which the group already applies.
				gc.POST("/:id/disable",
					deps.AuthzMiddleware.RequireTenantRelation(authz.GiftCardsEditRole),
					deps.GiftCardHandler.Disable)
				gc.POST("/:id/enable",
					deps.AuthzMiddleware.RequireTenantRelation(authz.GiftCardsEditRole),
					deps.GiftCardHandler.Enable)
			}
		}

		// Loyalty program (routes.go:487-513).
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

		// Campaigns (routes.go:531-563) — View list/get, Edit mutate + lifecycle.
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

		// Segments (routes.go:615-635) — reuse Campaigns roles.
		if deps.SegmentHandler != nil {
			segments := storeRoute.Group("/segments")
			{
				segments.GET("",
					deps.AuthzMiddleware.RequireTenantRelation(authz.CampaignsViewRole),
					deps.SegmentHandler.List)
				segments.POST("",
					deps.AuthzMiddleware.RequireTenantRelation(authz.CampaignsEditRole),
					deps.SegmentHandler.Create)
				segments.GET("/:id",
					deps.AuthzMiddleware.RequireTenantRelation(authz.CampaignsViewRole),
					deps.SegmentHandler.Get)
				segments.PATCH("/:id",
					deps.AuthzMiddleware.RequireTenantRelation(authz.CampaignsEditRole),
					deps.SegmentHandler.Update)
				segments.DELETE("/:id",
					deps.AuthzMiddleware.RequireTenantRelation(authz.CampaignsEditRole),
					deps.SegmentHandler.Delete)
			}
		}

		// ── Store settings ── mirrors web routes.go. Handlers shared with web
		// (all on Deps). CSV export + logo upload-url are intentionally NOT
		// exposed on mobile (file streams / signed-upload flows are web-only).

		// Branding (routes.go:849-862) — GET + PUT. The mobile UI edits only
		// the text basics; the PUT body is all-optional (partial update).
		if deps.BrandingHandler != nil {
			branding := storeRoute.Group("/branding")
			{
				branding.GET("",
					deps.AuthzMiddleware.RequireTenantRelation(authz.BrandingViewRole),
					deps.BrandingHandler.Get)
				branding.PUT("",
					deps.AuthzMiddleware.RequireTenantRelation(authz.BrandingEditRole),
					deps.BrandingHandler.Update)
			}
		}

		// Audit logs (routes.go:774-787) — read-only list. Export CSV deferred.
		if deps.AuditLogsHandler != nil {
			auditLogs := storeRoute.Group("/audit-logs")
			{
				auditLogs.GET("",
					deps.AuthzMiddleware.RequireTenantRelation(authz.AuditLogsViewRole),
					deps.AuditLogsHandler.List)
			}
		}

		// Support tickets (routes.go:802-820) — full: list/create/get/reply/status.
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

		// Team / staff — proxies platform-api's tenant team endpoints. Mounted
		// under the store group so the mobile client's store-prefixed requests
		// resolve; TeamHandler reads tenant_id + actor UID from the bearer token
		// and ignores :storeId. Members are staff-visible; invitations + role
		// changes are Admin-gated (platform-api enforces the finer owner/admin
		// guard using the actor UID).
		if deps.TeamHandler != nil {
			team := storeRoute.Group("/team")
			{
				team.GET("/members",
					deps.AuthzMiddleware.RequireTenantRelation(authz.RoleStaff),
					deps.TeamHandler.ListMembers)
				team.PATCH("/members/role",
					deps.AuthzMiddleware.RequireTenantRelation(authz.RoleAdmin),
					deps.TeamHandler.UpdateRole)
				team.GET("/invitations",
					deps.AuthzMiddleware.RequireTenantRelation(authz.RoleAdmin),
					deps.TeamHandler.ListInvitations)
				team.POST("/invitations",
					deps.AuthzMiddleware.RequireTenantRelation(authz.RoleAdmin),
					deps.TeamHandler.Invite)
				team.DELETE("/invitations/:invId",
					deps.AuthzMiddleware.RequireTenantRelation(authz.RoleAdmin),
					deps.TeamHandler.Revoke)
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
			// Per-type notification preferences — mirrors web routes.go.
			// Same store scope, different path; store-wide (governs whether
			// each notification type is generated at all, bell + push).
			storeRoute.GET("/notification-preferences",
				deps.AuthzMiddleware.RequireTenantRelation(authz.NotificationsViewRole),
				deps.NotificationsHandler.GetPreferences)
			storeRoute.PATCH("/notification-preferences",
				deps.AuthzMiddleware.RequireTenantRelation(authz.NotificationsEditRole),
				deps.NotificationsHandler.UpdatePreferences)
		}
	}
}
