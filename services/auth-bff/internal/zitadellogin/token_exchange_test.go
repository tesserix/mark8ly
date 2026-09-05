package zitadellogin

import (
	"context"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// Deliberately dictionary words, not random-looking strings: secret
// scanners flag a high-entropy literal sitting in a client-secret
// position, and a test fixture that satisfies a credential detector costs
// a CI failure and a history rewrite to undo.
const (
	testClientID          = "example-client-id"
	testClientPlaceholder = "not-a-real-value"
)

func TestExchangeCodeForTokens_SendsAuthorizationCodeGrantWithClientAuth(t *testing.T) {
	var gotPath, gotAuth, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		b := make([]byte, r.ContentLength)
		_, _ = r.Body.Read(b)
		gotBody = string(b)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"at","refresh_token":"rt","token_type":"Bearer","expires_in":3599}`))
	}))
	defer srv.Close()

	ex := NewTokenExchanger(srv.URL, testClientID, testClientPlaceholder, srv.Client())
	tok, err := ex.ExchangeCodeForTokens(context.Background(), "the-code", "https://admin.mark8ly.com/auth/callback")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if gotPath != "/oauth/v2/token" {
		t.Fatalf("path = %q, want /oauth/v2/token", gotPath)
	}
	// client_secret_basic: the app is a CONFIDENTIAL client
	// (authMethodType BASIC), so the secret goes in the Authorization
	// header, not the form body.
	wantAuth := "Basic " + base64.StdEncoding.EncodeToString([]byte(testClientID+":"+testClientPlaceholder))
	if gotAuth != wantAuth {
		t.Fatalf("Authorization = %q, want basic client auth", gotAuth)
	}
	form, _ := url.ParseQuery(gotBody)
	if form.Get("grant_type") != "authorization_code" {
		t.Fatalf("grant_type = %q", form.Get("grant_type"))
	}
	if form.Get("code") != "the-code" {
		t.Fatalf("code = %q", form.Get("code"))
	}
	// redirect_uri is REQUIRED on the exchange and must byte-match the
	// one the auth request was created with, or Zitadel refuses.
	if form.Get("redirect_uri") != "https://admin.mark8ly.com/auth/callback" {
		t.Fatalf("redirect_uri = %q", form.Get("redirect_uri"))
	}
	// The secret is in the header; sending it in the body too would leak
	// it into any request-body logging for no benefit.
	if form.Get("client_secret") != "" {
		t.Fatalf("client_secret must not be duplicated into the form body")
	}

	if tok.AccessToken != "at" || tok.RefreshToken != "rt" {
		t.Fatalf("tokens not parsed: %+v", tok)
	}
	if tok.ExpiresIn != 3599 {
		t.Fatalf("ExpiresIn = %d, want 3599", tok.ExpiresIn)
	}
}

// The mobile bearer token is verified by marketplace-api's ZitadelVerifier,
// which pins `aud` to the admin project id. That audience only appears if
// the AUTHORIZE step requested the project-aud scope — so CodeFromCallbackURL
// and the exchange must not silently succeed on a token that will later be
// rejected. This test pins the helper that builds that authorize URL.
func TestAuthorizeURL_RequestsTheProjectAudienceScope(t *testing.T) {
	ex := NewTokenExchanger("https://auth.tesserix.app", testClientID, testClientPlaceholder, nil)
	got := ex.AuthorizeURL("https://admin.mark8ly.com/auth/callback", "389070376568619523", "state-1")

	u, err := url.Parse(got)
	if err != nil {
		t.Fatalf("not a URL: %v", err)
	}
	q := u.Query()
	if q.Get("response_type") != "code" {
		t.Fatalf("response_type = %q", q.Get("response_type"))
	}
	scope := q.Get("scope")
	// Without this scope the access token verifies but carries no project
	// audience, and marketplace-api rejects it as a 401 that looks exactly
	// like a bad credential.
	if !strings.Contains(scope, "urn:zitadel:iam:org:project:id:389070376568619523:aud") {
		t.Fatalf("scope %q is missing the project-audience scope", scope)
	}
	if !strings.Contains(scope, "openid") {
		t.Fatalf("scope %q is missing openid", scope)
	}
	// offline_access is what yields a refresh token; without it a mobile
	// session dies at the access token's ~1h expiry and the merchant is
	// silently signed out.
	if !strings.Contains(scope, "offline_access") {
		t.Fatalf("scope %q is missing offline_access", scope)
	}
}

func TestCodeFromCallbackURL(t *testing.T) {
	got, err := CodeFromCallbackURL("https://admin.mark8ly.com/auth/callback?code=abc123&state=s")
	if err != nil || got != "abc123" {
		t.Fatalf("got %q err %v", got, err)
	}

	// Zitadel can return an error on the callback instead of a code; that
	// must surface as an error rather than an empty-string "success".
	if _, err := CodeFromCallbackURL("https://admin.mark8ly.com/auth/callback?error=access_denied"); err == nil {
		t.Fatal("an error callback must not parse as a code")
	}
	if _, err := CodeFromCallbackURL("://nonsense"); err == nil {
		t.Fatal("an unparseable URL must error")
	}
}

func TestExchangeCodeForTokens_UpstreamErrorSurfaces(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"invalid_grant"}`))
	}))
	defer srv.Close()

	_, err := NewTokenExchanger(srv.URL, testClientID, testClientPlaceholder, srv.Client()).
		ExchangeCodeForTokens(context.Background(), "bad", "https://x/cb")
	if err == nil {
		t.Fatal("a rejected exchange must return an error")
	}
	if !strings.Contains(err.Error(), "invalid_grant") {
		t.Fatalf("the upstream reason must survive for debugging, got %v", err)
	}
}
