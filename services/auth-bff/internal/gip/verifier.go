// Package gip is the auth-bff Google Identity Platform integration.
//
// Multi-tenant aware: GIP issues per-tenant ID tokens that include a
// firebase.tenant claim. The verifier rejects tokens whose tenant claim
// does not match the configured pool — this is the fix for auth-bug #5
// from docs/planning/auth-bugs.md.
//
// Implementation note: we do NOT use the Firebase Admin SDK. The SDK
// requires Application Default Credentials (a service account or
// `gcloud auth application-default login`), which adds friction to local
// dev and is unnecessary for our use case.
//
// JWT signature verification only needs Google's public keys, which are
// served unauthenticated at:
//
//	https://www.googleapis.com/service_accounts/v1/jwk/securetoken@system.gserviceaccount.com
//
// We fetch + cache the JWKS, verify the JWT signature, check standard
// claims (iss, aud, exp, iat, nbf), and explicitly compare the
// `firebase.tenant` custom claim against the expected pool. This is
// exactly what the SDK does internally for verification — minus 25
// transitive dependencies and the ADC bootstrap.
package gip

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/MicahParks/keyfunc/v3"
	"github.com/golang-jwt/jwt/v5"
)

// VerifiedToken is the relevant subset of a verified GIP ID token.
type VerifiedToken struct {
	UID      string // GIP user ID (sub claim)
	Email    string
	TenantID string // GIP tenant pool the token was issued from
}

// Verifier verifies a GIP ID token bearer string and returns the
// VerifiedToken on success.
type Verifier interface {
	VerifyToken(ctx context.Context, idToken, expectedTenantID string) (*VerifiedToken, error)
}

// Errors returned by Verifier implementations.
var (
	ErrTokenInvalid   = errors.New("gip: token invalid or expired")
	ErrTenantMismatch = errors.New("gip: token tenant does not match expected pool")
)

// jwksURL is Google's public JWKS endpoint for Firebase/GIP ID tokens.
// Anyone can fetch it; no authentication required.
const jwksURL = "https://www.googleapis.com/service_accounts/v1/jwk/securetoken@system.gserviceaccount.com"

// jwtIssuerPrefix is the issuer prefix for GIP-issued ID tokens. The full
// issuer is "https://securetoken.google.com/<projectID>".
const jwtIssuerPrefix = "https://securetoken.google.com/"

// jwtVerifier is the keyfunc-backed implementation that verifies tokens
// against Google's public JWKS.
type jwtVerifier struct {
	projectID string
	keys      keyfunc.Keyfunc
}

// Config holds the values needed to construct a Verifier.
type Config struct {
	ProjectID string // GCP project ID — must match the JWT iss/aud claims
}

// New constructs a Verifier wired to Google's public JWKS endpoint.
//
// JWKS is fetched lazily on first verification (and cached + refreshed
// automatically by the keyfunc library). No GCP credentials needed.
func New(ctx context.Context, cfg Config) (Verifier, error) {
	if cfg.ProjectID == "" {
		return nil, fmt.Errorf("gip: ProjectID is required")
	}

	// keyfunc handles JWKS fetch, parsing, key rotation, and re-fetch on
	// kid-not-found. Default refresh interval is 1 hour, which matches
	// Google's key rotation schedule.
	k, err := keyfunc.NewDefaultCtx(ctx, []string{jwksURL})
	if err != nil {
		return nil, fmt.Errorf("gip: init jwks: %w", err)
	}

	return &jwtVerifier{
		projectID: cfg.ProjectID,
		keys:      k,
	}, nil
}

// gipClaims is the subset of GIP ID token claims we need.
//
// The "firebase" object holds the tenant ID under "tenant". GIP also
// includes "sign_in_provider" (e.g. "password", "google.com") which we
// could enforce later for "passwordless only" or similar policies.
type gipClaims struct {
	Email    string `json:"email"`
	Firebase struct {
		Tenant         string `json:"tenant"`
		SignInProvider string `json:"sign_in_provider"`
	} `json:"firebase"`
	jwt.RegisteredClaims
}

// VerifyToken parses + signature-verifies the JWT, asserts standard claims,
// and explicitly compares the firebase.tenant claim against expectedTenantID.
//
// The standard claim checks happen INSIDE jwt.ParseWithClaims via
// jwt.WithValidMethods, jwt.WithIssuer, jwt.WithAudience, jwt.WithExpirationRequired.
// The tenant comparison happens AFTER parsing because expectedTenantID is
// not a standard JWT claim.
func (v *jwtVerifier) VerifyToken(ctx context.Context, idToken, expectedTenantID string) (*VerifiedToken, error) {
	if idToken == "" {
		return nil, ErrTokenInvalid
	}
	if expectedTenantID == "" {
		return nil, fmt.Errorf("gip: expectedTenantID is required (won't verify pool-less tokens)")
	}

	expectedIssuer := jwtIssuerPrefix + v.projectID

	claims := &gipClaims{}
	tok, err := jwt.ParseWithClaims(idToken, claims, v.keys.Keyfunc,
		jwt.WithValidMethods([]string{"RS256"}),
		jwt.WithIssuer(expectedIssuer),
		jwt.WithAudience(v.projectID),
		jwt.WithExpirationRequired(),
		jwt.WithIssuedAt(),
		jwt.WithLeeway(10*time.Second), // small leeway for clock skew
	)
	if err != nil {
		return nil, fmt.Errorf("%w: %s", ErrTokenInvalid, err)
	}
	if !tok.Valid {
		return nil, ErrTokenInvalid
	}

	// auth-bug #5 fix: explicitly compare the firebase.tenant claim.
	if claims.Firebase.Tenant != expectedTenantID {
		return nil, fmt.Errorf("%w: token tenant %q != expected %q",
			ErrTenantMismatch, claims.Firebase.Tenant, expectedTenantID)
	}

	// `sub` is the GIP user ID. RegisteredClaims.Subject holds it after parse.
	uid := claims.Subject
	if uid == "" {
		return nil, fmt.Errorf("%w: missing sub claim", ErrTokenInvalid)
	}

	return &VerifiedToken{
		UID:      uid,
		Email:    claims.Email,
		TenantID: claims.Firebase.Tenant,
	}, nil
}
