package sso_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	josejwt "github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"
	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/sso"
)

// oidcTestKey is a shared RSA key used across OIDC sub-tests within a single
// test run.  Generated once per package test run, not per test.
var oidcTestKey *rsa.PrivateKey

func init() {
	var err error
	oidcTestKey, err = rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		panic("init oidcTestKey: " + err.Error())
	}
}

// stubOIDCServer starts an httptest.Server that serves:
//
//	GET /.well-known/openid-configuration  — discovery document
//	GET /jwks                              — JSON Web Key Set with oidcTestKey
//
// The returned issuer is srv.URL.
func stubOIDCServer(t *testing.T) *httptest.Server {
	t.Helper()

	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/.well-known/openid-configuration":
			doc := map[string]any{
				"issuer":                                srv.URL,
				"authorization_endpoint":                srv.URL + "/auth",
				"token_endpoint":                        srv.URL + "/token",
				"jwks_uri":                              srv.URL + "/jwks",
				"response_types_supported":              []string{"code"},
				"subject_types_supported":               []string{"public"},
				"id_token_signing_alg_values_supported": []string{"RS256"},
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(doc)

		case "/jwks":
			pub := oidcTestKey.Public().(*rsa.PublicKey)
			jwk := josejwt.JSONWebKey{
				Key:       pub,
				KeyID:     "test-key-1",
				Algorithm: "RS256",
				Use:       "sig",
			}
			set := josejwt.JSONWebKeySet{Keys: []josejwt.JSONWebKey{jwk}}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(set)

		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

// signIDToken creates a minimal signed RS256 JWT that go-oidc will accept.
func signIDToken(t *testing.T, issuer, clientID, subject, nonce string, extra map[string]any) string {
	t.Helper()

	sig, err := josejwt.NewSigner(
		josejwt.SigningKey{Algorithm: josejwt.RS256, Key: oidcTestKey},
		(&josejwt.SignerOptions{}).WithType("JWT").WithHeader("kid", "test-key-1"),
	)
	require.NoError(t, err)

	now := time.Now()
	claims := map[string]any{
		"iss":   issuer,
		"sub":   subject,
		"aud":   []string{clientID},
		"iat":   now.Unix(),
		"exp":   now.Add(time.Hour).Unix(),
		"nonce": nonce,
	}
	for k, v := range extra {
		claims[k] = v
	}

	raw, err := jwt.Signed(sig).Claims(claims).Serialize()
	require.NoError(t, err)
	return raw
}

// stubTokenServer returns an httptest.Server that mocks the token endpoint,
// returning an id_token signed with oidcTestKey.
func stubTokenServer(t *testing.T, issuer, clientID, subject, nonce string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		idToken := signIDToken(t, issuer, clientID, subject, nonce, nil)
		resp := map[string]any{
			"access_token": "test-access-token",
			"token_type":   "Bearer",
			"id_token":     idToken,
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	t.Cleanup(srv.Close)
	return srv
}

// buildTestRP creates an OIDCRelyingParty against the stub server.
func buildTestRP(t *testing.T, srv *httptest.Server, clientID string) *sso.OIDCRelyingParty {
	t.Helper()
	cfg := &sso.Config{
		Provider: sso.ProviderOIDC,
		Metadata: map[string]any{
			sso.OIDCKeyIssuer:   srv.URL,
			sso.OIDCKeyClientID: clientID,
		},
	}
	rp, err := sso.BuildOIDCRelyingParty(context.Background(), cfg, "test-secret")
	require.NoError(t, err)
	return rp
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

func TestOIDC_BuildRP_FromConfig(t *testing.T) {
	srv := stubOIDCServer(t)

	cfg := &sso.Config{
		Provider: sso.ProviderOIDC,
		Metadata: map[string]any{
			sso.OIDCKeyIssuer:   srv.URL,
			sso.OIDCKeyClientID: "client-abc",
		},
	}

	rp, err := sso.BuildOIDCRelyingParty(context.Background(), cfg, "test-secret")
	require.NoError(t, err)
	require.Equal(t, "client-abc", rp.ClientID)
}

func TestOIDC_BuildRP_MissingIssuer(t *testing.T) {
	cfg := &sso.Config{
		Provider: sso.ProviderOIDC,
		Metadata: map[string]any{
			sso.OIDCKeyClientID: "client-abc",
			// OIDCKeyIssuer intentionally omitted
		},
	}
	_, err := sso.BuildOIDCRelyingParty(context.Background(), cfg, "secret")
	require.Error(t, err)
	require.ErrorIs(t, err, sso.ErrInvalidMetadata)
}

func TestOIDC_BuildRP_MissingClientID(t *testing.T) {
	srv := stubOIDCServer(t)
	cfg := &sso.Config{
		Provider: sso.ProviderOIDC,
		Metadata: map[string]any{
			sso.OIDCKeyIssuer: srv.URL,
			// OIDCKeyClientID intentionally omitted
		},
	}
	_, err := sso.BuildOIDCRelyingParty(context.Background(), cfg, "secret")
	require.Error(t, err)
	require.ErrorIs(t, err, sso.ErrInvalidMetadata)
}

func TestOIDC_BuildRP_CustomScopes(t *testing.T) {
	srv := stubOIDCServer(t)
	cfg := &sso.Config{
		Provider: sso.ProviderOIDC,
		Metadata: map[string]any{
			sso.OIDCKeyIssuer:   srv.URL,
			sso.OIDCKeyClientID: "client-xyz",
			sso.OIDCKeyScopes:   "openid email profile groups",
		},
	}
	rp, err := sso.BuildOIDCRelyingParty(context.Background(), cfg, "secret")
	require.NoError(t, err)
	require.Contains(t, rp.OAuth.Scopes, "openid")
	require.Contains(t, rp.OAuth.Scopes, "groups")
}

func TestOIDC_AuthURL_ContainsStateAndNonce(t *testing.T) {
	srv := stubOIDCServer(t)
	rp := buildTestRP(t, srv, "client-abc")

	authURL := rp.AuthURL("state-xyz", "nonce-abc")
	require.Contains(t, authURL, "state=state-xyz")
	require.Contains(t, authURL, "nonce=")
}

func TestOIDC_Exchange_WrongNonce(t *testing.T) {
	discoverySrv := stubOIDCServer(t)
	const clientID = "client-abc"
	const subject = "user-123"
	const correctNonce = "good-nonce"

	// Build RP against the discovery server.
	rp := buildTestRP(t, discoverySrv, clientID)

	// Point the token endpoint at a stub that returns an id_token with correctNonce.
	tokenSrv := stubTokenServer(t, discoverySrv.URL, clientID, subject, correctNonce)

	// Patch the RP's OAuth config token endpoint URL to use the stub.
	rp.OAuth.Endpoint = rp.OAuth.Endpoint // keep auth endpoint
	rp.OAuth.Endpoint.TokenURL = tokenSrv.URL + "/token"
	tokenSrv.URL = tokenSrv.URL // suppress unused warning

	_, err := rp.Exchange(context.Background(), "some-code", "wrong-nonce")
	require.Error(t, err)
	require.ErrorContains(t, err, "nonce mismatch")
}

func TestOIDC_Exchange_HappyPath(t *testing.T) {
	discoverySrv := stubOIDCServer(t)
	const clientID = "client-abc"
	const subject = "user-123"
	const nonce = "correct-nonce"

	rp := buildTestRP(t, discoverySrv, clientID)

	// Stub the token endpoint so it returns our signed id_token.
	tokenSrv := stubTokenServer(t, discoverySrv.URL, clientID, subject, nonce)
	rp.OAuth.Endpoint.TokenURL = tokenSrv.URL + "/token"

	claims, err := rp.Exchange(context.Background(), "code-xyz", nonce)
	require.NoError(t, err)
	require.Equal(t, subject, claims["sub"])
	require.Equal(t, clientID, claims["aud"].([]any)[0])
}
