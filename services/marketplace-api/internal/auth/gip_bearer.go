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

// TokenClaims holds the verified claims extracted from a bearer token.
//
// It carries no tenant information. Tenancy used to travel here as a
// GIP custom claim, but that let an unvalidated, caller-controlled value
// compete with auth.TenantFromRequest's FGA-validated tenant_id for the
// same gin context key — whichever middleware ran last silently won. The
// claim path is gone: the only way tenant_id reaches the context now is
// TenantFromRequest, after an FGA membership check. See
// internal/auth/tenant_from_request.go.
type TokenClaims struct {
	UserID string
}

// TokenVerifier verifies a GIP ID token and returns its claims.
// In production this wraps Firebase Admin SDK; in tests a FakeVerifier.
type TokenVerifier interface {
	Verify(ctx context.Context, idToken string) (*TokenClaims, error)
}

// FakeVerifier is a test double for TokenVerifier.
type FakeVerifier struct {
	UserID string
	Err    error
}

func (f *FakeVerifier) Verify(_ context.Context, _ string) (*TokenClaims, error) {
	if f.Err != nil {
		return nil, f.Err
	}
	return &TokenClaims{UserID: f.UserID}, nil
}

// GIPBearerAuth returns a gin middleware that validates a GIP Bearer token.
// On success it sets "user_id" on the gin context — same contract as
// HeaderTrustAuth so downstream handlers work unchanged. It deliberately
// does NOT set "tenant_id": tenancy comes only from TenantFromRequest's
// FGA-validated result, mounted later in the chain. Setting it here too
// would give an unvalidated claim value a chance to win a race against the
// validated one, depending on middleware order — exactly the bug this
// change removes.
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
		c.Next()
	}
}
