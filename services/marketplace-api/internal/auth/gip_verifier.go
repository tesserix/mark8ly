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

// Verify checks the token signature and extracts the user_id claim.
//
// It no longer extracts tenant_id. That custom claim used to be the only
// source of tenancy for GIP tokens, but it competed — unvalidated — with
// auth.TenantFromRequest's FGA-checked value for the same gin context key
// (#524 phase 4). Tenancy is now decided exclusively by TenantFromRequest,
// so nothing here needs to read the claim at all, regardless of whether a
// given token happens to carry one.
func (v *GIPVerifier) Verify(ctx context.Context, idToken string) (*TokenClaims, error) {
	token, err := v.client.VerifyIDToken(ctx, idToken)
	if err != nil {
		return nil, fmt.Errorf("verify GIP token: %w", err)
	}

	userID := token.UID
	if userID == "" {
		return nil, ErrInvalidToken
	}

	return &TokenClaims{
		UserID: userID,
	}, nil
}
