package storefront

import (
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/mark8ly/marketplace-api/internal/stores"
)

// cacheControlValue is the shared Cache-Control header value applied to
// every successful storefront read. 60s shared-cache TTL with 5m
// stale-while-revalidate — tuned for a Cloudflare Worker in front.
const cacheControlValue = "public, s-maxage=60, stale-while-revalidate=300"

// buildETag returns the weak ETag for a store's current products
// watermark. Format: W/"<store_id>-<unix_ms>".
func buildETag(store *stores.Store, watermark time.Time) string {
	return fmt.Sprintf(`W/"%s-%d"`, store.ID, watermark.UnixMilli())
}

// setCacheHeaders writes Cache-Control, ETag, Last-Modified, and Vary on
// a successful response. Must be called BEFORE c.JSON.
func setCacheHeaders(c *gin.Context, store *stores.Store, watermark time.Time) {
	c.Header("Cache-Control", cacheControlValue)
	c.Header("ETag", buildETag(store, watermark))
	c.Header("Last-Modified", watermark.UTC().Format(http.TimeFormat))
	c.Header("Vary", "Accept-Encoding, X-Storefront-Key")
}

// checkIfNoneMatch returns true and writes 304 if the client's
// If-None-Match header matches the current ETag. Handlers must
// short-circuit when this returns true.
func checkIfNoneMatch(c *gin.Context, store *stores.Store, watermark time.Time) bool {
	wantETag := buildETag(store, watermark)
	if c.GetHeader("If-None-Match") == wantETag {
		c.Header("Cache-Control", cacheControlValue)
		c.Header("ETag", wantETag)
		c.Status(http.StatusNotModified)
		return true
	}
	return false
}
