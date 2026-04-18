package readonly

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/mark8ly/marketplace-api/internal/subscription"
	"github.com/mark8ly/marketplace-api/internal/subscription/statemachine"
)

// Config wires RequireActive into the admin router.
// The middleware expects StoreMiddleware to have already written the current
// subscription status to cfg.StatusContextKey on the Gin context.
type Config struct {
	// StatusContextKey — default "subscription_status".
	StatusContextKey string
	// Allowlist — default DefaultAllowlist.
	Allowlist []AllowedRoute
}

// RequireActive returns a Gin middleware that blocks read-only subscriptions
// on non-allowlisted admin routes with 402 Payment Required.
func RequireActive(cfg Config) gin.HandlerFunc {
	if cfg.StatusContextKey == "" {
		cfg.StatusContextKey = "subscription_status"
	}
	if cfg.Allowlist == nil {
		cfg.Allowlist = DefaultAllowlist
	}

	return func(c *gin.Context) {
		raw, ok := c.Get(cfg.StatusContextKey)
		if !ok {
			c.Next()
			return
		}
		status, _ := raw.(subscription.SubscriptionStatus)

		if !statemachine.IsReadOnly(status) {
			c.Next()
			return
		}
		if routeAllowed(c, cfg.Allowlist) {
			c.Next()
			return
		}

		c.AbortWithStatusJSON(http.StatusPaymentRequired, gin.H{
			"error":  "subscription_inactive",
			"status": string(status),
		})
	}
}

func routeAllowed(c *gin.Context, allowlist []AllowedRoute) bool {
	// All GET /admin/** are always allowed (view-only).
	if c.Request.Method == http.MethodGet && strings.HasPrefix(c.FullPath(), "/admin/") {
		return true
	}
	full := c.FullPath()
	for _, r := range allowlist {
		if r.Method != "" && r.Method != c.Request.Method {
			continue
		}
		if patternMatches(r.Pattern, full) {
			return true
		}
	}
	return false
}

// patternMatches supports exact match + a trailing `*path` wildcard. §17.3
// allowlist doesn't need more sophisticated matching. If we add routes that
// need interior wildcards, swap this for gin's internal tree matcher.
func patternMatches(pattern, full string) bool {
	if pattern == full {
		return true
	}
	if strings.HasSuffix(pattern, "*path") {
		prefix := strings.TrimSuffix(pattern, "*path")
		return strings.HasPrefix(full, prefix)
	}
	return false
}
