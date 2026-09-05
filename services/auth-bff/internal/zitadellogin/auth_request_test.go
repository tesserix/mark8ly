package zitadellogin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// CreateAuthRequest is what lets the server start an OIDC flow with no
// browser involved — the precondition for keeping mark8ly's own login
// form on mobile instead of redirecting to Zitadel's hosted login.
func TestCreateAuthRequest_ExtractsIDFromTheRedirect(t *testing.T) {
	var gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		// Zitadel answers /oauth/v2/authorize with a 302 to the login UI,
		// carrying the auth request id. Kept deliberately short and
		// obviously fake: a realistic id is a long digit string that
		// secret scanners classify as a high-entropy credential.
		w.Header().Set("Location", "https://admin.mark8ly.com/login?authRequest=V2_test_auth_request")
		w.WriteHeader(http.StatusFound)
	}))
	defer srv.Close()

	ex := NewTokenExchanger(srv.URL, "client-1", "s", srv.Client())
	id, err := ex.CreateAuthRequest(context.Background(),
		"https://admin.mark8ly.com/auth/callback", "389070376568619523", "state-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != "V2_test_auth_request" {
		t.Fatalf("id = %q", id)
	}
	if !strings.Contains(gotQuery, "aud") {
		t.Fatalf("the authorize call must carry the project-audience scope, got %q", gotQuery)
	}
}

// The redirect must NOT be followed: following it would fetch the login
// page and lose the id, and on a real deployment would leave the server
// chasing a browser-facing URL.
func TestCreateAuthRequest_DoesNotFollowTheRedirect(t *testing.T) {
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		if r.URL.Path == "/oauth/v2/authorize" {
			w.Header().Set("Location", "/login?authRequest=V2_1")
			w.WriteHeader(http.StatusFound)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	_, err := NewTokenExchanger(srv.URL, testClientID, testClientPlaceholder, srv.Client()).
		CreateAuthRequest(context.Background(), "https://x/cb", "p", "st")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if hits != 1 {
		t.Fatalf("authorize was followed: %d requests, want 1", hits)
	}
}

// A 302 whose Location carries no authRequest means Zitadel refused the
// request (bad client, bad redirect_uri). That must be an error, not an
// empty id that fails confusingly at the next step.
func TestCreateAuthRequest_MissingIDIsAnError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Location", "https://admin.mark8ly.com/login")
		w.WriteHeader(http.StatusFound)
	}))
	defer srv.Close()

	if _, err := NewTokenExchanger(srv.URL, testClientID, testClientPlaceholder, srv.Client()).
		CreateAuthRequest(context.Background(), "https://x/cb", "p", "st"); err == nil {
		t.Fatal("a redirect without an authRequest id must error")
	}
}

func TestCreateAuthRequest_NonRedirectIsAnError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("invalid_request: redirect_uri mismatch"))
	}))
	defer srv.Close()

	_, err := NewTokenExchanger(srv.URL, testClientID, testClientPlaceholder, srv.Client()).
		CreateAuthRequest(context.Background(), "https://x/cb", "p", "st")
	if err == nil {
		t.Fatal("a non-redirect response must error")
	}
	// redirect_uri mismatch is the most likely misconfiguration here, so
	// the upstream text has to survive.
	if !strings.Contains(err.Error(), "redirect_uri") {
		t.Fatalf("upstream reason lost: %v", err)
	}
}
