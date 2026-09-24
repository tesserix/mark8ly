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

		// `message` is part of the shape every admin client expects
		// (apps/admin ApiError is {error, message, details?}). Omitting it
		// made every one of the 23 server-side fetch helpers render
		// `402: unknown error`, because they read errBody?.message and
		// fell back. The status stays a separate machine-readable field;
		// the message is for the human reading a log or a toast.
		c.AbortWithStatusJSON(http.StatusPaymentRequired, gin.H{
			"error":   "subscription_inactive",
			"status":  string(status),
			"message": readOnlyMessage(status),
		})
	}
}

func routeAllowed(c *gin.Context, allowlist []AllowedRoute) bool {
	// Matched against the path from "/admin/" onward, NOT against
	// c.FullPath() directly.
	//
	// Production mounts this group under a base prefix —
	// admin.RegisterAdmin(r.Group("/api/v1"), ...) — so FullPath() is
	// "/api/v1/admin/stores/:storeId/orders". Every pattern in
	// DefaultAllowlist is written as "/admin/...", and the view-only rule
	// used HasPrefix(FullPath(), "/admin/"). Neither could ever match.
	//
	// The effect was total rather than partial. A merchant in expired,
	// store_closed or pending_hard_delete lost every GET (the view-only
	// rule), order export, the tax-ID fix, auth — and, worst, POST
	// subscription and POST billing, which are the routes this package's
	// own doc comment calls out as "always allowed so merchants can
	// recover". An expired trial had no in-product way back to paying.
	//
	// It survived because every test in this package mounted its routes at
	// "/admin/..." with no base group, so the suite asserted the behaviour
	// of a path shape production never produces.
	admin := adminSuffix(c.FullPath())
	if admin == "" {
		return false
	}

	// All GET /admin/** are always allowed (view-only).
	if c.Request.Method == http.MethodGet {
		return true
	}
	for _, r := range allowlist {
		if r.Method != "" && r.Method != c.Request.Method {
			continue
		}
		if patternMatches(r.Pattern, admin) {
			return true
		}
	}
	return false
}

// adminSuffix returns the route path from its "/admin/" segment onward, or ""
// when the route is not under an admin group.
//
// Prefix-agnostic on purpose: the allowlist should keep working if the group
// is ever remounted under a different base, and a rule that silently stops
// matching is exactly what this function exists to stop repeating.
//
// Segment-anchored, so "/api/v1/store-admin/x" does NOT match — the leading
// slash in the search string is load-bearing.
func adminSuffix(full string) string {
	const marker = "/admin/"
	idx := strings.Index(full, marker)
	if idx < 0 {
		return ""
	}
	return full[idx:]
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

// readOnlyMessage explains, in one sentence a merchant could read, why the
// request was refused. Kept next to the statuses rather than in the client so
// every consumer -- browser, server component, mobile, a curl in an incident
// -- gets the same explanation.
func readOnlyMessage(status subscription.SubscriptionStatus) string {
	switch status {
	case subscription.StatusExpired:
		return "your trial has ended — this store is read-only until a card is added"
	case subscription.StatusStoreClosed:
		return "this store is closed and read-only until a card is added"
	case subscription.StatusPendingHardDelete:
		return "this store is scheduled for deletion and is read-only"
	default:
		return "this store's subscription is not active — it is read-only"
	}
}
