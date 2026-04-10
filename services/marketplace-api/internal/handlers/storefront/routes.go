// Package storefront — routes.go: RegisterStorefront mounts the M6 public
// read routes on a caller-supplied router group. The caller owns the API
// version prefix (typically "/api/v1").
package storefront

import (
	"github.com/gin-gonic/gin"

	"github.com/mark8ly/marketplace-api/internal/stores"
)

// Deps groups every dependency the storefront route registrar needs.
// Constructed in cmd/marketplace-api/main.go.
type Deps struct {
	Handler         *StorefrontHandler
	CheckoutHandler *CheckoutHandler
	SlugCache       *stores.SlugCache
	StorefrontKey   string
	CountryHandler  CountryLister // optional — set when country handler is wired
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

		// Orders M5 — public checkout endpoint.
		if deps.CheckoutHandler != nil {
			group.POST("/checkout", deps.CheckoutHandler.Checkout)
		}
	}

	// Public reference data — no auth, no store context.
	if deps.CountryHandler != nil {
		router.GET("/public/supported-countries", deps.CountryHandler.ListSupported)
	}
}
