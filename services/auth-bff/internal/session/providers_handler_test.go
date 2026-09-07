package session

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

// meProvidersRouter mounts a Handler with the given resolver (nil is a
// deployment that cannot answer) and no FGA — /auth/me/providers reads
// the cookie and nothing else.
func meProvidersRouter(mgr *Manager, r LinkedProvidersResolver) *gin.Engine {
	gin.SetMode(gin.TestMode)
	h := NewHandler(mgr, nil).WithLinkedProviders(r)
	e := gin.New()
	h.Register(e.Group("/auth"))
	return e
}

func getMyProvidersResp(t *testing.T, r LinkedProvidersResolver, withCookie bool) *httptest.ResponseRecorder {
	t.Helper()
	mgr := newTestManager(t)
	router := meProvidersRouter(mgr, r)

	var req *http.Request
	if withCookie {
		req = requestWithSession(t, mgr, http.MethodGet, "/auth/me/providers", "")
	} else {
		req = httptest.NewRequest(http.MethodGet, "/auth/me/providers", nil)
	}
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

// decodeMyProviders asserts the wire shape the admin app's
// getMyProviders() parses: {"data":{"providers":[{"provider_id",…}]}}.
func decodeMyProviders(t *testing.T, w *httptest.ResponseRecorder) []LinkedProvider {
	t.Helper()
	var body struct {
		Data providersResponse `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v (body=%s)", err, w.Body.String())
	}
	return body.Data.Providers
}

func TestGetMyProviders_PasswordOnly(t *testing.T) {
	f := &fakeProviders{out: []LinkedProvider{{ProviderID: "password", Email: "user@example.com"}}}
	w := getMyProvidersResp(t, f, true)
	if w.Code != http.StatusOK {
		t.Fatalf("status: got %d want 200, body=%s", w.Code, w.Body.String())
	}
	got := decodeMyProviders(t, w)
	if len(got) != 1 || got[0].ProviderID != "password" || got[0].Email != "user@example.com" {
		t.Fatalf("providers = %+v", got)
	}
	if f.calls != 1 {
		t.Fatalf("resolver calls = %d, want 1", f.calls)
	}
}

// The exact bytes apps/admin/lib/auth/auth-bff.ts reads: a "data"
// envelope wrapping "providers", each entry keyed provider_id/email.
// This is the contract the GIP port must not have moved.
func TestGetMyProviders_WireShapeUnchanged(t *testing.T) {
	f := &fakeProviders{out: []LinkedProvider{
		{ProviderID: "password", Email: "user@example.com"},
		{ProviderID: "google.com", Email: "user@gmail.com"},
	}}
	w := getMyProvidersResp(t, f, true)
	want := `{"data":{"providers":[{"provider_id":"password","email":"user@example.com"},{"provider_id":"google.com","email":"user@gmail.com"}]}}`
	if w.Body.String() != want {
		t.Fatalf("body:\n got %s\nwant %s", w.Body.String(), want)
	}
}

// email is omitempty, so a provider that asserted no address must not
// emit a null the consumer would render as the string "null".
func TestGetMyProviders_OmitsEmptyEmail(t *testing.T) {
	f := &fakeProviders{out: []LinkedProvider{{ProviderID: "password"}}}
	w := getMyProvidersResp(t, f, true)
	if got, want := w.Body.String(), `{"data":{"providers":[{"provider_id":"password"}]}}`; got != want {
		t.Fatalf("body: got %s want %s", got, want)
	}
}

// An IDP the deployment has not named renders under its raw Zitadel id
// rather than vanishing from a list the merchant uses to audit access.
func TestGetMyProviders_UnrecognisedIDPKeepsRawID(t *testing.T) {
	f := &fakeProviders{out: []LinkedProvider{
		{ProviderID: "password", Email: "user@example.com"},
		{ProviderID: "idp-338290", Email: "user@corp.example"},
	}}
	w := getMyProvidersResp(t, f, true)
	got := decodeMyProviders(t, w)
	if len(got) != 2 || got[1].ProviderID != "idp-338290" {
		t.Fatalf("providers = %+v", got)
	}
}

// No resolver must be 503, never 200 with an empty list: a merchant
// reading an empty "Linked sign-in methods" panel would conclude their
// Google link had been removed.
func TestGetMyProviders_UnconfiguredIs503(t *testing.T) {
	w := getMyProvidersResp(t, nil, true)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status: got %d want 503, body=%s", w.Code, w.Body.String())
	}
	if got := decodeMyProviders(t, w); len(got) != 0 {
		t.Fatalf("503 must carry no providers, got %+v", got)
	}
}

func TestGetMyProviders_UpstreamFailureIs502(t *testing.T) {
	f := &fakeProviders{err: errors.New("zitadel unreachable")}
	w := getMyProvidersResp(t, f, true)
	if w.Code != http.StatusBadGateway {
		t.Fatalf("status: got %d want 502, body=%s", w.Code, w.Body.String())
	}
}

func TestGetMyProviders_NoSessionIs401(t *testing.T) {
	f := &fakeProviders{}
	w := getMyProvidersResp(t, f, false)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status: got %d want 401, body=%s", w.Code, w.Body.String())
	}
	if f.calls != 0 {
		t.Fatalf("resolver must not be called without a session, calls = %d", f.calls)
	}
}

// The resolver is asked about the cookie's own uid — never an id the
// caller supplied, which is what keeps this endpoint reading only the
// current merchant's methods.
func TestGetMyProviders_UsesSessionUID(t *testing.T) {
	var seen string
	r := LinkedProvidersFunc(func(_ context.Context, userID string) ([]LinkedProvider, error) {
		seen = userID
		return nil, nil
	})
	w := getMyProvidersResp(t, r, true)
	if w.Code != http.StatusOK {
		t.Fatalf("status: got %d want 200", w.Code)
	}
	if seen != "user-1" {
		t.Fatalf("resolver got user id %q, want %q", seen, "user-1")
	}
	// A nil slice must marshal as [] — the consumer maps over it.
	if got, want := w.Body.String(), `{"data":{"providers":[]}}`; got != want {
		t.Fatalf("body: got %s want %s", got, want)
	}
}
