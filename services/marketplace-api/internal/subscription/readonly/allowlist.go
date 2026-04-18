// Package readonly provides the RequireActive Gin middleware that returns
// 402 Payment Required for admin routes when a subscription is in a read-only
// state (expired | store_closed | pending_hard_delete). Billing + subscription
// + order-export + auth routes are always allowed so merchants can recover.
package readonly

import "net/http"

// AllowedRoute identifies a Gin route by method + FullPath pattern.
type AllowedRoute struct {
	Method  string
	Pattern string
}

// DefaultAllowlist encodes §17.3. Patterns use Gin tokens (:storeId, *path).
// A separate rule in middleware.go allows every GET /admin/** through (view-only).
var DefaultAllowlist = []AllowedRoute{
	{http.MethodPost, "/admin/stores/:storeId/subscription/*path"},
	{http.MethodPost, "/admin/stores/:storeId/billing/*path"},
	{http.MethodGet, "/admin/stores/:storeId/orders/export/*path"},
	{http.MethodPost, "/admin/auth/*path"},
	{http.MethodGet, "/admin/stores/:storeId/migration-fast-path/*path"},
	// §4.7: merchants in any terminal state must be able to reach the Stripe
	// hosted invoice page to pay and recover their subscription.
	{http.MethodGet, "/admin/stores/:storeId/subscription/complete-action"},
}
