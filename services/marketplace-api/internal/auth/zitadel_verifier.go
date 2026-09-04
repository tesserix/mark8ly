// Package auth — Zitadel token verifier.
//
// ZitadelVerifier verifies Zitadel-issued bearer access tokens (JWTs) using
// github.com/coreos/go-oidc/v3, the same OIDC library already used by
// internal/sso/oidc.go for the SSO relying-party flow. That package
// discovers the IDP's JWKS via /.well-known/openid-configuration and hands
// back an *oidc.IDTokenVerifier that performs full RS256 signature
// verification against the fetched keys, plus issuer and expiry checks —
// exactly what's needed here, and exactly what this codebase already
// trusts elsewhere. There is no hand-rolled JWT parsing in this file.
package auth

import (
	"context"
	"fmt"

	gooidc "github.com/coreos/go-oidc/v3/oidc"
)

// ZitadelVerifier verifies Zitadel access tokens against a configured
// issuer's JWKS.
//
// Zitadel mints no tenant claim on access tokens (see decision D7 in the
// #524 migration design — adding one would require a Zitadel Actions v2
// script and a new runtime dependency on a shared instance). Tenancy is
// instead supplied by a separate FGA-backed middleware
// (TenantFromRequest). So Verify always returns an empty TenantID — this
// is deliberate, not a gap to fill in later.
type ZitadelVerifier struct {
	verifier *gooidc.IDTokenVerifier
}

// NewZitadelVerifier discovers the Zitadel issuer's OIDC configuration
// (and therefore its JWKS endpoint) and returns a ZitadelVerifier ready to
// verify bearer tokens issued by it.
//
// SkipClientIDCheck is set because this verifies API bearer tokens from
// any authorized client, not a single OIDC relying party's ID token — there
// is no single expected audience to pin here. Signature, issuer, and
// expiry are still fully checked.
func NewZitadelVerifier(ctx context.Context, issuer string) (*ZitadelVerifier, error) {
	if issuer == "" {
		return nil, fmt.Errorf("zitadel: issuer is required")
	}

	provider, err := gooidc.NewProvider(ctx, issuer)
	if err != nil {
		return nil, fmt.Errorf("zitadel: discover %s: %w", issuer, err)
	}

	verifier := provider.Verifier(&gooidc.Config{SkipClientIDCheck: true})

	return &ZitadelVerifier{verifier: verifier}, nil
}

// Verify checks the token's signature against the issuer's JWKS, and that
// its issuer matches and it is unexpired, then returns the subject as
// UserID. TenantID is always empty — see the ZitadelVerifier doc comment.
func (v *ZitadelVerifier) Verify(ctx context.Context, idToken string) (*TokenClaims, error) {
	token, err := v.verifier.Verify(ctx, idToken)
	if err != nil {
		// go-oidc's verification errors describe *why* (bad signature,
		// issuer mismatch, expired) but never echo the token itself, so
		// wrapping err is safe under the no-token-in-logs constraint.
		return nil, fmt.Errorf("%w: %v", ErrInvalidToken, err)
	}

	if token.Subject == "" {
		return nil, ErrInvalidToken
	}

	return &TokenClaims{
		UserID:   token.Subject,
		TenantID: "",
	}, nil
}
