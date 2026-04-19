// Package sso — OIDC RP wiring via github.com/coreos/go-oidc/v3.
//
// BuildOIDCRelyingParty discovers the IDP's OIDC endpoints using the standard
// /.well-known/openid-configuration URL and returns an OIDCRelyingParty ready
// to drive the Authorization Code flow.
//
// The client secret is passed in by the caller (resolved from Secret Manager).
// It is NEVER stored in Config.Metadata — only a Secret Manager path reference
// lives there (OIDCKeyClientSecretRef).
package sso

import (
	"context"
	"fmt"
	"strings"

	gooidc "github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

// OIDCRelyingParty wraps coreos/go-oidc and oauth2 into a single struct that
// covers the Authorization Code flow: AuthURL → Exchange → verified claims.
type OIDCRelyingParty struct {
	// Provider is the underlying go-oidc provider (exposes endpoint metadata).
	Provider *gooidc.Provider
	// OAuth is the oauth2.Config used to build auth URLs and exchange codes.
	OAuth *oauth2.Config
	// verifier validates ID tokens returned by the IDP.
	verifier *gooidc.IDTokenVerifier
	// ClientID is the registered OAuth2 client identifier.
	ClientID string
}

// BuildOIDCRelyingParty constructs an OIDCRelyingParty by discovering the IDP
// endpoints from the issuer URL stored in cfg.Metadata[OIDCKeyIssuer].
//
// clientSecret is the plaintext secret resolved by the caller from Secret
// Manager — it must not be stored in the database.
//
// Scopes default to "openid email profile".  If OIDCKeyScopes is present in
// metadata its space-separated value is used instead (it must include
// "openid").
func BuildOIDCRelyingParty(ctx context.Context, cfg *Config, clientSecret string) (*OIDCRelyingParty, error) {
	if cfg == nil {
		return nil, fmt.Errorf("%w: cfg is nil", ErrInvalidMetadata)
	}

	issuer, ok := nonEmpty(cfg.Metadata, OIDCKeyIssuer)
	if !ok {
		return nil, fmt.Errorf("%w: %s required", ErrInvalidMetadata, OIDCKeyIssuer)
	}

	clientID, ok := nonEmpty(cfg.Metadata, OIDCKeyClientID)
	if !ok {
		return nil, fmt.Errorf("%w: %s required", ErrInvalidMetadata, OIDCKeyClientID)
	}

	redirectURI, _ := nonEmpty(cfg.Metadata, OIDCKeyRedirectURI)

	scopes := defaultOIDCScopes()
	if raw, ok := nonEmpty(cfg.Metadata, OIDCKeyScopes); ok {
		scopes = parseScopes(raw)
	}

	provider, err := gooidc.NewProvider(ctx, issuer)
	if err != nil {
		return nil, fmt.Errorf("oidc: discover %s: %w", issuer, err)
	}

	verifier := provider.Verifier(&gooidc.Config{ClientID: clientID})

	oauthCfg := &oauth2.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		RedirectURL:  redirectURI,
		Endpoint:     provider.Endpoint(),
		Scopes:       scopes,
	}

	return &OIDCRelyingParty{
		Provider: provider,
		OAuth:    oauthCfg,
		verifier: verifier,
		ClientID: clientID,
	}, nil
}

// AuthURL returns the IDP authorization endpoint URL with the given state and
// nonce parameters embedded.  The nonce is hashed and included in the auth
// request per the OIDC spec; it is compared against the ID token on Exchange.
func (rp *OIDCRelyingParty) AuthURL(state, nonce string) string {
	return rp.OAuth.AuthCodeURL(state, gooidc.Nonce(nonce))
}

// Exchange swaps an authorization code for tokens, verifies the ID token
// signature and nonce, and returns the full claims map.
//
// Returns an error wrapping "oidc: nonce mismatch" if the token's nonce does
// not equal expectedNonce.
func (rp *OIDCRelyingParty) Exchange(ctx context.Context, code, expectedNonce string) (map[string]any, error) {
	token, err := rp.OAuth.Exchange(ctx, code)
	if err != nil {
		return nil, fmt.Errorf("oidc: token exchange: %w", err)
	}

	rawIDToken, ok := token.Extra("id_token").(string)
	if !ok || rawIDToken == "" {
		return nil, fmt.Errorf("oidc: id_token missing from token response")
	}

	idToken, err := rp.verifier.Verify(ctx, rawIDToken)
	if err != nil {
		return nil, fmt.Errorf("oidc: id_token verify: %w", err)
	}

	if idToken.Nonce != expectedNonce {
		return nil, fmt.Errorf("oidc: nonce mismatch: got %q, want %q", idToken.Nonce, expectedNonce)
	}

	var claims map[string]any
	if err := idToken.Claims(&claims); err != nil {
		return nil, fmt.Errorf("oidc: decode claims: %w", err)
	}

	return claims, nil
}

// defaultOIDCScopes returns the baseline scopes for the Authorization Code flow.
func defaultOIDCScopes() []string {
	return []string{gooidc.ScopeOpenID, "email", "profile"}
}

// parseScopes splits a space-separated scope string.  "openid" is always
// prepended if not already present so the ID token is always requested.
func parseScopes(raw string) []string {
	parts := strings.Fields(raw)
	hasOpenID := false
	for _, p := range parts {
		if p == gooidc.ScopeOpenID {
			hasOpenID = true
			break
		}
	}
	if !hasOpenID {
		parts = append([]string{gooidc.ScopeOpenID}, parts...)
	}
	return parts
}
