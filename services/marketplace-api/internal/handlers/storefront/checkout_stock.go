package storefront

import (
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/mark8ly/marketplace-api/internal/checkoutcore"
)

// stockLinesFromItems selects the checkout items that consume inventory.
//
// An item with no variant_id is a custom or unstocked line — refusing those
// would break every unstocked sale, and there is nothing to decrement.
//
// Variant ids are lowercased here, at the boundary where lines are built.
// The legacy path passed the client's string straight into SQL, where
// Postgres casts to uuid case-insensitively; everything downstream of this
// function does Go map lookups against ids read back from the database in
// canonical lowercase, so an uppercase id from a client would otherwise miss
// every lookup and read as zero availability.
func stockLinesFromItems(items []CheckoutItemRequest) []checkoutcore.Line {
	lines := make([]checkoutcore.Line, 0, len(items))
	for _, it := range items {
		if it.VariantID == nil || *it.VariantID == "" || it.Quantity <= 0 {
			continue
		}
		lines = append(lines, checkoutcore.Line{VariantID: strings.ToLower(*it.VariantID), Quantity: it.Quantity})
	}
	return lines
}

// cartTokenForCheckout returns the cart identity to commit holds against,
// and to claim the buyer's personalisation uploads with (#967).
//
// The web storefront carries it in the mk_cart_token cookie, set by the
// Next proxy. A native app has no cookie jar it can rely on, so the mobile
// storefront sends the same token in the request body instead (#969) —
// the way the upload endpoints already accept it, see cartTokenFrom in
// personalisation_uploads.go. Cookie first: a web client never sends the
// body field, and a client that sends both is telling us the same thing
// twice.
//
// A checkout arriving with neither — an API client, or a storefront build
// predating #232 — gets a fresh token, so the hold-then-commit path still
// enforces availability for it. Enforcement must not depend on the caller
// having cooperated.
func cartTokenForCheckout(c *gin.Context, fromBody *string) string {
	if ck, err := c.Cookie(CartTokenCookie); err == nil {
		if _, perr := uuid.Parse(ck); perr == nil {
			return ck
		}
	}
	if fromBody != nil {
		if _, perr := uuid.Parse(*fromBody); perr == nil {
			return *fromBody
		}
	}
	return uuid.NewString()
}
