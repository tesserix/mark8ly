package storefront

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/mark8ly/marketplace-api/internal/ottoclient"
	"github.com/mark8ly/marketplace-api/internal/stores"
)

// newSupportTestRig wires a gin engine whose routes are guarded by a stub
// middleware that mimics StoreContext + mobileCustomerProfileMW (sets the
// resolved store + verified customer identity on the context), pointed at
// a fake otto server. Returns the engine and a pointer the fake otto
// handler reads, so each test can assert on what otto received.
func newSupportTestRig(t *testing.T, otto http.HandlerFunc) (*gin.Engine, func()) {
	t.Helper()
	srv := httptest.NewServer(otto)
	gin.SetMode(gin.TestMode)
	h := NewMobileSupportHandler(ottoclient.New(srv.URL, "secret"), "wss://api.test", nil)

	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("store", &stores.Store{ID: "store-1", TenantID: "tenant-1", Status: stores.StatusActive})
		c.Set(CustomerGipUIDKey, "gip-abc")
		c.Set(CustomerEmailKey, "buyer@example.com")
		c.Next()
	})
	grp := r.Group("/support")
	h.Register(grp)
	return r, srv.Close
}

func TestSupport_Create_ForwardsScopeAndMergesSessionToken(t *testing.T) {
	var sawTenant, sawStore, sawUser string
	var sawBody string
	r, closeFn := newSupportTestRig(t, func(w http.ResponseWriter, req *http.Request) {
		sawTenant = req.Header.Get("X-Tenant-Id")
		sawStore = req.Header.Get("X-Store-Id")
		sawUser = req.Header.Get("X-User-Id")
		b, _ := io.ReadAll(req.Body)
		sawBody = string(b)
		http.SetCookie(w, &http.Cookie{Name: "otto_session", Value: "sess-xyz", Path: "/"})
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"conversation":{"id":"c1","case_id":"CS-1"},"first_message":{"id":"m1"}}`))
	})
	defer closeFn()

	rec := httptest.NewRecorder()
	body := `{"message":"hi","reason":"order_issue","status_info":"late","name":"Sam","email":"buyer@example.com"}`
	req, _ := http.NewRequest(http.MethodPost, "/support/conversations", strings.NewReader(body))
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
	if sawTenant != "tenant-1" || sawStore != "store-1" {
		t.Errorf("scope headers: tenant=%q store=%q", sawTenant, sawStore)
	}
	if sawUser != "gip-abc" {
		t.Errorf("X-User-Id = %q, want gip-abc (skips OTP)", sawUser)
	}
	if !strings.Contains(sawBody, `"reason":"order_issue"`) {
		t.Errorf("body not forwarded verbatim: %q", sawBody)
	}
	// The session token otto minted via Set-Cookie must be surfaced to the
	// app in the JSON body (the app can't read the HttpOnly cookie).
	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out["session_token"] != "sess-xyz" {
		t.Errorf("session_token = %v, want sess-xyz", out["session_token"])
	}
	if _, ok := out["conversation"]; !ok {
		t.Errorf("conversation missing from relayed body")
	}
	if rec.Header().Get("X-Otto-Session") != "sess-xyz" {
		t.Errorf("X-Otto-Session header not set")
	}
}

func TestSupport_PostMessage_ForwardsSessionHeaderAndId(t *testing.T) {
	var sawSession, sawPath string
	r, closeFn := newSupportTestRig(t, func(w http.ResponseWriter, req *http.Request) {
		sawSession = req.Header.Get("X-Otto-Session")
		sawPath = req.URL.Path
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"message":{"id":"m2"}}`))
	})
	defer closeFn()

	rec := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPost, "/support/conversations/c1/messages", strings.NewReader(`{"body":"hello"}`))
	req.Header.Set("X-Otto-Session", "sess-xyz")
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	if sawSession != "sess-xyz" {
		t.Errorf("X-Otto-Session forwarded = %q", sawSession)
	}
	if !strings.HasSuffix(sawPath, "/api/v1/storefront/otto/conversations/c1/messages") {
		t.Errorf("path = %q", sawPath)
	}
}

func TestSupport_WsTicket_AugmentsWithWsURL(t *testing.T) {
	r, closeFn := newSupportTestRig(t, func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ticket":"tkt-123"}`))
	})
	defer closeFn()

	rec := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPost, "/support/conversations/c1/ws-ticket", nil)
	req.Header.Set("X-Otto-Session", "sess-xyz")
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	var out struct {
		Ticket string `json:"ticket"`
		WsURL  string `json:"ws_url"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.Ticket != "tkt-123" {
		t.Errorf("ticket = %q", out.Ticket)
	}
	want := "wss://api.test/api/v1/storefront/otto/conversations/c1/ws"
	if out.WsURL != want {
		t.Errorf("ws_url = %q, want %q", out.WsURL, want)
	}
}

func TestSupport_NilClient_ReturnsUnavailable(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := NewMobileSupportHandler(nil, "", nil)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("store", &stores.Store{ID: "s", TenantID: "t", Status: stores.StatusActive})
		c.Next()
	})
	h.Register(r.Group("/support"))

	rec := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/support/resume", nil)
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503", rec.Code)
	}
}
