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
// issuer's JWKS and a required audience.
//
// Zitadel mints no tenant claim on access tokens (see decision D7 in the
// #524 migration design — adding one would require a Zitadel Actions v2
// script and a new runtime dependency on a shared instance). Tenancy is
// instead supplied by a separate FGA-backed middleware
// (TenantFromRequest). So Verify always returns an empty TenantID — this
// is deliberate, not a gap to fill in later. Callers additionally MUST
// wire GIPBearerAuth with setTenantFromClaim=false whenever this verifier
// is selected (ZITADEL_ENABLED=true), so this empty value is never even
// the thing that decides "tenant_id" is unset — TenantFromRequest is the
// only writer in that mode.
type ZitadelVerifier struct {
	verifier *gooidc.IDTokenVerifier
}

// NewZitadelVerifier discovers the Zitadel issuer's OIDC configuration
// (and therefore its JWKS endpoint) and returns a ZitadelVerifier ready to
// verify bearer tokens issued by it for the given audience.
//
// audience MUST be the mark8ly-admin Zitadel project ID (deployed as
// ZITADEL_ADMIN_PROJECT_ID — pass it in as configuration, never hardcode
// it here). All mark8ly projects — mark8ly-admin, mark8ly-storefront, and
// others — share one Zitadel instance (auth.tesserix.app), so they share
// both signer and issuer. Per decision D1, identity is also shared one
// human, one account across products — so a shopper's browser-held
// mark8ly-storefront token (a public PKCE client, reachable by XSS) is
// signed by the same key and carries the same issuer as a legitimate
// mark8ly-admin credential for that same human. Without an audience check,
// that shopper token would verify here and grant admin access (orders,
// refunds, team invites, account deletion) — FGA membership answers "is
// this human a member of the tenant", never "was this token issued for
// this API", so it cannot substitute for pinning aud. Setting ClientID here
// makes go-oidc require the token's "aud" claim to contain audience
// (go-oidc does a contains-check, not strict equality, so azp-style tokens
// with multiple audiences still verify) — the one field that actually
// discriminates between mark8ly-admin and mark8ly-storefront tokens.
func NewZitadelVerifier(ctx context.Context, issuer, audience string) (*ZitadelVerifier, error) {
	if issuer == "" {
		return nil, fmt.Errorf("zitadel: issuer is required")
	}
	if audience == "" {
		return nil, fmt.Errorf("zitadel: audience is required")
	}

	provider, err := gooidc.NewProvider(ctx, issuer)
	if err != nil {
		return nil, fmt.Errorf("zitadel: discover %s: %w", issuer, err)
	}

	verifier := provider.Verifier(&gooidc.Config{ClientID: audience})

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
