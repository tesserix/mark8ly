package admin

import (
	"log/slog"

	"github.com/gin-gonic/gin"

	"github.com/mark8ly/marketplace-api/internal/auth"
	"github.com/mark8ly/marketplace-api/internal/authz"
)

// MobileDeps extends Deps with the mobile-specific bearer token verifier.
type MobileDeps struct {
	Deps
	// TokenVerifier is GIP or Zitadel, selected by main.go's config-driven
	// wiring; RegisterAdminMobile treats either the same way for
	// signature verification, but NOT for tenancy — see ZitadelEnabled.
	TokenVerifier auth.TokenVerifier
	// ZitadelEnabled MUST match the same flag that selected TokenVerifier
	// (cfg.ZitadelEnabled) — it decides which single middleware is the
	// source of "tenant_id" on the gin context:
	//
	//   - true:  GIPBearerAuth writes no tenant; auth.TenantFromRequest is
	//     mounted and is the only writer (FGA-validated X-Acting-Tenant-Id).
	//   - false (today's production default): auth.TenantFromRequest is
	//     NOT mounted at all — there is no X-Acting-Tenant-Id support
	//     anywhere outside this service, so mounting it would 404 every
	//     mobile-admin request — and GIPBearerAuth writes tenant_id from
	//     the GIP custom claim, exactly as before #524 phase 4.
	//
	// Exactly one of these two writers is ever active. Getting this wrong
	// either bricks the flag-off mobile app (mounting TenantFromRequest
	// with no header support anywhere) or reopens the unvalidated-claim-
	// wins-the-race bug (flag-on with the claim write still live).
	ZitadelEnabled bool
	// DualIssuer makes tenancy a per-TOKEN decision instead of a
	// per-deployment one, so a Zitadel-capable app release and the GIP
	// apps already installed can both work during the drain (#686).
	//
	// The incumbent ZitadelEnabled switch is atomic: it changes the bearer
	// verifier AND the tenant source together, so the moment it flips,
	// every installed app breaks at once. For a store-distributed app that
	// is unshippable — old installs cannot be forced to update.
	//
	// With DualIssuer, BOTH tenancy writers are mounted, and mutual
	// exclusion is replaced by ORDERING: GIPBearerAuth writes tenant_id
	// only when the token actually carries a claim (GIP tokens do,
	// Zitadel tokens never do), and TenantFromRequest runs AFTER it, so an
	// FGA-VALIDATED value can only ever overwrite an unvalidated claim,
	// never the reverse. That direction is the safety property; getting it
	// backwards reintroduces exactly the unvalidated-claim-wins bug that
	// phase 4's mutual exclusion existed to remove.
	//
	// Requires TokenVerifier to accept both issuers — see
	// auth.CompositeVerifier. Ignored unless ZitadelEnabled.
	DualIssuer bool
	// TenantMembershipChecker backs auth.TenantFromRequest (#524 phase 4):
	// it resolves the caller's X-Acting-Tenant-Id header via an FGA
	// membership check, for tokens (Zitadel) that carry no tenant_id
	// claim at all. Only read when ZitadelEnabled is true; TenantFromRequest
	// itself also tolerates a nil checker (guards before calling it), so
	// leaving this nil is safe either way.
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

// RegisterAdminMobile mounts the mobile admin route group. Uses GIPBearerAuth
// instead of HeaderTrustAuth. Same handlers, same authz, different auth.
// Includes per-user rate limiting since these routes are public-internet-facing.
func RegisterAdminMobile(router *gin.RouterGroup, deps MobileDeps) {
	if deps.TokenVerifier == nil {
		return // mobile routes disabled when no GIP config
	}

	// bearerAuth's setTenantFromClaim is the exact complement of
	// ZitadelEnabled: whichever of {this claim write, TenantFromRequest
	// below} is active, the other MUST NOT be — see ZitadelEnabled's doc
	// comment on MobileDeps for why both-active or both-inactive are each
	// a real incident, not just untidy.
	// In dual-issuer mode the claim write stays ON for both issuers: it is
	// now self-selecting, because GIPBearerAuth writes only a NON-EMPTY
	// claim and a Zitadel token has none. See MobileDeps.DualIssuer.
	bearerAuth := auth.GIPBearerAuth(deps.TokenVerifier, !deps.ZitadelEnabled || deps.DualIssuer)
	rateLimiter := auth.NewPerUserRateLimiter(60, 10) // 60 req/min, burst 10
	// Fails closed for callers with no tenant bound to their identity.
	// GIPBearerAuth intentionally lets a validly-signed token through even
	// when the caller has no tenant bound yet (a 401 there made the mobile
	// client sign the user out and bounce to /login), so this guard
	// supplies the authorization half — as 404, never 401. Mounted at
	// group level so routes without an explicit RequireTenantRelation are
	// still protected.
	requireTenant := auth.RequireBoundTenant()

	// tenantMW mirrors routes.go's tenantMW: auth, [FGA acting-tenant
	// resolver when Zitadel is selected], then the bound-tenant guard,
	// then TenantGate (#287, F1) so a suspended tenant is refused on every
	// non-store-scoped mobile group too — this is the fifth admin route
	// group the design's four-group count missed. TenantGate is a nil-safe
	// method value (see Deps.TenantGate's doc), so appending it
	// unconditionally is safe whether or not the gate is wired.
	//
	// auth.TenantFromRequest is mounted ONLY when deps.ZitadelEnabled —
	// mounting it unconditionally would make it the mobile app's sole
	// path to a tenant on GIP deployments too, and packages/mobile-shared
	// (the one client that calls this API) never sends
	// X-Acting-Tenant-Id, so every request would resolve no tenant and
	// requireTenant would 404 all of them. When it IS mounted, it MUST
	// sit here, between bearerAuth and requireTenant, and nowhere else:
	//   - before bearerAuth there is no user_id yet for its FGA
	//     membership check to use;
	//   - after requireTenant, requireTenant has already 404'd any
	//     request TenantFromRequest would otherwise have rescued by
	//     resolving tenant_id from X-Acting-Tenant-Id.
	// It is safe to mount with a nil TenantMembershipChecker/logger: it
	// never aborts, and it only ever touches the checker when the header
	// is present — see its doc comment in
	// internal/auth/tenant_from_request.go.
	tenantMW := []gin.HandlerFunc{bearerAuth}
	if deps.ZitadelEnabled || deps.DualIssuer {
		tenantMW = append(tenantMW, auth.TenantFromRequest(deps.TenantMembershipChecker, deps.TenantMembershipLogger))
	}
	tenantMW = append(tenantMW, requireTenant)
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
