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

// ---------------------------------------------------------------------------
// Promo at signup (#620)
// ---------------------------------------------------------------------------

type stubPromo struct {
	redeems   []subscription.SignupPromoInput
	validates []subscription.SignupPromoInput
	offer     subscription.SignupPromoOffer
	err       error
}

func (p *stubPromo) RedeemAtSignup(_ context.Context, in subscription.SignupPromoInput) (subscription.SignupPromoOffer, error) {
	p.redeems = append(p.redeems, in)
	return p.offer, p.err
}

func (p *stubPromo) ValidateForSignup(_ context.Context, in subscription.SignupPromoInput) (subscription.SignupPromoOffer, error) {
	p.validates = append(p.validates, in)
	return p.offer, p.err
}

func promoRouter(t *testing.T, svc subscription.Bootstrapper, p subscription.SignupPromoRedeemer) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	e := gin.New()
	h := subscription.NewInternalHandler(svc)
	if p != nil {
		h = h.WithPromo(p)
	}
	h.RegisterRoutes(e.Group("/internal"), testInternalSecret)
	return e
}

func decode(t *testing.T, body []byte) map[string]any {
	t.Helper()
	var out struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatalf("decode %s: %v", body, err)
	}
	return out.Data
}

func TestEnsureSubscription_RedeemsThePromoCodeAgainstTheNewRow(t *testing.T) {
	svc := &stubBootstrapper{}
	p := &stubPromo{offer: subscription.SignupPromoOffer{TrialExtensionDays: 14}}
	storeID := uuid.New()

	rec := httptest.NewRecorder()
	promoRouter(t, svc, p).ServeHTTP(rec, ensureReq(t, storeID, map[string]any{
		"tenant_id":  uuid.New().String(),
		"email":      "founder@example.com",
		"currency":   "aud",
		"promo_code": "STAYLONGER",
	}, testInternalSecret))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if len(p.redeems) != 1 {
		t.Fatalf("redeemed %d times, want 1", len(p.redeems))
	}
	// Redemption must run against the row Bootstrap just created — the ledger
	// row needs its id and the extension needs a row to move.
	if p.redeems[0].Sub == nil || p.redeems[0].Sub.StoreID != storeID {
		t.Errorf("redeemed against %+v, want the freshly bootstrapped row", p.redeems[0].Sub)
	}
	if got := decode(t, rec.Body.Bytes())["promo"].(map[string]any)["trial_extension_days"]; got != float64(14) {
		t.Errorf("trial_extension_days = %v, want 14", got)
	}
}

// Most merchants type nothing, and that must not look like a refused code.
func TestEnsureSubscription_NoPromoCodeIsNotARefusal(t *testing.T) {
	p := &stubPromo{}
	rec := httptest.NewRecorder()
	promoRouter(t, &stubBootstrapper{}, p).ServeHTTP(rec, ensureReq(t, uuid.New(), map[string]any{
		"tenant_id": uuid.New().String(),
	}, testInternalSecret))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if len(p.redeems) != 0 {
		t.Error("looked up a promo code that was never supplied")
	}
	if got := decode(t, rec.Body.Bytes())["promo"].(map[string]any)["reject_reason"]; got != "" {
		t.Errorf("reject_reason = %v, want empty", got)
	}
}

// A refused code must not fail onboarding: the store exists and the trial has
// started. It must also not be reported as applied.
func TestEnsureSubscription_ARefusedCodeStillCompletesOnboarding(t *testing.T) {
	p := &stubPromo{
		offer: subscription.SignupPromoOffer{RejectReason: "redeem_in_billing"},
		err:   errors.New("refused"),
	}
	rec := httptest.NewRecorder()
	promoRouter(t, &stubBootstrapper{}, p).ServeHTTP(rec, ensureReq(t, uuid.New(), map[string]any{
		"tenant_id":  uuid.New().String(),
		"promo_code": "WINBACK20OFF6MONTHS",
	}, testInternalSecret))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d — a refused promo code failed onboarding", rec.Code)
	}
	got := decode(t, rec.Body.Bytes())["promo"].(map[string]any)
	if got["reject_reason"] != "redeem_in_billing" {
		t.Errorf("reject_reason = %v", got["reject_reason"])
	}
	if got["trial_extension_days"] != float64(0) {
		t.Errorf("trial_extension_days = %v, want 0", got["trial_extension_days"])
	}
}

// With no promo service wired, a code must be REFUSED rather than dropped.
// Silently ignoring it would tell a merchant it applied when nothing could
// have applied it — the shape of #620's original defect.
func TestEnsureSubscription_ACodeWithNoPromoServiceIsRefusedNotIgnored(t *testing.T) {
	rec := httptest.NewRecorder()
	promoRouter(t, &stubBootstrapper{}, nil).ServeHTTP(rec, ensureReq(t, uuid.New(), map[string]any{
		"tenant_id":  uuid.New().String(),
		"promo_code": "STAYLONGER",
	}, testInternalSecret))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if got := decode(t, rec.Body.Bytes())["promo"].(map[string]any)["reject_reason"]; got != "promo_unavailable" {
		t.Errorf("reject_reason = %v, want promo_unavailable", got)
	}
}

func TestValidatePromoForSignup_ReportsTheOfferWithoutRedeeming(t *testing.T) {
	p := &stubPromo{offer: subscription.SignupPromoOffer{TrialExtensionDays: 30}}

	body, _ := json.Marshal(map[string]any{"code": "STAYLONGER", "email": "f@example.com", "currency": "aud"})
	req := httptest.NewRequest(http.MethodPost, "/internal/promo/validate-for-signup", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Internal-Auth", testInternalSecret)

	rec := httptest.NewRecorder()
	promoRouter(t, &stubBootstrapper{}, p).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	got := decode(t, rec.Body.Bytes())
	if got["valid"] != true || got["trial_extension_days"] != float64(30) {
		t.Errorf("data = %+v", got)
	}
	if len(p.redeems) != 0 {
		t.Fatal("validating consumed the code")
	}
}

// A refused code is a 200 with valid:false. Giving it a 4xx invites the caller
// to treat a successful answer as a transport failure and retry it.
func TestValidatePromoForSignup_ARefusalIsStillA200(t *testing.T) {
	p := &stubPromo{
		offer: subscription.SignupPromoOffer{RejectReason: "invalid_or_expired"},
		err:   errors.New("refused"),
	}
	body, _ := json.Marshal(map[string]any{"code": "NOPE"})
	req := httptest.NewRequest(http.MethodPost, "/internal/promo/validate-for-signup", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Internal-Auth", testInternalSecret)

	rec := httptest.NewRecorder()
	promoRouter(t, &stubBootstrapper{}, p).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	got := decode(t, rec.Body.Bytes())
	if got["valid"] != false || got["reject_reason"] != "invalid_or_expired" {
		t.Errorf("data = %+v", got)
	}
}

// #620 calls an open validate endpoint "an oracle for guessing valid codes".
// It is on /internal for that reason, and the guard is what makes that true.
func TestValidatePromoForSignup_RefusesWithoutTheInternalSecret(t *testing.T) {
	p := &stubPromo{}
	body, _ := json.Marshal(map[string]any{"code": "STAYLONGER"})
	req := httptest.NewRequest(http.MethodPost, "/internal/promo/validate-for-signup", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	rec := httptest.NewRecorder()
	promoRouter(t, &stubBootstrapper{}, p).ServeHTTP(rec, req)

	if rec.Code == http.StatusOK {
		t.Fatalf("status = %d — the code oracle is open", rec.Code)
	}
	if len(p.validates) != 0 {
		t.Fatal("validated a code for a request that failed the guard")
	}
}
