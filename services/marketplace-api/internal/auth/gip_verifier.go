package auth

import (
	"context"
	"fmt"

	firebaseAuth "firebase.google.com/go/v4/auth"
)

// GIPVerifier verifies GIP ID tokens using the Firebase Admin Auth client.
type GIPVerifier struct {
	client *firebaseAuth.Client
}

// NewGIPVerifier creates a GIPVerifier from a Firebase Auth client.
func NewGIPVerifier(client *firebaseAuth.Client) *GIPVerifier {
	return &GIPVerifier{client: client}
}

// Verify checks the token signature and extracts user_id and tenant_id claims.
func (v *GIPVerifier) Verify(ctx context.Context, idToken string) (*TokenClaims, error) {
	token, err := v.client.VerifyIDToken(ctx, idToken)
	if err != nil {
		return nil, fmt.Errorf("verify GIP token: %w", err)
	}

	userID := token.UID
	if userID == "" {
		return nil, ErrInvalidToken
	}

	tenantID := tenantIDFromClaims(token.Claims)
	if tenantID == "" {
		return nil, fmt.Errorf("token missing tenant_id claim")
	}

	return &TokenClaims{
		UserID:   userID,
		TenantID: tenantID,
	}, nil
}

// tenantIDFromClaims reads the tenant_id custom claim set on the GIP user
// record during onboarding.
//
// tenant-service writes it as a multi-value array (its UpdateUserAttributes
// appends into an []interface{} so a user can be added to further tenants), so
// the array form is what production tokens carry. The scalar form is accepted
// too, so a token from any other writer still verifies.
//
// A user may belong to several tenants. Mobile has no tenant switcher, so the
// first usable entry wins; revisit if per-tenant selection is ever added.
func tenantIDFromClaims(claims map[string]interface{}) string {
	switch v := claims["tenant_id"].(type) {
	case string:
		return v
	case []interface{}:
		for _, item := range v {
			if s, ok := item.(string); ok && s != "" {
				return s
			}
		}
	case []string:
		for _, s := range v {
			if s != "" {
				return s
			}
		}
	}
	return ""
}
