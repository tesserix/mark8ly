package ottoclient

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestForward_Create_PassesScopeIdentityAndBody covers the happy-path
// create bridge: the BFF forwards a customer's "open a chat" POST to
// otto's storefront surface. We assert every header otto's
// CustomerContext middleware reads plus the body passthrough, then check
// the session token is lifted out of otto's Set-Cookie so the mobile app
// can echo it back on later calls (it can't read the HttpOnly cookie).
func TestForward_Create_PassesScopeIdentityAndBody(t *testing.T) {
	var gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("X-Internal-Auth"); got != "shh" {
			t.Errorf("X-Internal-Auth = %q, want shh", got)
		}
		if got := r.Header.Get("X-Tenant-Id"); got != "tenant-1" {
			t.Errorf("X-Tenant-Id = %q, want tenant-1", got)
		}
		if got := r.Header.Get("X-Store-Id"); got != "store-1" {
			t.Errorf("X-Store-Id = %q, want store-1", got)
		}
		if got := r.Header.Get("X-User-Id"); got != "gip-abc" {
			t.Errorf("X-User-Id = %q, want gip-abc", got)
		}
		if got := r.Header.Get("X-User-Email"); got != "a@b.com" {
			t.Errorf("X-User-Email = %q, want a@b.com", got)
		}
		if !strings.HasSuffix(r.URL.Path, "/api/v1/storefront/otto/conversations") {
			t.Errorf("path = %q", r.URL.Path)
		}
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		http.SetCookie(w, &http.Cookie{Name: "otto_session", Value: "raw-token-123", Path: "/"})
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"conversation":{"id":"c1"},"first_message":{"id":"m1"}}`))
	}))
	defer srv.Close()

	c := New(srv.URL, "shh")
	res, err := c.Forward(context.Background(), ForwardRequest{
		Method:   http.MethodPost,
		Path:     "/conversations",
		Scope:    ForwardScope{TenantID: "tenant-1", StoreID: "store-1"},
		Identity: ForwardIdentity{UserID: "gip-abc", UserEmail: "a@b.com"},
		Body:     []byte(`{"message":"hi","reason":"other","status_info":"x"}`),
	})
	if err != nil {
		t.Fatalf("Forward: %v", err)
	}
	if res.Status != http.StatusCreated {
		t.Errorf("Status = %d, want 201", res.Status)
	}
	if gotBody != `{"message":"hi","reason":"other","status_info":"x"}` {
		t.Errorf("forwarded body = %q", gotBody)
	}
	if res.SessionToken != "raw-token-123" {
		t.Errorf("SessionToken = %q, want raw-token-123", res.SessionToken)
	}
	if !strings.Contains(string(res.Body), `"conversation"`) {
		t.Errorf("body passthrough = %q", res.Body)
	}
}

// TestForward_SessionTokenSentAsHeader covers the resume/reply path: the
// mobile app holds the opaque session token and the BFF re-presents it to
// otto as X-Otto-Session (otto's RequireCustomerSession accepts either the
// cookie or that header). No new cookie => SessionToken stays empty.
func TestForward_SessionTokenSentAsHeader(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("X-Otto-Session"); got != "raw-token-123" {
			t.Errorf("X-Otto-Session = %q, want raw-token-123", got)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"messages":[]}`))
	}))
	defer srv.Close()

	c := New(srv.URL, "shh")
	res, err := c.Forward(context.Background(), ForwardRequest{
		Method:       http.MethodGet,
		Path:         "/conversations/c1/messages",
		Scope:        ForwardScope{TenantID: "t", StoreID: "s"},
		SessionToken: "raw-token-123",
	})
	if err != nil {
		t.Fatalf("Forward: %v", err)
	}
	if res.Status != http.StatusOK {
		t.Errorf("Status = %d, want 200", res.Status)
	}
	if res.SessionToken != "" {
		t.Errorf("SessionToken = %q, want empty (no new cookie)", res.SessionToken)
	}
}

// TestForward_PlatformIdentity covers #119: merchant→platform chat runs
// on the same storefront surface but pinned to the platform tenant, with
// the merchant's own tenant carried as X-Client-Tenant-* so the Tesserix
// agent can see who is talking.
func TestForward_PlatformIdentity(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("X-Tenant-Id"); got != "platform" {
			t.Errorf("X-Tenant-Id = %q, want platform", got)
		}
		if got := r.Header.Get("X-Store-Id"); got != "default" {
			t.Errorf("X-Store-Id = %q, want default", got)
		}
		if got := r.Header.Get("X-Client-Tenant-Id"); got != "merchant-9" {
			t.Errorf("X-Client-Tenant-Id = %q, want merchant-9", got)
		}
		if got := r.Header.Get("X-Client-Tenant-Name"); got != "Acme Store" {
			t.Errorf("X-Client-Tenant-Name = %q, want Acme Store", got)
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"conversation":{"id":"p1"}}`))
	}))
	defer srv.Close()

	c := New(srv.URL, "shh")
	_, err := c.Forward(context.Background(), ForwardRequest{
		Method: http.MethodPost,
		Path:   "/conversations",
		Scope:  ForwardScope{TenantID: "platform", StoreID: "default"},
		Identity: ForwardIdentity{
			UserID:           "admin-1",
			UserEmail:        "owner@acme.com",
			ClientTenantID:   "merchant-9",
			ClientTenantName: "Acme Store",
		},
		Body: []byte(`{"message":"billing question","reason":"billing","status_info":"x"}`),
	})
	if err != nil {
		t.Fatalf("Forward: %v", err)
	}
}

// TestForward_UpstreamErrorPassesThrough — a 4xx from otto (e.g. closed
// thread) is NOT a Go error; the BFF must relay otto's status + body to
// the client verbatim so the app shows the right message.
func TestForward_UpstreamErrorPassesThrough(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"error":"thread_closed"}`))
	}))
	defer srv.Close()

	c := New(srv.URL, "shh")
	res, err := c.Forward(context.Background(), ForwardRequest{
		Method: http.MethodPost,
		Path:   "/conversations/c1/messages",
		Scope:  ForwardScope{TenantID: "t", StoreID: "s"},
		Body:   []byte(`{"body":"hello"}`),
	})
	if err != nil {
		t.Fatalf("Forward should pass through 4xx without error, got %v", err)
	}
	if res.Status != http.StatusConflict {
		t.Errorf("Status = %d, want 409", res.Status)
	}
	if !strings.Contains(string(res.Body), "thread_closed") {
		t.Errorf("body = %q", res.Body)
	}
}

// TestForward_TransportErrorSurfaces — a dead upstream IS a Go error so
// the handler can map it to 502.
func TestForward_TransportErrorSurfaces(t *testing.T) {
	c := New("http://127.0.0.1:0", "shh")
	_, err := c.Forward(context.Background(), ForwardRequest{
		Method: http.MethodGet,
		Path:   "/resume",
		Scope:  ForwardScope{TenantID: "t", StoreID: "s"},
	})
	if err == nil {
		t.Fatal("expected transport error")
	}
}
