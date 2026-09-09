package admin

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/billing/appcreds"
	"github.com/mark8ly/marketplace-api/internal/subscription"
)

// fakeSubRepo satisfies the narrow subscriptionLookup interface used by
// AppCredentialsHandler. Returns (sub, err) on every call.
type fakeSubRepo struct {
	sub *subscription.StoreSubscription
	err error
}

func (f *fakeSubRepo) GetByStoreID(_ context.Context, _ *gorm.DB, _, _ uuid.UUID) (*subscription.StoreSubscription, error) {
	return f.sub, f.err
}

func makeP8Fixture(t *testing.T) []byte {
	t.Helper()
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate p256: %v", err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		t.Fatalf("marshal pkcs8: %v", err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})
}

func makeMultipart(t *testing.T, fields map[string]any) (*bytes.Buffer, string) {
	t.Helper()
	buf := &bytes.Buffer{}
	w := multipart.NewWriter(buf)
	for k, v := range fields {
		switch val := v.(type) {
		case []byte:
			fw, err := w.CreateFormFile(k, k+".bin")
			if err != nil {
				t.Fatalf("CreateFormFile: %v", err)
			}
			if _, err := fw.Write(val); err != nil {
				t.Fatalf("write file: %v", err)
			}
		case string:
			if err := w.WriteField(k, val); err != nil {
				t.Fatalf("WriteField: %v", err)
			}
		default:
			t.Fatalf("unsupported multipart field type %T", v)
		}
	}
	w.Close()
	return buf, w.FormDataContentType()
}

// router constructs a gin.Engine with StoreMiddleware + user_id/tenant_id
// injection. Simulates what HeaderTrustAuth + StoresMiddleware would set.
func router(h *AppCredentialsHandler, tenantID, userID uuid.UUID) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("tenant_id", tenantID.String())
		c.Set("user_id", userID.String())
		c.Next()
	})
	r.POST("/admin/stores/:storeId/app-credentials/apple", h.PostApple)
	r.POST("/admin/stores/:storeId/app-credentials/google", h.PostGoogle)
	return r
}

func TestPostApple_Success(t *testing.T) {
	tenantID := uuid.New()
	storeID := uuid.New()
	userID := uuid.New()
	fake := appcreds.NewFakeSM()
	svc := appcreds.NewService(appcreds.Config{ProjectID: "p", SM: fake, Emitter: nil})
	repo := &fakeSubRepo{sub: &subscription.StoreSubscription{
		TenantID: tenantID, StoreID: storeID,
		Plan: subscription.PlanPro, HasWhiteLabelAppAddOn: true,
	}}
	h := NewAppCredentialsHandler(nil, repo, svc)

	body, ct := makeMultipart(t, map[string]any{
		"p8":        makeP8Fixture(t),
		"issuer_id": "69a6de7e-aaaa-bbbb-cccc-ddddeeeeffff",
		"key_id":    "ABCD1234",
	})
	req := httptest.NewRequest(http.MethodPost, "/admin/stores/"+storeID.String()+"/app-credentials/apple", body)
	req.Header.Set("Content-Type", ct)
	w := httptest.NewRecorder()
	router(h, tenantID, userID).ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204; body: %s", w.Code, w.Body.String())
	}
	// All three credentials in the fake SM.
	for _, ct := range []appcreds.CredType{
		appcreds.CredTypeAppleP8,
		appcreds.CredTypeAppleIssuerID,
		appcreds.CredTypeAppleKeyID,
	} {
		name := appcreds.Path("p", tenantID.String(), ct)
		if !fake.Has(name) {
			t.Errorf("expected %s stored at %s, missing", ct, name)
		}
	}
}

func TestPostApple_RejectsNonProStore(t *testing.T) {
	tenantID, storeID, userID := uuid.New(), uuid.New(), uuid.New()
	fake := appcreds.NewFakeSM()
	svc := appcreds.NewService(appcreds.Config{ProjectID: "p", SM: fake, Emitter: nil})
	repo := &fakeSubRepo{sub: &subscription.StoreSubscription{
		TenantID: tenantID, StoreID: storeID,
		Plan: subscription.PlanStarter, HasWhiteLabelAppAddOn: false,
	}}
	h := NewAppCredentialsHandler(nil, repo, svc)

	body, ct := makeMultipart(t, map[string]any{
		"p8": []byte("x"), "issuer_id": "a", "key_id": "b",
	})
	req := httptest.NewRequest(http.MethodPost, "/admin/stores/"+storeID.String()+"/app-credentials/apple", body)
	req.Header.Set("Content-Type", ct)
	w := httptest.NewRecorder()
	router(h, tenantID, userID).ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", w.Code)
	}
	if !bytes.Contains(w.Body.Bytes(), []byte("add_on_not_active")) {
		t.Errorf("body = %s, want add_on_not_active", w.Body.String())
	}
}

func TestPostApple_RejectsProWithoutAddOn(t *testing.T) {
	tenantID, storeID, userID := uuid.New(), uuid.New(), uuid.New()
	fake := appcreds.NewFakeSM()
	svc := appcreds.NewService(appcreds.Config{ProjectID: "p", SM: fake, Emitter: nil})
	repo := &fakeSubRepo{sub: &subscription.StoreSubscription{
		TenantID: tenantID, StoreID: storeID,
		Plan: subscription.PlanPro, HasWhiteLabelAppAddOn: false,
	}}
	h := NewAppCredentialsHandler(nil, repo, svc)

	body, ct := makeMultipart(t, map[string]any{
		"p8": []byte("x"), "issuer_id": "a", "key_id": "b",
	})
	req := httptest.NewRequest(http.MethodPost, "/admin/stores/"+storeID.String()+"/app-credentials/apple", body)
	req.Header.Set("Content-Type", ct)
	w := httptest.NewRecorder()
	router(h, tenantID, userID).ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", w.Code)
	}
	if !bytes.Contains(w.Body.Bytes(), []byte("add_on_not_active")) {
		t.Errorf("body = %s, want add_on_not_active (not pro_plan_required — shape stays stable)", w.Body.String())
	}
}

func TestPostApple_InvalidP8_Returns400(t *testing.T) {
	tenantID, storeID, userID := uuid.New(), uuid.New(), uuid.New()
	svc := appcreds.NewService(appcreds.Config{ProjectID: "p", SM: appcreds.NewFakeSM(), Emitter: nil})
	repo := &fakeSubRepo{sub: &subscription.StoreSubscription{
		TenantID: tenantID, StoreID: storeID,
		Plan: subscription.PlanPro, HasWhiteLabelAppAddOn: true,
	}}
	h := NewAppCredentialsHandler(nil, repo, svc)

	body, ct := makeMultipart(t, map[string]any{
		"p8":        []byte("not a pem file"),
		"issuer_id": "a",
		"key_id":    "b",
	})
	req := httptest.NewRequest(http.MethodPost, "/admin/stores/"+storeID.String()+"/app-credentials/apple", body)
	req.Header.Set("Content-Type", ct)
	w := httptest.NewRecorder()
	router(h, tenantID, userID).ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
	if !bytes.Contains(w.Body.Bytes(), []byte("invalid_p8_format")) {
		t.Errorf("body = %s, want invalid_p8_format", w.Body.String())
	}
}

func TestPostApple_MissingFields(t *testing.T) {
	tenantID, storeID, userID := uuid.New(), uuid.New(), uuid.New()
	svc := appcreds.NewService(appcreds.Config{ProjectID: "p", SM: appcreds.NewFakeSM(), Emitter: nil})
	repo := &fakeSubRepo{sub: &subscription.StoreSubscription{
		TenantID: tenantID, StoreID: storeID,
		Plan: subscription.PlanPro, HasWhiteLabelAppAddOn: true,
	}}
	h := NewAppCredentialsHandler(nil, repo, svc)

	cases := []struct {
		name   string
		fields map[string]any
	}{
		{"no p8", map[string]any{"issuer_id": "a", "key_id": "b"}},
		{"no issuer_id", map[string]any{"p8": makeP8Fixture(t), "key_id": "b"}},
		{"no key_id", map[string]any{"p8": makeP8Fixture(t), "issuer_id": "a"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			body, ct := makeMultipart(t, tc.fields)
			req := httptest.NewRequest(http.MethodPost, "/admin/stores/"+storeID.String()+"/app-credentials/apple", body)
			req.Header.Set("Content-Type", ct)
			w := httptest.NewRecorder()
			router(h, tenantID, userID).ServeHTTP(w, req)

			if w.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want 400 (%s)", w.Code, tc.name)
			}
		})
	}
}

func TestPostGoogle_Success(t *testing.T) {
	tenantID, storeID, userID := uuid.New(), uuid.New(), uuid.New()
	fake := appcreds.NewFakeSM()
	svc := appcreds.NewService(appcreds.Config{ProjectID: "p", SM: fake, Emitter: nil})
	repo := &fakeSubRepo{sub: &subscription.StoreSubscription{
		TenantID: tenantID, StoreID: storeID,
		Plan: subscription.PlanPro, HasWhiteLabelAppAddOn: true,
	}}
	h := NewAppCredentialsHandler(nil, repo, svc)

	sa := []byte(`{
	  "type":"service_account","project_id":"proj",
	  "private_key":"-----BEGIN PRIVATE KEY-----\nfake\n-----END PRIVATE KEY-----",
	  "client_email":"sa@proj.iam.gserviceaccount.com"
	}`)
	body, ct := makeMultipart(t, map[string]any{"service_account_json": sa})
	req := httptest.NewRequest(http.MethodPost, "/admin/stores/"+storeID.String()+"/app-credentials/google", body)
	req.Header.Set("Content-Type", ct)
	w := httptest.NewRecorder()
	router(h, tenantID, userID).ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204; body: %s", w.Code, w.Body.String())
	}
	name := appcreds.Path("p", tenantID.String(), appcreds.CredTypeGooglePlayJSON)
	if !fake.Has(name) {
		t.Errorf("service account JSON not stored at %s", name)
	}
}

func TestPostGoogle_InvalidShape_Returns400(t *testing.T) {
	tenantID, storeID, userID := uuid.New(), uuid.New(), uuid.New()
	svc := appcreds.NewService(appcreds.Config{ProjectID: "p", SM: appcreds.NewFakeSM(), Emitter: nil})
	repo := &fakeSubRepo{sub: &subscription.StoreSubscription{
		TenantID: tenantID, StoreID: storeID,
		Plan: subscription.PlanPro, HasWhiteLabelAppAddOn: true,
	}}
	h := NewAppCredentialsHandler(nil, repo, svc)

	cases := []struct {
		name    string
		payload []byte
	}{
		{"authorized_user not allowed", []byte(`{"type":"authorized_user","client_id":"x"}`)},
		{"garbage JSON", []byte("not json")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			body, ct := makeMultipart(t, map[string]any{"service_account_json": tc.payload})
			req := httptest.NewRequest(http.MethodPost, "/admin/stores/"+storeID.String()+"/app-credentials/google", body)
			req.Header.Set("Content-Type", ct)
			w := httptest.NewRecorder()
			router(h, tenantID, userID).ServeHTTP(w, req)

			if w.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want 400 (%s)", w.Code, tc.name)
			}
			if !bytes.Contains(w.Body.Bytes(), []byte("invalid_service_account_json")) {
				t.Errorf("body = %s, want invalid_service_account_json", w.Body.String())
			}
		})
	}
}

func TestAppAddOnGate_SubRepoError(t *testing.T) {
	tenantID, storeID, userID := uuid.New(), uuid.New(), uuid.New()
	svc := appcreds.NewService(appcreds.Config{ProjectID: "p", SM: appcreds.NewFakeSM(), Emitter: nil})
	repo := &fakeSubRepo{err: errors.New("db down")}
	h := NewAppCredentialsHandler(nil, repo, svc)

	body, ct := makeMultipart(t, map[string]any{
		"p8": makeP8Fixture(t), "issuer_id": "a", "key_id": "b",
	})
	req := httptest.NewRequest(http.MethodPost,
		fmt.Sprintf("/admin/stores/%s/app-credentials/apple", storeID), body)
	req.Header.Set("Content-Type", ct)
	w := httptest.NewRecorder()
	router(h, tenantID, userID).ServeHTTP(w, req)

	// Fail-closed: repo error → 403 add_on_not_active, not 500.
	if w.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403 (fail-closed)", w.Code)
	}
}

// validGoogleSA is the shape PostGoogle accepts; the package-name tests below
// vary only the package_name field so a failure names the field it is about.
func validGoogleSA() []byte {
	return []byte(`{
	  "type":"service_account","project_id":"proj",
	  "private_key":"-----BEGIN PRIVATE KEY-----\nfake\n-----END PRIVATE KEY-----",
	  "client_email":"sa@proj.iam.gserviceaccount.com"
	}`)
}

func postGoogle(t *testing.T, fields map[string]any) (*httptest.ResponseRecorder, *appcreds.FakeSM, uuid.UUID) {
	t.Helper()
	tenantID, storeID, userID := uuid.New(), uuid.New(), uuid.New()
	fake := appcreds.NewFakeSM()
	svc := appcreds.NewService(appcreds.Config{ProjectID: "p", SM: fake, Emitter: nil})
	repo := &fakeSubRepo{sub: &subscription.StoreSubscription{
		TenantID: tenantID, StoreID: storeID,
		Plan: subscription.PlanPro, HasWhiteLabelAppAddOn: true,
	}}
	h := NewAppCredentialsHandler(nil, repo, svc)

	body, ct := makeMultipart(t, fields)
	req := httptest.NewRequest(http.MethodPost, "/admin/stores/"+storeID.String()+"/app-credentials/google", body)
	req.Header.Set("Content-Type", ct)
	w := httptest.NewRecorder()
	router(h, tenantID, userID).ServeHTTP(w, req)
	return w, fake, tenantID
}

// package_name — the optional Android applicationId (#872).

func TestPostGoogle_StoresPackageName(t *testing.T) {
	w, fake, tenantID := postGoogle(t, map[string]any{
		"service_account_json": validGoogleSA(),
		"package_name":         "com.example.storefront",
	})
	if w.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204; body: %s", w.Code, w.Body.String())
	}
	name := appcreds.Path("p", tenantID.String(), appcreds.CredTypeGooglePackageName)
	if !fake.Has(name) {
		t.Fatalf("package name not stored at %s", name)
	}
}

func TestPostGoogle_PackageNameIsOptional(t *testing.T) {
	// It HAS to be optional: this endpoint is behind appAddOnGate, which
	// requires an active Pro+App subscription, so the sale always precedes
	// the upload and no earlier point could have required a package name.
	// Requiring it here would also lock existing merchants out of
	// re-uploading their service account.
	w, fake, tenantID := postGoogle(t, map[string]any{
		"service_account_json": validGoogleSA(),
	})
	if w.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204; body: %s", w.Code, w.Body.String())
	}
	if fake.Has(appcreds.Path("p", tenantID.String(), appcreds.CredTypeGooglePackageName)) {
		t.Error("no package name was supplied; nothing should have been written for it")
	}
	if !fake.Has(appcreds.Path("p", tenantID.String(), appcreds.CredTypeGooglePlayJSON)) {
		t.Error("the service account must still be stored")
	}
}

func TestPostGoogle_RejectsMalformedPackageName(t *testing.T) {
	// And critically: rejects it WITHOUT storing the service account, so a
	// merchant never has to discover a half-applied upload by retrying.
	for _, bad := range []string{"com", "com.example.my-app", "com.1example", "not a package"} {
		t.Run(bad, func(t *testing.T) {
			w, fake, tenantID := postGoogle(t, map[string]any{
				"service_account_json": validGoogleSA(),
				"package_name":         bad,
			})
			if w.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400; body: %s", w.Code, w.Body.String())
			}
			if fake.Has(appcreds.Path("p", tenantID.String(), appcreds.CredTypeGooglePlayJSON)) {
				t.Error("service account was stored despite the request being rejected")
			}
		})
	}
}

func TestPostGoogle_PackageNameErrorNamesTheField(t *testing.T) {
	// The service-account failure already answers "invalid_service_account_json".
	// A package-name failure answering the same thing would send a merchant to
	// re-export a credential that was fine.
	w, _, _ := postGoogle(t, map[string]any{
		"service_account_json": validGoogleSA(),
		"package_name":         "nope",
	})
	if got := w.Body.String(); !strings.Contains(got, "invalid_package_name") {
		t.Errorf("body = %s, want it to name invalid_package_name", got)
	}
}
