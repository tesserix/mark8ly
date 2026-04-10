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

	// tenant_id is stored as a custom claim by auth-bff during login.
	tenantID, _ := token.Claims["tenant_id"].(string)
	if tenantID == "" {
		return nil, fmt.Errorf("token missing tenant_id claim")
	}

	return &TokenClaims{
		UserID:   userID,
		TenantID: tenantID,
	}, nil
}
