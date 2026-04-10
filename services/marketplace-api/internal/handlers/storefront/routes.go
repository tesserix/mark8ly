// Package storefront — routes.go: RegisterStorefront mounts the M6 public
// read routes on a caller-supplied router group. The caller owns the API
// version prefix (typically "/api/v1").
package storefront

import (
	"log/slog"

	"github.com/gin-gonic/gin"

	"github.com/mark8ly/marketplace-api/internal/customer"
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
	ShippingRatesHandler *ShippingRatesHandler
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
	Logger          *slog.Logger
}

// CountryLister is satisfied by country.Handler.ListSupported.
type CountryLister interface {
	ListSupported(c *gin.Context)
}

// RegisterStorefront mounts the storefront routes on the given router
// group. Chain: RequireStorefrontKey → StoreContext → handler. No auth,
// no authz, no admin middleware.
func RegisterStorefront(router *gin.RouterGroup, deps Deps) {
	keyMW := RequireStorefrontKey(deps.StorefrontKey)
	storeMW := StoreContext(deps.SlugCache)

	group := router.Group("/storefront/stores/:storeSlug", keyMW, storeMW)
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
		if deps.OrderDetailHandler != nil {
			group.GET("/orders/:id", deps.OrderDetailHandler.GetOrder)
		}

		// Coupon validation — Marketing M1.
		// Rate-limited: 10 req/min per IP.
		if deps.CouponValidateHandler != nil {
			group.POST("/coupons/validate",
				ratelimit.PerIP(0.167, 10), // ~10 req/min
				deps.CouponValidateHandler.Validate)
		}

		// Gift cards — Marketing M2.
		// Rate-limited: 10 req/min per IP.
		if deps.GiftCardHandler != nil {
			group.POST("/gift-cards/check-balance",
				ratelimit.PerIP(0.167, 10), // ~10 req/min
				deps.GiftCardHandler.CheckBalance)
		}

		// Reviews — C3. Public read endpoint (no auth).
		if deps.ReviewsHandler != nil {
			group.GET("/products/:handle/reviews", deps.ReviewsHandler.ListProductReviews)
		}

		// Branding — B1. Public, cached, no auth.
		if deps.BrandingHandler != nil {
			group.GET("/branding", deps.BrandingHandler.Get)
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
		optionalAuth := OptionalCustomerAuth(deps.CustomerSessionSecret, deps.CustomerService, deps.Logger)
		requireAuth := RequireCustomerAuth()

		// Apply optional auth to ALL storefront routes so customer context
		// is available even on product pages (for future review submission).
		group.Use(optionalAuth)

		account := group.Group("/account", requireAuth)
		{
			account.GET("", deps.CustomerAccountHandler.GetProfile)
			account.PATCH("", deps.CustomerAccountHandler.UpdateProfile)
			account.GET("/addresses", deps.CustomerAccountHandler.ListAddresses)
			account.POST("/addresses", deps.CustomerAccountHandler.CreateAddress)
			account.PATCH("/addresses/:id", deps.CustomerAccountHandler.UpdateAddress)
			account.DELETE("/addresses/:id", deps.CustomerAccountHandler.DeleteAddress)

			// C3 — Reviews (authenticated: submit + react).
			if deps.ReviewsHandler != nil {
				account.POST("/products/:handle/reviews",
					ratelimit.PerIP(0.167, 10), // ~10 req/min
					deps.ReviewsHandler.SubmitReview)
				account.POST("/reviews/:id/reactions",
					ratelimit.PerIP(0.167, 10), // ~10 req/min
					deps.ReviewsHandler.AddReaction)
			}

			// C4 — Wishlists.
			if deps.WishlistHandler != nil {
				account.GET("/wishlist", deps.WishlistHandler.List)
				account.POST("/wishlist", deps.WishlistHandler.Add)
				account.DELETE("/wishlist/:productId", deps.WishlistHandler.Remove)
				account.GET("/wishlist/check/:productId", deps.WishlistHandler.Check)
			}
		}
	}

	// Webhook endpoints — at the API root, not under /storefront/stores.
	if deps.WebhookHandler != nil {
		router.POST("/webhooks/:provider", deps.WebhookHandler.HandleWebhook)
	}

	// Public reference data — no auth, no store context.
	if deps.CountryHandler != nil {
		router.GET("/public/supported-countries", deps.CountryHandler.ListSupported)
	}
}
