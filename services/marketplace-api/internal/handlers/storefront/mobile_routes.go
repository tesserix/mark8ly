// Package storefront — mobile_routes.go: RegisterMobileStorefront mounts the
// mobile storefront route group under /mobile/storefront/stores/:storeSlug.
// Uses GIP Bearer token auth instead of X-Storefront-Key + session cookies.
package storefront

import (
	"log/slog"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/mark8ly/marketplace-api/internal/customer"
	"github.com/mark8ly/marketplace-api/internal/ratelimit"
	"github.com/mark8ly/marketplace-api/internal/stores"
)

// MobileDeps groups every dependency the mobile storefront route registrar
// needs. Constructed in cmd/marketplace-api/main.go.
type MobileDeps struct {
	Handler               *StorefrontHandler
	CheckoutHandler       *CheckoutHandler
	CheckoutExtHandler    *CheckoutExtHandler
	PaymentMethodsHandler *PaymentMethodsHandler
	ShippingRatesHandler  *ShippingRatesHandler
	OrderDetailHandler    *OrderDetailHandler
	CouponValidateHandler *CouponValidateHandler
	GiftCardHandler       *GiftCardStorefrontHandler
	LoyaltyHandler        *LoyaltyHandler
	SlugCache             *stores.SlugCache
	// C1 customer auth.
	CustomerAccountHandler *CustomerAccountHandler
	CustomerService        *customer.Service
	// C3 reviews.
	ReviewsHandler *ReviewsHandler
	// C4 wishlists.
	WishlistHandler *WishlistHandler
	// B1 branding.
	BrandingHandler *BrandingHandler
	// Mobile-specific.
	PushTokenHandler *PushTokenHandler
	NotifyMeHandler  *NotifyMeHandler
	DevMode          bool
	Logger           *slog.Logger
}

// RegisterMobileStorefront mounts the mobile storefront routes on the given
// router group. Chain: StoreContext → handler. Auth via GIP Bearer token
// instead of X-Storefront-Key.
func RegisterMobileStorefront(router *gin.RouterGroup, deps MobileDeps) {
	storeMW := StoreContext(deps.SlugCache)

	// All mobile storefront routes share the store context middleware.
	group := router.Group("/mobile/storefront/stores/:storeSlug", storeMW)

	// --- Public routes (no auth required) ---
	{
		if deps.Handler != nil {
			group.GET("/products", deps.Handler.List)
			group.GET("/products/:handle", deps.Handler.GetByHandle)
			group.GET("/categories", deps.Handler.ListCategories)
			group.GET("/categories/:slug/products", deps.Handler.ListByCategorySlug)
		}

		if deps.PaymentMethodsHandler != nil {
			group.GET("/payment-methods", deps.PaymentMethodsHandler.ListPaymentMethods)
		}

		// Reviews — public read endpoint.
		if deps.ReviewsHandler != nil {
			group.GET("/products/:handle/reviews", deps.ReviewsHandler.ListProductReviews)
		}

		// Branding — public, cached.
		if deps.BrandingHandler != nil {
			group.GET("/branding", deps.BrandingHandler.Get)
		}

		// Loyalty — public program info.
		if deps.LoyaltyHandler != nil {
			group.GET("/loyalty/program", deps.LoyaltyHandler.GetProgram)
		}
	}

	// --- Guest-accessible routes (optional auth) ---
	{
		optionalAuth := OptionalGIPBearerAuth(deps.DevMode)
		guestCustomerMW := mobileCustomerProfileMW(deps.CustomerService, deps.Logger)

		if deps.ShippingRatesHandler != nil {
			group.POST("/checkout/shipping-rates",
				optionalAuth, guestCustomerMW,
				deps.ShippingRatesHandler.GetRates)
		}

		if deps.CheckoutExtHandler != nil {
			group.POST("/checkout/submit",
				optionalAuth, guestCustomerMW,
				deps.CheckoutExtHandler.Checkout)
		} else if deps.CheckoutHandler != nil {
			group.POST("/checkout/submit",
				optionalAuth, guestCustomerMW,
				deps.CheckoutHandler.Checkout)
		}
	}

	// --- Authenticated routes (GIP Bearer required) ---
	{
		requireAuth := GIPBearerAuth(deps.DevMode)
		customerMW := mobileCustomerProfileMW(deps.CustomerService, deps.Logger)
		requireCustomer := RequireCustomerAuth()

		authed := group.Group("", requireAuth, customerMW, requireCustomer)
		{
			// Checkout — authenticated.
			if deps.CheckoutExtHandler != nil {
				authed.POST("/checkout",
					deps.CheckoutExtHandler.Checkout)
			} else if deps.CheckoutHandler != nil {
				authed.POST("/checkout",
					deps.CheckoutHandler.Checkout)
			}

			// Orders.
			if deps.OrderDetailHandler != nil {
				authed.GET("/orders", deps.OrderDetailHandler.ListOrders)
				authed.GET("/orders/:id", deps.OrderDetailHandler.GetOrder)
			}

			// Customer account.
			if deps.CustomerAccountHandler != nil {
				authed.POST("/account/register", deps.CustomerAccountHandler.Register)
				authed.GET("/account/check-email", deps.CustomerAccountHandler.CheckEmail)
				authed.GET("/account", deps.CustomerAccountHandler.GetProfile)
				authed.PATCH("/account", deps.CustomerAccountHandler.UpdateProfile)
				authed.GET("/account/addresses", deps.CustomerAccountHandler.ListAddresses)
				authed.POST("/account/addresses", deps.CustomerAccountHandler.CreateAddress)
				authed.PATCH("/account/addresses/:id", deps.CustomerAccountHandler.UpdateAddress)
				authed.DELETE("/account/addresses/:id", deps.CustomerAccountHandler.DeleteAddress)
			}

			// Wishlists — C4.
			if deps.WishlistHandler != nil {
				authed.GET("/wishlist", deps.WishlistHandler.List)
				authed.POST("/wishlist", deps.WishlistHandler.Add)
				authed.DELETE("/wishlist/:productId", deps.WishlistHandler.Remove)
				authed.GET("/wishlist/check/:productId", deps.WishlistHandler.Check)
			}

			// Reviews — authenticated: submit + my reviews.
			if deps.ReviewsHandler != nil {
				authed.POST("/products/:handle/reviews",
					ratelimit.PerIP(0.167, 10),
					deps.ReviewsHandler.SubmitReview)
				authed.GET("/my-reviews", deps.ReviewsHandler.ListMyReviews)
			}

			// Loyalty — authenticated endpoints.
			if deps.LoyaltyHandler != nil {
				authed.GET("/loyalty/me", deps.LoyaltyHandler.GetMe)
				authed.POST("/loyalty/enroll",
					ratelimit.PerIP(0.167, 10),
					deps.LoyaltyHandler.Enroll)
				authed.POST("/loyalty/redeem",
					ratelimit.PerIP(0.167, 10),
					deps.LoyaltyHandler.Redeem)
			}

			// Coupons — validate.
			if deps.CouponValidateHandler != nil {
				authed.POST("/coupons/validate",
					ratelimit.PerIP(0.167, 10),
					deps.CouponValidateHandler.Validate)
			}

			// Gift cards — check balance.
			if deps.GiftCardHandler != nil {
				authed.POST("/gift-cards/check-balance",
					ratelimit.PerIP(0.167, 10),
					deps.GiftCardHandler.CheckBalance)
			}

			// Push tokens — mobile-specific.
			if deps.PushTokenHandler != nil {
				authed.POST("/push-tokens", deps.PushTokenHandler.Register)
				authed.DELETE("/push-tokens/:tokenId", deps.PushTokenHandler.Delete)
			}

			// Notify-me subscriptions — mobile-specific.
			if deps.NotifyMeHandler != nil {
				authed.POST("/products/:handle/notify-me", deps.NotifyMeHandler.Subscribe)
				authed.DELETE("/products/:handle/notify-me/:notifyType", deps.NotifyMeHandler.Unsubscribe)
			}
		}
	}
}

// mobileCustomerProfileMW resolves the GIP UID from context (set by
// GIPBearerAuth) into a customer profile via EnsureProfile. This bridges
// the Bearer token auth to the same customer context keys used by the
// cookie-based web storefront.
func mobileCustomerProfileMW(customerSvc *customer.Service, logger *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		gipUID := c.GetString(CustomerGipUIDKey)
		email := c.GetString(CustomerEmailKey)
		if gipUID == "" || email == "" {
			// No auth context — continue as guest.
			c.Next()
			return
		}

		storeVal, exists := c.Get("store")
		if !exists {
			c.Next()
			return
		}
		store, ok := storeVal.(*stores.Store)
		if !ok || store == nil {
			c.Next()
			return
		}

		storeID, err := uuid.Parse(store.ID)
		if err != nil {
			c.Next()
			return
		}
		tenantID, err := uuid.Parse(store.TenantID)
		if err != nil {
			c.Next()
			return
		}

		profile, err := customerSvc.EnsureProfile(c.Request.Context(), customer.EnsureProfileInput{
			StoreID:  storeID,
			TenantID: tenantID,
			GipUID:   gipUID,
			Email:    email,
		}, c)
		if err != nil {
			if logger != nil {
				logger.Error("mobile: failed to ensure customer profile",
					"error", err,
					"email", email,
					"store_id", store.ID,
				)
			}
			c.Next()
			return
		}

		c.Set(CustomerProfileIDKey, profile.ID.String())
		c.Set(CustomerEmailKey, profile.Email)
		c.Set(CustomerGipUIDKey, gipUID)
		c.Set(CustomerProfileKey, profile)
		c.Next()
	}
}
