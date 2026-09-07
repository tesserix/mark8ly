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
// TenantID is NEVER a source of tenancy. ZitadelVerifier — the only
// verifier wired into the mobile admin group (#786) — always returns it
// empty by design (see zitadel_verifier.go), and BearerAuth ignores the
// field regardless of what a verifier puts there: tenancy comes
// exclusively from TenantFromRequest's FGA-validated result, mounted
// later in the chain. The field survives so that a verifier CAN surface a
// token's own tenant assertion, and so bearer_test.go can prove such an
// assertion never reaches the gin context — an unvalidated claim racing a
// validated FGA result for the same context key is the bug #524 phase 4
// exists to remove. See internal/handlers/admin/mobile_routes.go for the
// chain ordering that keeps TenantFromRequest the single writer.
type TokenClaims struct {
	UserID   string
	TenantID string
}

// TokenVerifier verifies a bearer ID token and returns its claims.
// In production this is ZitadelVerifier; in tests a FakeVerifier.
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

// BearerAuth returns a gin middleware that validates a Bearer token with
// the supplied verifier. It is provider-agnostic: it verifies whatever
// TokenVerifier it is handed and knows nothing about the issuer.
//
// On success it sets "user_id" on the gin context — same contract as
// HeaderTrustAuth, so downstream handlers work unchanged — and NOTHING
// else. In particular it never writes "tenant_id": tenancy is resolved
// exclusively by auth.TenantFromRequest, which validates the caller's
// stated tenant against real FGA membership before writing it. Adding a
// second, unvalidated writer here would reopen the race #524 phase 4
// closed.
func BearerAuth(verifier TokenVerifier) gin.HandlerFunc {
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
