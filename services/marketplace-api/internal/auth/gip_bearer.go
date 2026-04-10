package auth

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

var (
	ErrNoToken      = errors.New("no bearer token")
	ErrInvalidToken = errors.New("invalid token")
)

// TokenClaims holds the verified claims extracted from a GIP ID token.
type TokenClaims struct {
	UserID   string
	TenantID string
}

// TokenVerifier verifies a GIP ID token and returns its claims.
// In production this wraps Firebase Admin SDK; in tests a FakeVerifier.
type TokenVerifier interface {
	Verify(ctx context.Context, idToken string) (*TokenClaims, error)
}

// FakeVerifier is a test double for TokenVerifier.
type FakeVerifier struct {
	UserID   string
	TenantID string
	Err      error
}

func (f *FakeVerifier) Verify(_ context.Context, _ string) (*TokenClaims, error) {
	if f.Err != nil {
		return nil, f.Err
	}
	return &TokenClaims{UserID: f.UserID, TenantID: f.TenantID}, nil
}

// GIPBearerAuth returns a gin middleware that validates a GIP Bearer token.
// On success it sets "user_id" and "tenant_id" on the gin context — same
// contract as HeaderTrustAuth so downstream handlers work unchanged.
func GIPBearerAuth(verifier TokenVerifier) gin.HandlerFunc {
	return func(c *gin.Context) {
		header := c.GetHeader("Authorization")
		if !strings.HasPrefix(header, "Bearer ") {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error":   "unauthorized",
				"message": "bearer token required",
			})
			return
		}
		idToken := strings.TrimPrefix(header, "Bearer ")

		claims, err := verifier.Verify(c.Request.Context(), idToken)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error":   "unauthorized",
				"message": "invalid or expired token",
			})
			return
		}

		c.Set("user_id", claims.UserID)
		c.Set("tenant_id", claims.TenantID)
		c.Next()
	}
}
