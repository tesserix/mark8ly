package session

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/mark8ly/auth-bff/internal/internalauth"
)

type fakeProviders struct {
	out   []LinkedProvider
	err   error
	calls int
}

func (f *fakeProviders) LinkedProviders(_ context.Context, _ string) ([]LinkedProvider, error) {
	f.calls++
	return f.out, f.err
}

func providersRequest(t *testing.T, h *InternalUsersHandler, secret string) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h.Register(r.Group("/internal"))

	req := httptest.NewRequest(http.MethodGet, "/internal/users/u-1/providers", nil)
	if secret != "" {
		req.Header.Set(internalauth.Header, secret)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func decodeProviders(t *testing.T, w *httptest.ResponseRecorder) []LinkedProvider {
	t.Helper()
	var body struct {
		Data providersResponse `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body %q: %v", w.Body.String(), err)
	}
	return body.Data.Providers
}

func TestLinkedProvidersReturnsResolvedList(t *testing.T) {
	res := &fakeProviders{out: []LinkedProvider{
		{ProviderID: "password", Email: "jane@example.com"},
		{ProviderID: "google.com", Email: "jane@example.com"},
	}}
	h := NewInternalUsersHandler(nil, nil, "s3cret", nil).WithLinkedProviders(res)

	w := providersRequest(t, h, "s3cret")
	if w.Code != http.StatusOK {
		t.Fatalf("code=%d, want 200: %s", w.Code, w.Body.String())
	}
	got := decodeProviders(t, w)
	if len(got) != 2 || got[0].ProviderID != "password" || got[1].ProviderID != "google.com" {
		t.Fatalf("providers = %+v", got)
	}
	if got[1].Email != "jane@example.com" {
		t.Errorf("google email = %q", got[1].Email)
	}
}

// An account with nothing enrolled marshals as [] rather than null: the
// browser consumer maps over the list without a nil check.
func TestLinkedProvidersEmptyListIsNotNull(t *testing.T) {
	h := NewInternalUsersHandler(nil, nil, "s3cret", nil).WithLinkedProviders(&fakeProviders{})

	w := providersRequest(t, h, "s3cret")
	if w.Code != http.StatusOK {
		t.Fatalf("code=%d, want 200", w.Code)
	}
	var body struct {
		Data struct {
			Providers *[]LinkedProvider `json:"providers"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Data.Providers == nil {
		t.Fatalf("providers marshalled as null: %s", w.Body.String())
	}
	if len(*body.Data.Providers) != 0 {
		t.Errorf("providers = %+v, want empty", *body.Data.Providers)
	}
}

// Unlike the display-name endpoint, an unconfigured lookup must NOT
// answer with a blank list: a customer would read that as "my Google
// link is gone" rather than "we could not check".
func TestLinkedProvidersWithoutResolverIsUnavailable(t *testing.T) {
	h := NewInternalUsersHandler(nil, nil, "s3cret", nil)

	w := providersRequest(t, h, "s3cret")
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("code=%d, want 503: %s", w.Code, w.Body.String())
	}
}

func TestLinkedProvidersUpstreamFailureIs502(t *testing.T) {
	h := NewInternalUsersHandler(nil, nil, "s3cret", nil).
		WithLinkedProviders(&fakeProviders{err: errors.New("zitadel unreachable")})

	w := providersRequest(t, h, "s3cret")
	if w.Code != http.StatusBadGateway {
		t.Fatalf("code=%d, want 502", w.Code)
	}
}

func TestLinkedProvidersRejectsWrongSecret(t *testing.T) {
	res := &fakeProviders{out: []LinkedProvider{{ProviderID: "password"}}}
	h := NewInternalUsersHandler(nil, nil, "s3cret", nil).WithLinkedProviders(res)

	w := providersRequest(t, h, "wrong")
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("code=%d, want 401", w.Code)
	}
	if res.calls != 0 {
		t.Errorf("resolver called %d times on a rejected request, want 0", res.calls)
	}
}

func TestLinkedProvidersWithoutSecretIsUnavailable(t *testing.T) {
	res := &fakeProviders{out: []LinkedProvider{{ProviderID: "password"}}}
	h := NewInternalUsersHandler(nil, nil, "", nil).WithLinkedProviders(res)

	w := providersRequest(t, h, "")
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("code=%d, want 503", w.Code)
	}
	if res.calls != 0 {
		t.Errorf("resolver called %d times with no secret configured, want 0", res.calls)
	}
}

func TestLinkedProvidersFuncAdapts(t *testing.T) {
	var seen string
	f := LinkedProvidersFunc(func(_ context.Context, userID string) ([]LinkedProvider, error) {
		seen = userID
		return []LinkedProvider{{ProviderID: "password"}}, nil
	})
	got, err := f.LinkedProviders(context.Background(), "u-9")
	if err != nil || len(got) != 1 || seen != "u-9" {
		t.Fatalf("got %+v, err %v, seen %q", got, err, seen)
	}
}
