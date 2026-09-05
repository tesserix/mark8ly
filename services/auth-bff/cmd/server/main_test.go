package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/mark8ly/auth-bff/internal/internalauth"
	"github.com/mark8ly/auth-bff/internal/zitadellogin"
	"github.com/mark8ly/auth-bff/pkg/config"
)

// TestZitadelHandlersUseTheCorrectReturnURLAllowlist is review Finding 5's
// fix: nothing else in this codebase notices if newZitadelHandlers ever
// passed the Admin allowlist to the customer handler (or the Storefront one
// to the merchant handler) — every test in internal/zitadellogin stays green
// either way, since that package only ever sees whatever ReturnURLAllowlist
// it is handed. This test drives the actual routes newZitadelHandlers wires,
// through Register, with an admin-only host and a storefront-only host, and
// would fail if main.go ever swapped which pair goes to which handler —
// exactly the swap that would reopen the merchant-controlled-origin open
// redirect the two-allowlist split exists to close.
func TestZitadelHandlersUseTheCorrectReturnURLAllowlist(t *testing.T) {
	fakeZitadel := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"authUrl":"https://idp.example.test/auth?state=intent-1"}`))
	}))
	defer fakeZitadel.Close()

	cfg := &config.Config{
		ZitadelIssuer:                 fakeZitadel.URL,
		ZitadelLoginClientToken:       "pat",
		MarketplaceInternalAuthSecret: "secret",
		ZitadelGoogleIDPID:            "idp-1",
		ZitadelOrgID:                  "org-1",
		// Deliberately non-overlapping domains — a suffix under one must
		// never accidentally also satisfy the other, which would make this
		// test unable to distinguish a swap from a correct wiring.
		ZitadelReturnURLAllowedHostsAdmin:         []string{"admin.tesserix-console.test"},
		ZitadelReturnURLAllowedSuffixesStorefront: []string{"shop-storefront.test"},
	}
	client := zitadellogin.New(fakeZitadel.URL, "pat", fakeZitadel.Client())

	zh, err := newZitadelHandlers(cfg, client, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("newZitadelHandlers: %v", err)
	}

	gin.SetMode(gin.TestMode)
	newRouter := func(register func(*gin.RouterGroup)) *gin.Engine {
		r := gin.New()
		register(r.Group("/auth"))
		return r
	}
	post := func(r *gin.Engine, path, returnURL string) int {
		rec := httptest.NewRecorder()
		body := `{"return_url":"` + returnURL + `"}`
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
		req.Header.Set(internalauth.Header, cfg.MarketplaceInternalAuthSecret)
		r.ServeHTTP(rec, req)
		return rec.Code
	}

	merchantRouter := newRouter(zh.merchant.Register)
	customerRouter := newRouter(zh.customer.Register)

	const adminURL = "https://admin.tesserix-console.test/auth/idp/finish"
	const storefrontURL = "https://shop.shop-storefront.test/auth/idp/finish"

	if code := post(merchantRouter, "/auth/zitadel/idp/start", adminURL); code != http.StatusOK {
		t.Errorf("merchant + admin host: status = %d, want 200 — the merchant handler must accept its own (Admin) allowlist", code)
	}
	if code := post(merchantRouter, "/auth/zitadel/idp/start", storefrontURL); code != http.StatusBadRequest {
		t.Errorf("merchant + storefront host: status = %d, want 400 — the merchant handler must NOT accept the Storefront allowlist; a pass here means main.go wired the wrong allowlist into the merchant handler", code)
	}
	if code := post(customerRouter, "/auth/customer/idp/start", storefrontURL); code != http.StatusOK {
		t.Errorf("customer + storefront host: status = %d, want 200 — the customer handler must accept its own (Storefront) allowlist", code)
	}
	if code := post(customerRouter, "/auth/customer/idp/start", adminURL); code != http.StatusBadRequest {
		t.Errorf("customer + admin host: status = %d, want 400 — the customer handler must NOT accept the Admin allowlist; a pass here means main.go wired the wrong allowlist into the customer handler", code)
	}
}
