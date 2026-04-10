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

// GIPBearerAuth validates Firebase/GIP Bearer tokens for mobile clients.
// Extracts "Authorization: Bearer <token>" header.
// In devMode, accepts any non-empty token and sets dev context values.
// In production, delegates to a verifier function (wired separately).
// Sets context keys: "customer_gip_uid", "customer_email".
// Returns 401 if missing or invalid.
func GIPBearerAuth(devMode bool) gin.HandlerFunc {
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

		if devMode {
			// Dev mode: accept any non-empty token, set placeholder context.
			c.Set(CustomerGipUIDKey, "dev-uid")
			c.Set(CustomerEmailKey, "dev@example.com")
			c.Next()
			return
		}

		// Production path: the mobile route group wires a MobileCustomerAuth
		// middleware (see RegisterMobileStorefront) that uses a real verifier.
		// This standalone function only handles the dev-mode shortcut. In
		// production, callers chain a verifier-backed middleware instead.
		//
		// If we reach here, it means the caller misconfigured the middleware
		// chain — no verifier was provided and devMode is false.
		abortUnauthorized(c, "token verification not configured")
	}
}

// OptionalGIPBearerAuth is the same as GIPBearerAuth but does NOT abort if
// the token is missing. Used for guest checkout endpoints where
// authenticated context is optional.
func OptionalGIPBearerAuth(devMode bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		header := c.GetHeader("Authorization")
		if !strings.HasPrefix(header, "Bearer ") {
			// No token — continue as guest.
			c.Next()
			return
		}

		idToken := strings.TrimPrefix(header, "Bearer ")
		if idToken == "" {
			// Empty bearer — continue as guest.
			c.Next()
			return
		}

		if devMode {
			c.Set(CustomerGipUIDKey, "dev-uid")
			c.Set(CustomerEmailKey, "dev@example.com")
			c.Next()
			return
		}

		// Production: no-op here, verifier-backed middleware handles it.
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
