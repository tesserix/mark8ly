// Package storefront — routes.go: RegisterStorefront mounts the M6 public
// read routes on a caller-supplied router group. The caller owns the API
// version prefix (typically "/api/v1").
package storefront

import (
	"log/slog"

	"github.com/gin-gonic/gin"

	"github.com/mark8ly/marketplace-api/internal/customer"
	"github.com/mark8ly/marketplace-api/internal/customerportal"
	"github.com/mark8ly/marketplace-api/internal/ratelimit"
	"github.com/mark8ly/marketplace-api/internal/stores"
)

// Deps groups every dependency the storefront route registrar needs.
// Constructed in cmd/marketplace-api/main.go.
type Deps struct {
	Handler              *StorefrontHandler
	CheckoutHandler      *CheckoutHandler
	CheckoutExtHandler   *CheckoutExtHandler
	PaymentMethodsHandler *PaymentMethodsHandler
	ShippingRatesHandler   *ShippingRatesHandler
	ShippingOptionsHandler *ShippingOptionsHandler
	WebhookHandler        *WebhookHandler
	OrderDetailHandler    *OrderDetailHandler
	CouponValidateHandler *CouponValidateHandler
	GiftCardHandler       *GiftCardStorefrontHandler
	LoyaltyHandler        *LoyaltyHandler
	SlugCache             *stores.SlugCache
	StorefrontKey        string
	CountryHandler       CountryLister // optional — set when country handler is wired
	// C1 customer auth.
	CustomerAccountHandler *CustomerAccountHandler
	CustomerService        *customer.Service
	CustomerSessionSecret  string
	// C3 reviews.
	ReviewsHandler *ReviewsHandler
	// C4 wishlists.
	WishlistHandler *WishlistHandler
	// B1 branding.
	BrandingHandler *BrandingHandler
	// Content pages.
	PagesHandler *PagesHandler
	// Domain resolution (custom domains → store slug).
	DomainResolveHandler gin.HandlerFunc
	// Support tickets — public contact form.
	TicketsHandler *TicketsHandler
	// P11 — GDPR customer portal (unauth order history + erasure request).
	CustomerPortalHandler *customerportal.Handler
	Logger                *slog.Logger
}

// CountryLister is satisfied by country.Handler.ListSupported.
type CountryLister interface {
	ListSupported(c *gin.Context)
}

// RegisterStorefront mounts the storefront routes on the given router
// group. Chain: RequireStorefrontKey → StoreContext → handler. No auth,
// no authz, no admin middleware.
func RegisterStorefront(router *gin.RouterGroup, deps Deps) {
	// Public domain resolver — no store key, no store context. Used by the
	// storefront to map custom domains (e.g. shop.mybrand.com) to a store slug.
	if deps.DomainResolveHandler != nil {
		router.GET("/storefront/resolve-domain", deps.DomainResolveHandler)
	}

	keyMW := RequireStorefrontKey(deps.StorefrontKey)
	storeMW := StoreContext(deps.SlugCache)

	// OptionalCustomerAuth is hoisted to the top of the chain so it runs
	// for EVERY storefront handler — including /checkout, which needs the
	// customer profile to stamp orders.customer_id when the shopper is
	// signed in. Previously this middleware was applied via group.Use()
	// AFTER checkout was registered (gin's Use is not retroactive), which
	// silently dropped the customer id on every order and left /account/
	// orders empty even for completed purchases.
	middlewares := []gin.HandlerFunc{keyMW, storeMW}
	if deps.CustomerService != nil {
		middlewares = append(middlewares,
			OptionalCustomerAuth(deps.CustomerSessionSecret, deps.CustomerService, deps.Logger))
	}
	group := router.Group("/storefront/stores/:storeSlug", middlewares...)
	{
		group.GET("/products", deps.Handler.List)
		group.GET("/products/:handle", deps.Handler.GetByHandle)
		group.GET("/categories", deps.Handler.ListCategories)
		group.GET("/categories/:slug/products", deps.Handler.ListByCategorySlug)

		// Checkout — use extended handler (with payment/tax/shipping) if
		// available, fall back to the simple M5 checkout.
		if deps.CheckoutExtHandler != nil {
			group.POST("/checkout", deps.CheckoutExtHandler.Checkout)
		} else if deps.CheckoutHandler != nil {
			group.POST("/checkout", deps.CheckoutHandler.Checkout)
		}

		// Payment methods and shipping rates for checkout UI.
		if deps.PaymentMethodsHandler != nil {
			group.GET("/payment-methods", deps.PaymentMethodsHandler.ListPaymentMethods)
		}
		if deps.ShippingRatesHandler != nil {
			group.POST("/shipping-rates", deps.ShippingRatesHandler.GetRates)
		}
		if deps.ShippingOptionsHandler != nil {
			// Public metadata about which carriers ship where — lets the
			// checkout UI render a helpful fallback ("we ship to India")
			// when a customer's address falls outside any carrier's zone.
			group.GET("/shipping-options", deps.ShippingOptionsHandler.GetOptions)
		}
		if deps.OrderDetailHandler != nil {
			group.GET("/orders/:id", deps.OrderDetailHandler.GetOrder)
			// Customer self-service cancel — gated server-side on
			// order ownership + status machine + shipment guard.
			group.POST("/orders/:id/cancel", deps.OrderDetailHandler.Cancel)
			// Customer self-service return/replace. GET returns any open
			// request so the order page can render a status banner; POST
			// creates a new request (one open per order in v1).
			group.GET("/orders/:id/returns", deps.OrderDetailHandler.ListReturnsForOrder)
			group.POST("/orders/:id/returns", deps.OrderDetailHandler.RequestReturn)
		}

		// Razorpay client-side checkout callback. The widget on the order
		// page hands back {razorpay_order_id, razorpay_payment_id,
		// razorpay_signature}; this endpoint verifies the HMAC and reuses
		// the same handler the webhook flow uses to mark the order paid.
		if deps.WebhookHandler != nil {
			group.POST("/orders/:id/verify-payment",
				deps.WebhookHandler.HandleVerifyPayment)
		}

		// Coupon validation — Marketing M1.
		// Rate-limited: 10 req/min per IP.
		if deps.CouponValidateHandler != nil {
			group.POST("/coupons/validate",
				ratelimit.PerIP(0.167, 10), // ~10 req/min
				deps.CouponValidateHandler.Validate)
		}

		// Gift cards — Marketing M2.
		// Rate-limited: 10 req/min per IP for lookups, 5 req/min for
		// purchases (each creates a Stripe Checkout Session — don't let
		// abusers spin up unlimited pending cards).
		if deps.GiftCardHandler != nil {
			group.POST("/gift-cards/check-balance",
				ratelimit.PerIP(0.167, 10),
				deps.GiftCardHandler.CheckBalance)
			group.POST("/gift-cards/purchase",
				ratelimit.PerIP(0.083, 5), // ~5 req/min
				deps.GiftCardHandler.Purchase)
		}

		// Reviews — C3. Public read endpoint (no auth).
		if deps.ReviewsHandler != nil {
			group.GET("/products/:handle/reviews", deps.ReviewsHandler.ListProductReviews)

			// Guest (unauthenticated) review submission. Comments
			// stay gated behind sign-in. Anonymous reactions need
			// migration 40 (cookie-keyed uniqueness) and ship next.
			group.POST("/products/:handle/reviews-guest",
				ratelimit.PerIP(0.083, 5), // ~5 req/min
				deps.ReviewsHandler.SubmitGuestReview)
		}

		// Branding — B1. Public, cached, no auth.
		if deps.BrandingHandler != nil {
			group.GET("/branding", deps.BrandingHandler.Get)
		}

		// Content pages — public, no auth.
		if deps.PagesHandler != nil {
			group.GET("/pages", deps.PagesHandler.List)
			group.GET("/pages/:pageSlug", deps.PagesHandler.Get)
		}

		// Support tickets — public contact form.
		// Rate-limited: 3 req/min per IP — ticket spam can be worse than
		// order spam because tickets have no payment friction.
		if deps.TicketsHandler != nil {
			group.POST("/support/tickets",
				ratelimit.PerIP(0.05, 3),
				deps.TicketsHandler.Create)
		}

		// P11 — GDPR customer portal (§15.4). No auth middleware — HMAC token IS the auth.
		// Rate-limited to prevent token-grinding attempts.
		if deps.CustomerPortalHandler != nil {
			group.GET("/my-orders/:email/:order_token",
				ratelimit.PerIP(0.083, 5), // 5 req/min per IP
				deps.CustomerPortalHandler.MyOrders)
			group.POST("/customer-erasure",
				ratelimit.PerIP(0.05, 3), // 3 req/min per IP
				deps.CustomerPortalHandler.CustomerErasure)
		}

		// Loyalty — Marketing M3. Public endpoints, no auth.
		// Rate-limited: 10 req/min per IP for enroll and redeem.
		if deps.LoyaltyHandler != nil {
			loyaltyGroup := group.Group("/loyalty")
			{
				loyaltyGroup.GET("/program", deps.LoyaltyHandler.GetProgram)
				loyaltyGroup.POST("/enroll",
					ratelimit.PerIP(0.167, 10),
					deps.LoyaltyHandler.Enroll)
				loyaltyGroup.GET("/me",
					ratelimit.PerIP(0.167, 10),
					deps.LoyaltyHandler.GetMe)
				loyaltyGroup.POST("/redeem",
					ratelimit.PerIP(0.167, 10),
					deps.LoyaltyHandler.Redeem)
			}
		}
	}

	// C1 — Customer account routes (auth required).
	if deps.CustomerAccountHandler != nil {
		requireAuth := RequireCustomerAuth()
		// OptionalCustomerAuth is already mounted at the top of the group
		// (see RegisterStorefront prologue), so /account/* only needs the
		// strict-require wrapper on top.
		account := group.Group("/account", requireAuth)
		{
			account.GET("", deps.CustomerAccountHandler.GetProfile)
			account.PATCH("", deps.CustomerAccountHandler.UpdateProfile)
			account.GET("/addresses", deps.CustomerAccountHandler.ListAddresses)
			account.POST("/addresses", deps.CustomerAccountHandler.CreateAddress)
			account.PATCH("/addresses/:id", deps.CustomerAccountHandler.UpdateAddress)
			account.DELETE("/addresses/:id", deps.CustomerAccountHandler.DeleteAddress)

			// Authenticated customer's gift cards (purchaser or recipient).
			if deps.GiftCardHandler != nil {
				account.GET("/gift-cards", deps.GiftCardHandler.MyGiftCards)
			}

			// Order history — list orders for authenticated customer.
			if deps.OrderDetailHandler != nil {
				account.GET("/orders", deps.OrderDetailHandler.ListOrders)
			}

			// C3 — Reviews (authenticated: submit + react + comment).
			if deps.ReviewsHandler != nil {
				account.POST("/products/:handle/reviews",
					ratelimit.PerIP(0.167, 10), // ~10 req/min
					deps.ReviewsHandler.SubmitReview)
				account.POST("/reviews/:id/reactions",
					ratelimit.PerIP(0.167, 10), // ~10 req/min
					deps.ReviewsHandler.AddReaction)
				// Nested thread of comments under an approved review.
				// Body optionally carries parent_reply_id for replies-
				// to-replies; missing = top-level comment on the review.
				account.POST("/reviews/:id/replies",
					ratelimit.PerIP(0.167, 10), // ~10 req/min
					deps.ReviewsHandler.AddCustomerReply)
			}

			// C4 — Wishlists.
			if deps.WishlistHandler != nil {
				account.GET("/wishlist", deps.WishlistHandler.List)
				account.POST("/wishlist", deps.WishlistHandler.Add)
				account.DELETE("/wishlist/:productId", deps.WishlistHandler.Remove)
				account.GET("/wishlist/check/:productId", deps.WishlistHandler.Check)
			}

			// Support tickets — authenticated customer view. The public
			// /support/tickets POST above creates tickets; this group
			// lets the signed-in submitter read their own tickets and
			// follow up. Scoping is by submitter email + store_id, so
			// a customer can never see another customer's ticket.
			if deps.TicketsHandler != nil {
				account.GET("/tickets",
					ratelimit.PerIP(0.167, 10),
					deps.TicketsHandler.ListMine)
				account.GET("/tickets/:id",
					ratelimit.PerIP(0.167, 10),
					deps.TicketsHandler.GetMine)
				account.POST("/tickets/:id/reply",
					ratelimit.PerIP(0.083, 5), // ~5 req/min — reply spam guard
					deps.TicketsHandler.AddMyReply)
			}
		}
	}

	// Webhook endpoints — at the API root, not under /storefront/stores.
	//
	// Two routes, deliberately on DIFFERENT prefixes:
	//   • /webhooks/:storeSlug/:provider — preferred. Config is scoped
	//     by the store named in the URL, so a callback for tenant A
	//     cannot match tenant B's config.
	//   • /legacy-webhooks/:provider — pre-scoped fallback. Only
	//     processes when exactly one active gateway config exists for
	//     the provider across the DB; otherwise fails closed
	//     (see 2026-04-10 prod-readiness §2.3). New merchant onboarding
	//     MUST use the scoped URL above.
	//
	// These used to share the "/webhooks/" prefix, but gin rejects two
	// differently-named wildcards at the same tree level and crash-
	// looped the storefront pod on boot. Moving the legacy fallback
	// to its own root restores both paths without a behaviour change.
	if deps.WebhookHandler != nil {
		router.POST("/webhooks/:storeSlug/:provider", deps.WebhookHandler.HandleScopedWebhook)
		router.POST("/legacy-webhooks/:provider", deps.WebhookHandler.HandleWebhook)
	}

	// Public reference data — no auth, no store context.
	if deps.CountryHandler != nil {
		router.GET("/public/supported-countries", deps.CountryHandler.ListSupported)
	}
}
