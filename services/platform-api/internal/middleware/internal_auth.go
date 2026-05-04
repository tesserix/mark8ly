// Package middleware holds platform-api's HTTP middleware. The package
// only carries the small generic gates (internal-auth, etc.); domain
// auth lives next to the handler that owns it.
package middleware

import (
	"crypto/sha256"
	"crypto/subtle"
	"net/http"

	"github.com/gin-gonic/gin"
)

// RequireInternalAuth gates a route group on the shared X-Internal-Auth
// header. In-cluster callers (admin BFF, auth-bff, marketplace-api) carry
// the header; anything reaching the route from outside the cluster — direct
// pod port-forwards, lateral pod traffic with no sidecar enforcement — is
// rejected.
//
// An empty secret no-ops the middleware. That keeps local dev working and
// makes the cutover safe: ship the binary first, populate the secret +
// caller headers next, and only then will real traffic require the header.
//
// The marketplace-api / auth-bff / otto services use the identical
// pattern; this exists so platform-api can match them without depending
// on the marketplace-api package.
func RequireInternalAuth(secret string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if secret == "" {
			c.Next()
			return
		}
		if !constantTimeEqual(c.GetHeader("X-Internal-Auth"), secret) {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error":   "unauthorized",
				"message": "internal auth required",
			})
			return
		}
		c.Next()
	}
}

func constantTimeEqual(got, want string) bool {
	if got == "" || want == "" {
		return false
	}
	gotSum := sha256.Sum256([]byte(got))
	wantSum := sha256.Sum256([]byte(want))
	return subtle.ConstantTimeCompare(gotSum[:], wantSum[:]) == 1
}
