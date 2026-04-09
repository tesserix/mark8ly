package storefront

import (
	"context"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/mark8ly/marketplace-api/internal/stores"
	"github.com/mark8ly/marketplace-api/pkg/apperrors"
)

// RequireStorefrontKey returns a middleware that rejects requests missing
// or mismatching X-Storefront-Key. When secret is empty the middleware is
// a no-op — used for local dev and tests.
func RequireStorefrontKey(secret string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if secret == "" {
			c.Next()
			return
		}
		if c.GetHeader("X-Storefront-Key") != secret {
			respondNotFound(c)
			return
		}
		c.Next()
	}
}

// SlugLookup is the narrow read contract StoreContext needs. Any type
// that satisfies Get(ctx, slug) can be supplied — production wiring
// passes a *stores.SlugCache; tests inject fakes.
type SlugLookup interface {
	Get(ctx context.Context, slug string) (*stores.Store, error)
}

// StoreContext resolves the :storeSlug path param to a store row via the
// SlugCache. Sets the resolved store on the gin context under key "store".
// Returns 404 on miss / suspended / archived — no existence leak — and
// 500 on unexpected cache errors.
func StoreContext(cache SlugLookup) gin.HandlerFunc {
	return func(c *gin.Context) {
		slug := c.Param("storeSlug")
		if slug == "" {
			respondNotFound(c)
			return
		}
		store, err := cache.Get(c.Request.Context(), slug)
		if err != nil {
			if !errors.Is(err, stores.ErrNotFound) {
				c.AbortWithStatusJSON(http.StatusInternalServerError, map[string]any{
					"error":   "internal",
					"message": "internal server error",
				})
				return
			}
			respondNotFound(c)
			return
		}
		if store == nil || store.Status != stores.StatusActive {
			respondNotFound(c)
			return
		}
		c.Set("store", store)
		c.Next()
	}
}

func respondNotFound(c *gin.Context) {
	c.AbortWithStatusJSON(http.StatusNotFound, map[string]any{
		"error":   string(apperrors.CodeNotFound),
		"message": "not found",
	})
}
