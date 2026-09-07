// Package public implements unauthenticated / pre-auth public endpoints.
// Routes are registered by RegisterPublic onto a caller-supplied
// *gin.RouterGroup — no auth middleware is applied at this layer; individual
// handlers validate identity themselves.
package public

import (
	"github.com/gin-gonic/gin"
)

// PublicDeps groups every dependency the public route registrar needs.
// Constructed in cmd/marketplace-api/main.go.
type PublicDeps struct {
	// DelhiveryWebhookHandler receives post-scan webhooks from
	// Delhivery. Nil-safe — when absent the route is simply not
	// mounted, which keeps merchants on polling-only.
	DelhiveryWebhookHandler *DelhiveryWebhookHandler
	// JournalSubscribeHandler serves the mark8ly.com Journal "coming
	// soon" page's email capture (#153). Nil-safe, like the others —
	// absent in test builds that don't wire a DB.
	JournalSubscribeHandler *JournalSubscribeHandler
	// JournalUnsubscribeHandler is the erasure counterpart to
	// JournalSubscribeHandler: it deletes a subscriber row by bearer
	// token (migration 000125). Nil-safe, like the others.
	JournalUnsubscribeHandler *JournalUnsubscribeHandler
}

// RegisterPublic mounts all public (unauthenticated) routes onto the given
// router group. Callers typically pass the root group so routes are served
// at the top level without an /api/v1 prefix.
func RegisterPublic(router *gin.RouterGroup, deps PublicDeps) {
	if deps.DelhiveryWebhookHandler != nil {
		// Mount under /carrier-webhooks/:provider rather than
		// /webhooks/:provider because the storefront already owns
		// `/webhooks/:provider` for payment gateway callbacks
		// (Razorpay, Stripe, etc). Gin rejects a literal path
		// segment alongside a wildcard at the same tree depth, so
		// putting shipping webhooks on a separate root keeps both
		// surfaces working without either cannibalising the other.
		router.POST("/carrier-webhooks/delhivery", deps.DelhiveryWebhookHandler.Handle)
	}
	if deps.JournalSubscribeHandler != nil {
		// Deliberately NOT under /storefront or /admin: a journal
		// subscriber is a platform-level marketing record with no
		// tenant_id (see migrations/000124), so it has no business
		// behind TenantMiddleware or any per-tenant route group. This
		// group — the same one the Delhivery webhook already shares —
		// is the one genuinely tenant-free public surface this router
		// exposes.
		router.POST("/journal/subscribe", deps.JournalSubscribeHandler.Subscribe)
	}
	if deps.JournalUnsubscribeHandler != nil {
		// Same tenant-free group, same reasoning as subscribe above:
		// an unsubscribe token has no tenant to route through either.
		router.POST("/journal/unsubscribe", deps.JournalUnsubscribeHandler.Unsubscribe)
	}
}
