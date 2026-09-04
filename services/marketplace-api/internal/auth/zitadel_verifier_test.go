package auth

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
)

// zitadelTestKey is the RSA key used to sign tokens across this file's
// tests. Low-entropy test fixtures elsewhere in the repo use a fixed
// string; a real RSA key can't be low-entropy, so it's generated once per
// test run instead of being a literal that might trip a secret scanner.
var zitadelTestKey *rsa.PrivateKey

func init() {
	var err error
	zitadelTestKey, err = rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		panic("init zitadelTestKey: " + err.Error())
	}
}

// stubZitadelServer serves the OIDC discovery document and JWKS for issuer
// srv.URL, signed with zitadelTestKey under kid "test-key-1".
func stubZitadelServer(t *testing.T) *httptest.Server {
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
			pub := zitadelTestKey.Public().(*rsa.PublicKey)
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

// signZitadelToken signs a JWT with zitadelTestKey. issuer and exp are
// caller-controlled so tests can produce a wrong-issuer or expired token.
func signZitadelToken(t *testing.T, issuer, subject string, exp time.Time, extra map[string]any) string {
	t.Helper()

	sig, err := josejwt.NewSigner(
		josejwt.SigningKey{Algorithm: josejwt.RS256, Key: zitadelTestKey},
		(&josejwt.SignerOptions{}).WithType("JWT").WithHeader("kid", "test-key-1"),
	)
	require.NoError(t, err)

	now := time.Now()
	claims := map[string]any{
		"iss": issuer,
		"sub": subject,
		"aud": []string{"zitadel-project-id"},
		"iat": now.Unix(),
		"exp": exp.Unix(),
	}
	for k, v := range extra {
		claims[k] = v
	}

	raw, err := jwt.Signed(sig).Claims(claims).Serialize()
	require.NoError(t, err)
	return raw
}

// signZitadelTokenWithWrongKey signs a token with a throwaway key that is
// not the one served at /jwks, so signature verification must fail.
func signZitadelTokenWithWrongKey(t *testing.T, issuer, subject string) string {
	t.Helper()

	wrongKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	sig, err := josejwt.NewSigner(
		josejwt.SigningKey{Algorithm: josejwt.RS256, Key: wrongKey},
		(&josejwt.SignerOptions{}).WithType("JWT").WithHeader("kid", "test-key-1"),
	)
	require.NoError(t, err)

	now := time.Now()
	claims := map[string]any{
		"iss": issuer,
		"sub": subject,
		"aud": []string{"zitadel-project-id"},
		"iat": now.Unix(),
		"exp": now.Add(time.Hour).Unix(),
	}
	raw, err := jwt.Signed(sig).Claims(claims).Serialize()
	require.NoError(t, err)
	return raw
}

// testAudience is the fixed "ZITADEL_ADMIN_PROJECT_ID"-shaped audience used
// across this file's happy-path tests. It's passed to NewZitadelVerifier as
// configuration, exactly as the real project ID would be, never hardcoded
// inside the verifier itself.
const testAudience = "zitadel-project-id"

func TestZitadelVerifier_ValidToken_ReturnsSubjectAsUserID(t *testing.T) {
	srv := stubZitadelServer(t)
	v, err := NewZitadelVerifier(context.Background(), srv.URL, testAudience)
	require.NoError(t, err)

	token := signZitadelToken(t, srv.URL, "zitadel-user-123", time.Now().Add(time.Hour), nil)

	claims, err := v.Verify(context.Background(), token)
	require.NoError(t, err)
	require.Equal(t, "zitadel-user-123", claims.UserID)
}

func TestZitadelVerifier_TenantIDAlwaysEmpty_EvenWithTenantClaims(t *testing.T) {
	srv := stubZitadelServer(t)
	v, err := NewZitadelVerifier(context.Background(), srv.URL, testAudience)
	require.NoError(t, err)

	// A token that DOES carry a tenant-ish claim — proves the verifier
	// ignores it rather than never having been tested against one.
	token := signZitadelToken(t, srv.URL, "zitadel-user-456", time.Now().Add(time.Hour), map[string]any{
		"tenant_id":                             "should-be-ignored",
		"urn:zitadel:iam:org:id":                "org-should-be-ignored",
		"urn:zitadel:iam:user:resourceowner:id": "owner-should-be-ignored",
	})

	claims, err := v.Verify(context.Background(), token)
	require.NoError(t, err)
	require.Equal(t, "zitadel-user-456", claims.UserID)
	require.Empty(t, claims.TenantID)
}

func TestZitadelVerifier_BadSignature_Fails(t *testing.T) {
	srv := stubZitadelServer(t)
	v, err := NewZitadelVerifier(context.Background(), srv.URL, testAudience)
	require.NoError(t, err)

	token := signZitadelTokenWithWrongKey(t, srv.URL, "zitadel-user-789")

	_, err = v.Verify(context.Background(), token)
	require.Error(t, err)
	require.ErrorIs(t, err, ErrInvalidToken)
}

func TestZitadelVerifier_WrongIssuer_Fails(t *testing.T) {
	srv := stubZitadelServer(t)
	v, err := NewZitadelVerifier(context.Background(), srv.URL, testAudience)
	require.NoError(t, err)

	token := signZitadelToken(t, "https://not-the-configured-issuer.example.com", "zitadel-user-999", time.Now().Add(time.Hour), nil)

	_, err = v.Verify(context.Background(), token)
	require.Error(t, err)
	require.ErrorIs(t, err, ErrInvalidToken)
}

func TestZitadelVerifier_ExpiredToken_Fails(t *testing.T) {
	srv := stubZitadelServer(t)
	v, err := NewZitadelVerifier(context.Background(), srv.URL, testAudience)
	require.NoError(t, err)

	token := signZitadelToken(t, srv.URL, "zitadel-user-expired", time.Now().Add(-time.Hour), nil)

	_, err = v.Verify(context.Background(), token)
	require.Error(t, err)
	require.ErrorIs(t, err, ErrInvalidToken)
}

// TestZitadelVerifier_WrongAudience_Fails is the regression test for the
// shopper-token→admin-credential escalation: a validly-signed, unexpired,
// correct-issuer token minted for a different Zitadel project (e.g.
// mark8ly-storefront, sharing this same instance and issuer) must NOT be
// accepted as a mark8ly-admin credential. Signature + issuer alone are not
// enough to distinguish "this project's token" from "any project's token
// belonging to the same signed-in human" — aud is the only field that does.
func TestZitadelVerifier_WrongAudience_Fails(t *testing.T) {
	srv := stubZitadelServer(t)
	v, err := NewZitadelVerifier(context.Background(), srv.URL, testAudience)
	require.NoError(t, err)

	// Minted for "mark8ly-storefront-project-id", not testAudience.
	token := signZitadelToken(t, srv.URL, "zitadel-user-shopper", time.Now().Add(time.Hour), map[string]any{
		"aud": []string{"mark8ly-storefront-project-id"},
	})

	_, err = v.Verify(context.Background(), token)
	require.Error(t, err)
	require.ErrorIs(t, err, ErrInvalidToken)
}

// TestZitadelVerifier_CorrectAudience_Passes proves the audience pin isn't
// simply rejecting everything: a token minted for the configured admin
// project, alongside another audience entry, still verifies.
func TestZitadelVerifier_CorrectAudience_Passes(t *testing.T) {
	srv := stubZitadelServer(t)
	v, err := NewZitadelVerifier(context.Background(), srv.URL, testAudience)
	require.NoError(t, err)

	token := signZitadelToken(t, srv.URL, "zitadel-user-admin", time.Now().Add(time.Hour), map[string]any{
		"aud": []string{testAudience, "some-other-audience"},
	})

	claims, err := v.Verify(context.Background(), token)
	require.NoError(t, err)
	require.Equal(t, "zitadel-user-admin", claims.UserID)
}
