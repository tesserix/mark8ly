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
// TenantID is populated only by GIPVerifier — ZitadelVerifier always
// returns it empty by design (see zitadel_verifier.go), because Zitadel
// tokens carry no tenant claim at all. Whether GIPBearerAuth actually
// copies TenantID onto the gin context's "tenant_id" key is controlled by
// its setTenantFromClaim parameter, NOT by this type: exactly one of
// {GIPBearerAuth's claim write, TenantFromRequest's FGA-validated write}
// may be active for a given deployment, selected by ZITADEL_ENABLED. Never
// let both run — an unvalidated claim racing a validated FGA result for
// the same context key is the bug #524 phase 4 exists to remove. See
// internal/handlers/admin/mobile_routes.go for how the two are kept
// mutually exclusive.
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
// On success it always sets "user_id" on the gin context — same contract
// as HeaderTrustAuth so downstream handlers work unchanged.
//
// setTenantFromClaim controls whether it ALSO sets "tenant_id" from the
// verified claims:
//
//   - false (Zitadel deployments): tenancy comes exclusively from
//     TenantFromRequest's FGA-validated result, mounted later in the
//     chain. Since ZitadelVerifier always returns an empty TenantID
//     anyway, this is belt-and-braces, not load-bearing on its own.
//   - true (GIP deployments — today's production, ZITADEL_ENABLED=false):
//     TenantFromRequest is never mounted (there is no X-Acting-Tenant-Id
//     support anywhere outside this service, so mounting it would 404
//     every mobile-admin request), so the GIP custom claim is the only
//     source of tenancy, exactly as before #524 phase 4.
//
// The caller (RegisterAdminMobile) MUST pass setTenantFromClaim as the
// exact complement of whether it also mounts TenantFromRequest — never
// both active, never both inactive — so exactly one writer of "tenant_id"
// is ever active.
func GIPBearerAuth(verifier TokenVerifier, setTenantFromClaim bool) gin.HandlerFunc {
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
		if setTenantFromClaim {
			c.Set("tenant_id", claims.TenantID)
		}
		c.Next()
	}
}
