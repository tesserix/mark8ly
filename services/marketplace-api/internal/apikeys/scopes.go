package apikeys

import (
	"fmt"
	"net/http"
	"slices"

	"github.com/gin-gonic/gin"
)

// Scope is a canonical permission string. Format is "<resource>:<verb>".
// Routes declare a required Scope; the auth middleware compares it against
// the key's ScopeSet after bcrypt verify succeeds.
type Scope string

const (
	ScopeProductsRead    Scope = "products:read"
	ScopeProductsWrite   Scope = "products:write"
	ScopeOrdersRead      Scope = "orders:read"
	ScopeOrdersWrite     Scope = "orders:write"
	ScopeCustomersRead   Scope = "customers:read"
	ScopeCustomersWrite  Scope = "customers:write"
	ScopeCategoriesRead  Scope = "categories:read"
	ScopeCategoriesWrite Scope = "categories:write"
	ScopeCouponsRead     Scope = "coupons:read"
	ScopeCouponsWrite    Scope = "coupons:write"
)

// AllScopes returns the canonical v1 scope list. Adding a new scope here
// also unblocks ValidateScopes for that string.
func AllScopes() []Scope {
	return []Scope{
		ScopeProductsRead, ScopeProductsWrite,
		ScopeOrdersRead, ScopeOrdersWrite,
		ScopeCustomersRead, ScopeCustomersWrite,
		ScopeCategoriesRead, ScopeCategoriesWrite,
		ScopeCouponsRead, ScopeCouponsWrite,
	}
}

// ValidateScopes returns nil iff every entry in `requested` is a known scope.
// Used at key creation — never reject at request time (a key with an unknown
// scope simply can't satisfy any RequireScope check).
func ValidateScopes(requested []string) error {
	known := AllScopes()
	for _, s := range requested {
		if !slices.Contains(known, Scope(s)) {
			return fmt.Errorf("apikeys: unknown scope %q", s)
		}
	}
	return nil
}

// IsReadOnlyScope reports whether the scope grants read-only access. Used by
// the service layer to enforce Studio's "read-only keys only" plan ceiling.
func IsReadOnlyScope(s string) bool {
	switch Scope(s) {
	case ScopeProductsRead, ScopeOrdersRead, ScopeCustomersRead,
		ScopeCategoriesRead, ScopeCouponsRead:
		return true
	}
	return false
}

// AllReadOnly reports whether every scope in the slice is read-only.
func AllReadOnly(scopes []string) bool {
	for _, s := range scopes {
		if !IsReadOnlyScope(s) {
			return false
		}
	}
	return true
}

// RequireScope returns Gin middleware that checks the authenticated key
// carries `required`. Mount it on individual routes after Authenticate. The
// 403 body never identifies which scope was required (defense-in-depth).
func RequireScope(required Scope) gin.HandlerFunc {
	return func(c *gin.Context) {
		raw, ok := c.Get("api_key_scopes")
		if !ok {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			return
		}
		scopes, ok := raw.([]string)
		if !ok {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "scope_required"})
			return
		}
		if !slices.Contains(scopes, string(required)) {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"error":   "scope_required",
				"message": fmt.Sprintf("This endpoint requires the %q scope.", required),
			})
			return
		}
		c.Next()
	}
}
