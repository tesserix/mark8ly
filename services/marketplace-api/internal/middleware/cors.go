package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// CORS returns middleware that handles Cross-Origin Resource Sharing for
// the storefront engine. Admin uses same-origin and does not need CORS.
//
// allowedOrigins is a comma-separated list of allowed origins. Supports
// wildcard subdomains (e.g. "https://*.mark8ly.com").
func CORS(allowedOrigins string) gin.HandlerFunc {
	origins := strings.Split(allowedOrigins, ",")
	for i := range origins {
		origins[i] = strings.TrimSpace(origins[i])
	}

	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		if origin == "" {
			c.Next()
			return
		}

		if matchOrigin(origin, origins) {
			c.Header("Access-Control-Allow-Origin", origin)
			c.Header("Access-Control-Allow-Methods", "GET, POST, PATCH, PUT, DELETE, OPTIONS")
			c.Header("Access-Control-Allow-Headers", "Content-Type, X-Storefront-Key, Authorization")
			c.Header("Access-Control-Allow-Credentials", "true")
			c.Header("Access-Control-Max-Age", "43200") // 12 hours
			c.Header("Vary", "Origin")
		}

		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		c.Next()
	}
}

func matchOrigin(origin string, allowed []string) bool {
	for _, a := range allowed {
		// We never honour a bare "*" — the response sets
		// Access-Control-Allow-Credentials: true, and pairing
		// credentials with a wildcard origin would let any site read
		// authenticated responses. Operators must use explicit hosts
		// or "https://*.<domain>" wildcard-subdomain entries.
		if a == "" || a == "*" {
			continue
		}
		if a == origin {
			return true
		}
		// Wildcard subdomain match: "https://*.mark8ly.com"
		if strings.HasPrefix(a, "https://*.") {
			suffix := a[len("https://*"):]                  // ".mark8ly.com"
			if strings.HasPrefix(origin, "https://") &&
				strings.HasSuffix(origin, suffix) &&
				len(origin) > len("https://")+len(suffix) {
				return true
			}
		}
	}
	return false
}
