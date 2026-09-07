package subscription_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/mark8ly/marketplace-api/internal/subscription"
)

const testInternalSecret = "internal-secret-for-tests"

type stubBootstrapper struct {
	calls []subscription.BootstrapInput
	err   error
}

func (b *stubBootstrapper) Bootstrap(_ context.Context, in subscription.BootstrapInput) (*subscription.StoreSubscription, error) {
	b.calls = append(b.calls, in)
	if b.err != nil {
		return nil, b.err
	}
	return &subscription.StoreSubscription{
		ID: uuid.New(), TenantID: in.TenantID, StoreID: in.StoreID,
		Plan: subscription.PlanTrial, Status: subscription.StatusSignup,
	}, nil
}

// router builds the real thing, through Register, and drives it with
// ServeHTTP. Deliberately not a 404-vs-405 probe: HandleMethodNotAllowed is
// never set in this estate, so gin answers 404 whether or not a route is
// mounted and that probe distinguishes nothing (#642).
func router(t *testing.T, svc subscription.Bootstrapper) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	e := gin.New()
	subscription.NewInternalHandler(svc).RegisterRoutes(e.Group("/internal"), testInternalSecret)
	return e
}

func ensureReq(t *testing.T, storeID uuid.UUID, body map[string]any, secret string) *http.Request {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost,
		"/internal/stores/"+storeID.String()+"/ensure-subscription", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	if secret != "" {
		req.Header.Set("X-Internal-Auth", secret)
	}
	return req
}

// The point of #827: completing onboarding leaves a row, and that row's
// existence is what starts the trial.
func TestEnsureSubscription_BootstrapsTheStore(t *testing.T) {
	svc := &stubBootstrapper{}
	storeID, tenantID := uuid.New(), uuid.New()

	rec := httptest.NewRecorder()
	router(t, svc).ServeHTTP(rec, ensureReq(t, storeID, map[string]any{
		"tenant_id": tenantID.String(),
		"email":     "founder@example.com",
		"name":      "Bondi Surf Co",
		"currency":  "aud",
	}, testInternalSecret))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if len(svc.calls) != 1 {
		t.Fatalf("Bootstrap called %d times, want 1", len(svc.calls))
	}
	got := svc.calls[0]
	if got.StoreID != storeID || got.TenantID != tenantID {
		t.Errorf("ids = %s/%s, want %s/%s", got.TenantID, got.StoreID, tenantID, storeID)
	}
	if got.BillingCurrency != "aud" {
		t.Errorf("BillingCurrency = %q, want aud — nothing else supplies it later", got.BillingCurrency)
	}
}

// The route writes a row that starts a billing clock. Unlike
// /internal/stores/upsert beside it, it is guarded — and the guard is only
// worth having if it actually refuses.
func TestEnsureSubscription_RefusesWithoutTheInternalSecret(t *testing.T) {
	svc := &stubBootstrapper{}

	for _, tc := range []struct{ name, secret string }{
		{"no header", ""},
		{"wrong secret", "not-the-secret"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			router(t, svc).ServeHTTP(rec, ensureReq(t, uuid.New(),
				map[string]any{"tenant_id": uuid.New().String()}, tc.secret))

			if rec.Code == http.StatusOK {
				t.Fatalf("status = %d — an unauthenticated caller started a trial", rec.Code)
			}
			if len(svc.calls) != 0 {
				t.Fatal("Bootstrap ran for a request that failed the guard")
			}
		})
	}
}

func TestEnsureSubscription_RejectsAMalformedStoreID(t *testing.T) {
	svc := &stubBootstrapper{}
	req := httptest.NewRequest(http.MethodPost,
		"/internal/stores/not-a-uuid/ensure-subscription", bytes.NewReader([]byte(`{"tenant_id":"x"}`)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Internal-Auth", testInternalSecret)

	rec := httptest.NewRecorder()
	router(t, svc).ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
	if len(svc.calls) != 0 {
		t.Fatal("Bootstrap ran with an unparseable store id")
	}
}

// A missing tenant must be a 400, not a row created against the zero UUID —
// which the repository would accept as a real tenant and no query would ever
// find again.
func TestEnsureSubscription_RejectsAMissingTenant(t *testing.T) {
	svc := &stubBootstrapper{}
	rec := httptest.NewRecorder()
	router(t, svc).ServeHTTP(rec, ensureReq(t, uuid.New(), map[string]any{}, testInternalSecret))

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
	if len(svc.calls) != 0 {
		t.Fatal("Bootstrap ran with no tenant")
	}
}

// Onboarding treats this call as best-effort, so the failure has to be
// legible on the wire rather than swallowed into a 200 the caller cannot
// distinguish from success.
func TestEnsureSubscription_ReportsABootstrapFailure(t *testing.T) {
	svc := &stubBootstrapper{err: errors.New("db down")}
	rec := httptest.NewRecorder()
	router(t, svc).ServeHTTP(rec, ensureReq(t, uuid.New(),
		map[string]any{"tenant_id": uuid.New().String()}, testInternalSecret))

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
}

// Documents a security-relevant choice rather than asserting a nice property:
// with NO secret configured the route is open, exactly like every other
// /internal route in this service. Made deliberately — a route that fails
// closed while the ones beside it stay open turns a missing deployment
// variable into a partial outage that presents as a bug in whichever feature
// uses the strict route. If the /internal namespace ever starts failing
// closed, this test should change WITH the others, not before them.
func TestEnsureSubscription_IsOpenWhenNoSecretIsConfigured(t *testing.T) {
	svc := &stubBootstrapper{}
	gin.SetMode(gin.TestMode)
	e := gin.New()
	subscription.NewInternalHandler(svc).RegisterRoutes(e.Group("/internal"), "")

	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, ensureReq(t, uuid.New(),
		map[string]any{"tenant_id": uuid.New().String()}, ""))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 — parity with the rest of /internal", rec.Code)
	}
}
