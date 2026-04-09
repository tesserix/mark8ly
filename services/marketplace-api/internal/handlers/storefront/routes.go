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
	Handler       *StorefrontHandler
	SlugCache     *stores.SlugCache
	StorefrontKey string
}

// RegisterStorefront mounts the 4 storefront routes on the given router
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
	}
}
