package session

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

const mintSessionSecret = "test-internal-secret"

func mintSessionRouter(t *testing.T, mgr *Manager, secret string) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	h := NewMintSessionHandler(mgr, secret, nil)
	r := gin.New()
	g := r.Group("/internal")
	h.Register(g)
	return r
}

func doMintSession(t *testing.T, router *gin.Engine, header string, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/internal/mint-session", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if header != "" {
		req.Header.Set("X-Internal-Auth", header)
	}
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

// ─────────────────────────────────────────────────────────────────────────
// Allow-list acceptance — one per allowed value
// ─────────────────────────────────────────────────────────────────────────

func TestMintSession_AcceptsAllowedAuthContexts(t *testing.T) {
	for _, ac := range []string{"staff", "customer", "break_glass"} {
		t.Run(ac, func(t *testing.T) {
			mgr := newTestManager(t)
			router := mintSessionRouter(t, mgr, mintSessionSecret)

			body := `{"tenant_id":"tenant-a","user_id":"user-1","email":"user@example.com","auth_context":"` + ac + `"}`
			w := doMintSession(t, router, mintSessionSecret, body)

			if w.Code != http.StatusOK {
				t.Fatalf("status: got %d want 200, body=%s", w.Code, w.Body.String())
			}

			var resp MintSessionResponse
			if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
				t.Fatalf("unmarshal response: %v", err)
			}
			if resp.SetCookie == "" {
				t.Fatal("expected non-empty set_cookie")
			}
			if !strings.Contains(resp.SetCookie, mgr.cookieName+"=") {
				t.Errorf("set_cookie missing cookie name: %s", resp.SetCookie)
			}
		})
	}
}

// ─────────────────────────────────────────────────────────────────────────
// Allow-list rejection — this is the point of the endpoint
// ─────────────────────────────────────────────────────────────────────────

func TestMintSession_RejectsUnknownAuthContext(t *testing.T) {
	mgr := newTestManager(t)
	router := mintSessionRouter(t, mgr, mintSessionSecret)

	body := `{"tenant_id":"tenant-a","user_id":"user-1","auth_context":"stff"}`
	w := doMintSession(t, router, mintSessionSecret, body)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status: got %d want 400, body=%s", w.Code, w.Body.String())
	}
}

func TestMintSession_RejectsAbsentAuthContext(t *testing.T) {
	mgr := newTestManager(t)
	router := mintSessionRouter(t, mgr, mintSessionSecret)

	body := `{"tenant_id":"tenant-a","user_id":"user-1"}`
	w := doMintSession(t, router, mintSessionSecret, body)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status: got %d want 400, body=%s", w.Code, w.Body.String())
	}
}

func TestMintSession_RejectsEmptyAuthContext(t *testing.T) {
	mgr := newTestManager(t)
	router := mintSessionRouter(t, mgr, mintSessionSecret)

	body := `{"tenant_id":"tenant-a","user_id":"user-1","auth_context":""}`
	w := doMintSession(t, router, mintSessionSecret, body)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status: got %d want 400, body=%s", w.Code, w.Body.String())
	}
}

// ─────────────────────────────────────────────────────────────────────────
// X-Internal-Auth guard
// ─────────────────────────────────────────────────────────────────────────

func TestMintSession_RejectsWrongInternalAuth(t *testing.T) {
	mgr := newTestManager(t)
	router := mintSessionRouter(t, mgr, mintSessionSecret)

	body := `{"tenant_id":"tenant-a","user_id":"user-1","auth_context":"staff"}`
	w := doMintSession(t, router, "wrong-secret", body)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status: got %d want 401, body=%s", w.Code, w.Body.String())
	}
}

func TestMintSession_RejectsMissingInternalAuth(t *testing.T) {
	mgr := newTestManager(t)
	router := mintSessionRouter(t, mgr, mintSessionSecret)

	body := `{"tenant_id":"tenant-a","user_id":"user-1","auth_context":"staff"}`
	w := doMintSession(t, router, "", body)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status: got %d want 401, body=%s", w.Code, w.Body.String())
	}
}

// TestMintSession_FailsClosedWhenSecretEmpty is the config-absent case:
// no caller can authenticate against an unconfigured secret, matching
// InternalUsersHandler's fail-closed behaviour.
func TestMintSession_FailsClosedWhenSecretEmpty(t *testing.T) {
	mgr := newTestManager(t)
	router := mintSessionRouter(t, mgr, "")

	body := `{"tenant_id":"tenant-a","user_id":"user-1","auth_context":"staff"}`
	w := doMintSession(t, router, "anything", body)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status: got %d want 503, body=%s", w.Code, w.Body.String())
	}
}

// ─────────────────────────────────────────────────────────────────────────
// Round trip through the real Manager
// ─────────────────────────────────────────────────────────────────────────

func TestMintSession_CookieRoundTripsWithAuthContext(t *testing.T) {
	mgr := newTestManager(t)
	router := mintSessionRouter(t, mgr, mintSessionSecret)

	body := `{"tenant_id":"tenant-a","user_id":"user-1","email":"user@example.com","auth_context":"break_glass","ttl_seconds":7200}`
	w := doMintSession(t, router, mintSessionSecret, body)
	if w.Code != http.StatusOK {
		t.Fatalf("status: got %d want 200, body=%s", w.Code, w.Body.String())
	}

	var resp MintSessionResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}

	// Parse the Set-Cookie header value the way net/http would, then feed
	// it through the same Manager's Read to prove it decodes with
	// AuthContext intact — this is the whole point: cryptography stays
	// in auth-bff, and what marketplace-api forwards is exactly what
	// auth-bff can read back.
	header := http.Header{}
	header.Add("Set-Cookie", resp.SetCookie)
	respRec := http.Response{Header: header}
	cookies := respRec.Cookies()
	if len(cookies) != 1 {
		t.Fatalf("expected 1 cookie parsed from set_cookie, got %d", len(cookies))
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(cookies[0])

	got, err := mgr.Read(req)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if got == nil {
		t.Fatal("expected non-nil session")
	}
	if got.UID != "user-1" {
		t.Errorf("UID: got %q want %q", got.UID, "user-1")
	}
	if got.TenantID != "tenant-a" {
		t.Errorf("TenantID: got %q want %q", got.TenantID, "tenant-a")
	}
	if got.AuthContext != "break_glass" {
		t.Errorf("AuthContext: got %q want %q", got.AuthContext, "break_glass")
	}
}

// ─────────────────────────────────────────────────────────────────────────
// Route mounting — through the real router, not a direct handler call.
// Proves the endpoint is actually wired the same way main.go wires it
// onto the /internal group, guarding against the #642 failure mode where
// a handler was constructed only in tests and never mounted.
//
// gin's default engine (as constructed by pkg/httpserver.New, which
// main.go uses) does not enable HandleMethodNotAllowed, so a wrong
// method and an absent route both answer 404 — a GET can't distinguish
// them here. Instead this proves mounting the way that actually
// discriminates on this engine: registering the handler on a router
// group makes POST /internal/mint-session reachable (any response other
// than 404), and a router with nothing registered on that path answers
// 404 for the identical request.
// ─────────────────────────────────────────────────────────────────────────

func TestMintSession_RouteIsMountedOnInternalGroup(t *testing.T) {
	mgr := newTestManager(t)
	body := `{"tenant_id":"tenant-a","user_id":"user-1","auth_context":"staff"}`

	t.Run("unmounted router 404s", func(t *testing.T) {
		gin.SetMode(gin.TestMode)
		bare := gin.New()
		w := doMintSession(t, bare, mintSessionSecret, body)
		if w.Code != http.StatusNotFound {
			t.Fatalf("bare router: got %d want 404, body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("mounted router reaches the handler", func(t *testing.T) {
		router := mintSessionRouter(t, mgr, mintSessionSecret)
		w := doMintSession(t, router, mintSessionSecret, body)
		if w.Code == http.StatusNotFound {
			t.Fatalf("route not mounted: POST /internal/mint-session returned 404")
		}
		if w.Code != http.StatusOK {
			t.Fatalf("status: got %d want 200, body=%s", w.Code, w.Body.String())
		}
	})
}
