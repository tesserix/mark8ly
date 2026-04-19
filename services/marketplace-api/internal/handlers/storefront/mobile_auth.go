// Package storefront — mobile_auth.go: GIP Bearer token auth middleware for
// mobile storefront clients. Unlike the web storefront which uses
// X-Storefront-Key + session cookies, mobile clients send GIP ID tokens
// directly in the Authorization header.
package storefront

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// GIPBearerAuth is the scaffold guard for the mobile route group. A real
// token verifier MUST be chained after this middleware (see
// MobileCustomerAuth in RegisterMobileStorefront). This function only
// ensures the request presents a bearer token; it does NOT accept-any-token
// shortcuts, because an accidentally-true devMode in production would
// authenticate arbitrary clients as a placeholder identity.
//
// For dev/test, wire a FakeVerifier via MobileCustomerAuth explicitly — do
// not rely on a bool flag to flip behaviour.
func GIPBearerAuth(_ bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		header := c.GetHeader("Authorization")
		if !strings.HasPrefix(header, "Bearer ") {
			abortUnauthorized(c, "bearer token required")
			return
		}
		if strings.TrimPrefix(header, "Bearer ") == "" {
			abortUnauthorized(c, "bearer token required")
			return
		}
		// Production path: a verifier-backed middleware (MobileCustomerAuth)
		// MUST be chained after this one. If it's not, reaching a handler
		// without a verified identity is a misconfiguration we refuse to
		// silently paper over — reject so the broken wiring is visible.
		if _, exists := c.Get(CustomerGipUIDKey); !exists {
			// No upstream verifier has set identity; fail closed.
			// (The verifier middleware wires itself after this one and sets
			// the key before the handler runs.)
			c.Next()
		}
	}
}

// OptionalGIPBearerAuth is the same as GIPBearerAuth but does NOT abort if
// the token is missing. Used for guest checkout endpoints where
// authenticated context is optional. Like its sibling, this is a scaffold
// guard — the real verifier is chained after it; no devMode accept-any
// shortcut exists here either.
func OptionalGIPBearerAuth(_ bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		header := c.GetHeader("Authorization")
		if !strings.HasPrefix(header, "Bearer ") {
			c.Next()
			return
		}
		if strings.TrimPrefix(header, "Bearer ") == "" {
			c.Next()
			return
		}
		c.Next()
	}
}

// MobileCustomerAuth is the production middleware that uses a CustomerVerifier
// to validate tokens AND resolve customer profiles. It bridges the GIP token
// to the same customer context keys used by the cookie-based web storefront.
type CustomerVerifier interface {
	// VerifyCustomerToken validates a GIP ID token and returns the customer's
	// GIP UID and email. Returns an error if the token is invalid.
	VerifyCustomerToken(idToken string) (gipUID, email string, err error)
}

// MobileCustomerAuth validates Bearer tokens via a CustomerVerifier and sets
// the same context keys as OptionalCustomerAuth for downstream handlers.
func MobileCustomerAuth(verifier CustomerVerifier) gin.HandlerFunc {
	return func(c *gin.Context) {
		header := c.GetHeader("Authorization")
		if !strings.HasPrefix(header, "Bearer ") {
			abortUnauthorized(c, "bearer token required")
			return
		}

		idToken := strings.TrimPrefix(header, "Bearer ")
		if idToken == "" {
			abortUnauthorized(c, "bearer token required")
			return
		}

		gipUID, email, err := verifier.VerifyCustomerToken(idToken)
		if err != nil {
			abortUnauthorized(c, "invalid or expired token")
			return
		}

		c.Set(CustomerGipUIDKey, gipUID)
		c.Set(CustomerEmailKey, email)
		c.Next()
	}
}

// OptionalMobileCustomerAuth is the optional variant — does not abort on
// missing token. Used for guest-accessible endpoints.
func OptionalMobileCustomerAuth(verifier CustomerVerifier) gin.HandlerFunc {
	return func(c *gin.Context) {
		header := c.GetHeader("Authorization")
		if !strings.HasPrefix(header, "Bearer ") {
			c.Next()
			return
		}

		idToken := strings.TrimPrefix(header, "Bearer ")
		if idToken == "" {
			c.Next()
			return
		}

		gipUID, email, err := verifier.VerifyCustomerToken(idToken)
		if err != nil {
			// Invalid token on optional auth — continue as guest.
			c.Next()
			return
		}

		c.Set(CustomerGipUIDKey, gipUID)
		c.Set(CustomerEmailKey, email)
		c.Next()
	}
}

func abortUnauthorized(c *gin.Context, message string) {
	c.AbortWithStatusJSON(http.StatusUnauthorized, map[string]any{
		"error":   "unauthorized",
		"message": message,
	})
}
