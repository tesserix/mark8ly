package session

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

// switchRouter mounts a bare Handler (no FGA — nil FGA degrades the
// membership check to "allow any target id", which is fine for these
// tests: they exercise the Session rebuild, not authorization).
func switchRouter(mgr *Manager) *gin.Engine {
	gin.SetMode(gin.TestMode)
	h := NewHandler(mgr, nil)
	r := gin.New()
	g := r.Group("/auth")
	h.Register(g)
	return r
}

// requestWithSession mints a session cookie carrying the given
// AuthContext and attaches it to a fresh request against the given path.
func requestWithSession(t *testing.T, mgr *Manager, method, path, body string) *http.Request {
	t.Helper()
	w := httptest.NewRecorder()
	if err := mgr.Mint(w, Session{
		UID:         "user-1",
		Email:       "user@example.com",
		TenantID:    "tenant-a",
		AuthContext: "break_glass",
	}); err != nil {
		t.Fatalf("Mint: %v", err)
	}
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	for _, c := range w.Result().Cookies() {
		req.AddCookie(c)
	}
	return req
}

// TestSwitchTenant_PreservesAuthContext is the regression test for the
// hole D1's amendment describes: a merchant calling POST
// /auth/switch-tenant must not be able to silently strip AuthContext
// off their own session by re-minting it.
func TestSwitchTenant_PreservesAuthContext(t *testing.T) {
	mgr := newTestManager(t)
	router := switchRouter(mgr)

	req := requestWithSession(t, mgr, http.MethodPost, "/auth/switch-tenant", `{"tenant_id":"tenant-b"}`)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status: got %d want 200, body=%s", w.Code, w.Body.String())
	}

	nextReq := httptest.NewRequest(http.MethodGet, "/", nil)
	for _, c := range w.Result().Cookies() {
		nextReq.AddCookie(c)
	}
	got, err := mgr.Read(nextReq)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if got == nil {
		t.Fatal("Read returned nil session after switch-tenant")
	}
	if got.AuthContext != "break_glass" {
		t.Errorf("AuthContext = %q, want %q — switch-tenant dropped it", got.AuthContext, "break_glass")
	}
	if got.TenantID != "tenant-b" {
		t.Errorf("TenantID = %q, want tenant-b", got.TenantID)
	}
}

// TestSwitchStore_PreservesAuthContext mirrors the switch-tenant
// regression test for POST /auth/switch-store — the other rebuild site
// D1's amendment names.
func TestSwitchStore_PreservesAuthContext(t *testing.T) {
	mgr := newTestManager(t)
	router := switchRouter(mgr)

	req := requestWithSession(t, mgr, http.MethodPost, "/auth/switch-store", `{"store_id":"store-b"}`)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status: got %d want 200, body=%s", w.Code, w.Body.String())
	}

	nextReq := httptest.NewRequest(http.MethodGet, "/", nil)
	for _, c := range w.Result().Cookies() {
		nextReq.AddCookie(c)
	}
	got, err := mgr.Read(nextReq)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if got == nil {
		t.Fatal("Read returned nil session after switch-store")
	}
	if got.AuthContext != "break_glass" {
		t.Errorf("AuthContext = %q, want %q — switch-store dropped it", got.AuthContext, "break_glass")
	}
	if got.StoreID != "store-b" {
		t.Errorf("StoreID = %q, want store-b", got.StoreID)
	}
}
