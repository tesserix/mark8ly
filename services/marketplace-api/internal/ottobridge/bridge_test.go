package ottobridge

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/mark8ly/marketplace-api/internal/ottoclient"
)

func fixedResolver(scope ottoclient.ForwardScope, id ottoclient.ForwardIdentity) Resolver {
	return func(c *gin.Context) (ottoclient.ForwardScope, ottoclient.ForwardIdentity, bool) {
		return scope, id, true
	}
}

func mountRig(t *testing.T, otto http.HandlerFunc, resolve Resolver) (*gin.Engine, func()) {
	t.Helper()
	srv := httptest.NewServer(otto)
	gin.SetMode(gin.TestMode)
	b := New(ottoclient.New(srv.URL, "secret"), "wss://api.test", nil)
	r := gin.New()
	b.Mount(r.Group("/support"), resolve)
	return r, srv.Close
}

func TestBridge_Create_MergesSessionTokenAndForwardsScope(t *testing.T) {
	var sawTenant string
	r, closeFn := mountRig(t, func(w http.ResponseWriter, req *http.Request) {
		sawTenant = req.Header.Get("X-Tenant-Id")
		http.SetCookie(w, &http.Cookie{Name: "otto_session", Value: "sess-1", Path: "/"})
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"conversation":{"id":"c1"}}`))
	}, fixedResolver(
		ottoclient.ForwardScope{TenantID: "tenant-x", StoreID: "store-x"},
		ottoclient.ForwardIdentity{UserID: "u1"},
	))
	defer closeFn()

	rec := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPost, "/support/conversations", strings.NewReader(`{"message":"hi"}`))
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("code=%d body=%s", rec.Code, rec.Body.String())
	}
	if sawTenant != "tenant-x" {
		t.Errorf("X-Tenant-Id forwarded = %q", sawTenant)
	}
	var out map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	if out["session_token"] != "sess-1" {
		t.Errorf("session_token = %v", out["session_token"])
	}
}

func TestBridge_WsTicket_ReturnsWsURL(t *testing.T) {
	r, closeFn := mountRig(t, func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ticket":"tkt"}`))
	}, fixedResolver(ottoclient.ForwardScope{TenantID: "t", StoreID: "s"}, ottoclient.ForwardIdentity{}))
	defer closeFn()

	rec := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPost, "/support/conversations/c9/ws-ticket", nil)
	r.ServeHTTP(rec, req)

	var out struct {
		Ticket string `json:"ticket"`
		WsURL  string `json:"ws_url"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	if out.Ticket != "tkt" {
		t.Errorf("ticket=%q", out.Ticket)
	}
	if out.WsURL != "wss://api.test/api/v1/storefront/otto/conversations/c9/ws" {
		t.Errorf("ws_url=%q", out.WsURL)
	}
}

func TestBridge_NilOtto_503(t *testing.T) {
	gin.SetMode(gin.TestMode)
	b := New(nil, "", nil)
	r := gin.New()
	b.Mount(r.Group("/support"), fixedResolver(ottoclient.ForwardScope{}, ottoclient.ForwardIdentity{}))

	rec := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/support/resume", nil)
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("code=%d want 503", rec.Code)
	}
}

func TestBridge_ResolverAbort_StopsRequest(t *testing.T) {
	var ottoHit bool
	r, closeFn := mountRig(t, func(w http.ResponseWriter, req *http.Request) {
		ottoHit = true
		w.WriteHeader(http.StatusOK)
	}, func(c *gin.Context) (ottoclient.ForwardScope, ottoclient.ForwardIdentity, bool) {
		c.JSON(http.StatusForbidden, gin.H{"error": "no_store"})
		return ottoclient.ForwardScope{}, ottoclient.ForwardIdentity{}, false
	})
	defer closeFn()

	rec := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/support/resume", nil)
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("code=%d want 403", rec.Code)
	}
	if ottoHit {
		t.Errorf("otto must not be called after resolver abort")
	}
}
