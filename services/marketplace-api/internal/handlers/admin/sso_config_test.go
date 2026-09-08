package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"

	"github.com/mark8ly/marketplace-api/internal/sso"
)

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func init() {
	gin.SetMode(gin.TestMode)
}

// newSSOTestRouter builds a minimal gin engine for SSO config handler tests.
// tenantID is set as the authenticated tenant (simulating auth middleware).
// pathTenantID is the :tenantId value in the URL.
func newSSOTestRouter(handler *SSOConfigHandler, authedTenant uuid.UUID) *gin.Engine {
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("tenant_id", authedTenant.String())
		c.Next()
	})
	g := r.Group("/admin/tenants/:tenantId/sso")
	g.POST("/config", handler.Upsert)
	g.GET("/config", handler.Get)
	g.DELETE("/config", handler.Delete)
	g.POST("/test", handler.Test)
	return r
}

func doJSON(r *gin.Engine, method, path string, body interface{}) *httptest.ResponseRecorder {
	var buf bytes.Buffer
	if body != nil {
		_ = json.NewEncoder(&buf).Encode(body)
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// stubSSOStore stands in for *sso.Repository so the branches that depend on a
// LOADED config can be tested without a database (#820).
type stubSSOStore struct {
	cfg *sso.Config
	err error
}

func (s *stubSSOStore) GetByTenant(context.Context, uuid.UUID) (*sso.Config, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.cfg, nil
}
func (s *stubSSOStore) Upsert(context.Context, *sso.Config) error { return nil }
func (s *stubSSOStore) Delete(context.Context, uuid.UUID) error   { return nil }

// oidcConfig is a configuration that passes sso.Validate — the three fields a
// login actually needs.
func oidcConfig() *sso.Config {
	return &sso.Config{
		Provider: sso.ProviderOIDC,
		Enabled:  true,
		Metadata: datatypes.JSONMap{
			sso.OIDCKeyIssuer:          "https://idp.acme.example",
			sso.OIDCKeyClientID:        "client-abc",
			sso.OIDCKeyClientSecretRef: "kv/acme/idp",
		},
	}
}

// runSSOTest drives POST /sso/test through the real router and returns the
// response plus the tenant it was called for.
func runSSOTest(t *testing.T, cfg *sso.Config, rps SSORelyingParties) (*httptest.ResponseRecorder, uuid.UUID) {
	t.Helper()
	tenantID := uuid.New()
	cfg.TenantID = tenantID

	h := NewSSOConfigHandler(&stubSSOStore{cfg: cfg}, nil, nil)
	if rps != nil {
		h = h.WithRelyingParties(rps)
	}
	r := newSSOTestRouter(h, tenantID)
	return doJSON(r, http.MethodPost, "/admin/tenants/"+tenantID.String()+"/sso/test", nil), tenantID
}

// ---------------------------------------------------------------------------
// Cross-tenant isolation tests (no DB needed — path vs authed tenant)
// ---------------------------------------------------------------------------

func TestSSOConfig_CrossTenantRead_Rejected(t *testing.T) {
	tenantA := uuid.New()
	tenantB := uuid.New()

	// Handler wired with nil repo — the tenant-match check fires before any DB call.
	h := NewSSOConfigHandler(nil, nil, nil)

	// Auth as tenant B, request tenant A's config.
	router := newSSOTestRouter(h, tenantB)
	w := doJSON(router, http.MethodGet, "/admin/tenants/"+tenantA.String()+"/sso/config", nil)
	require.Equal(t, http.StatusForbidden, w.Code)
}

func TestSSOConfig_CrossTenantUpsert_Rejected(t *testing.T) {
	tenantA := uuid.New()
	tenantB := uuid.New()

	h := NewSSOConfigHandler(nil, nil, nil)
	router := newSSOTestRouter(h, tenantB)

	body := map[string]any{
		"provider": "saml",
		"metadata": map[string]any{"idp_entity_id": "x"},
		"enabled":  true,
	}
	w := doJSON(router, http.MethodPost, "/admin/tenants/"+tenantA.String()+"/sso/config", body)
	require.Equal(t, http.StatusForbidden, w.Code)
}

func TestSSOConfig_CrossTenantDelete_Rejected(t *testing.T) {
	tenantA := uuid.New()
	tenantB := uuid.New()

	h := NewSSOConfigHandler(nil, nil, nil)
	router := newSSOTestRouter(h, tenantB)

	w := doJSON(router, http.MethodDelete, "/admin/tenants/"+tenantA.String()+"/sso/config", nil)
	require.Equal(t, http.StatusForbidden, w.Code)
}

// ---------------------------------------------------------------------------
// Validation tests (no DB — fail before any repo call)
// ---------------------------------------------------------------------------

func TestSSOConfig_InvalidAttrMapping_Returns400(t *testing.T) {
	tenantID := uuid.New()
	h := NewSSOConfigHandler(nil, nil, nil)
	router := newSSOTestRouter(h, tenantID)

	// attr_mapping value doesn't start with "claims." — must be rejected.
	body := map[string]any{
		"provider": "oidc",
		"metadata": map[string]any{
			"issuer":        "https://idp.example.com",
			"client_id":     "cid",
			"discovery_url": "https://idp.example.com/.well-known/openid-configuration",
		},
		"attr_mapping": map[string]string{
			"email": "not-a-claims-path",
		},
		"enabled": true,
	}
	w := doJSON(router, http.MethodPost, "/admin/tenants/"+tenantID.String()+"/sso/config", body)
	require.Equal(t, http.StatusBadRequest, w.Code)

	var resp map[string]any
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	require.Equal(t, "invalid_attr_mapping", resp["error"])
}

func TestSSOConfig_MissingRequiredMetadata_Returns400(t *testing.T) {
	tenantID := uuid.New()
	h := NewSSOConfigHandler(nil, nil, nil)
	router := newSSOTestRouter(h, tenantID)

	// SAML config missing idp_cert_pem.
	body := map[string]any{
		"provider": "saml",
		"metadata": map[string]any{
			"idp_entity_id": "https://idp.example.com",
			"idp_acs_url":   "https://idp.example.com/sso",
			// idp_cert_pem missing
		},
		"enabled": false,
	}
	w := doJSON(router, http.MethodPost, "/admin/tenants/"+tenantID.String()+"/sso/config", body)
	require.Equal(t, http.StatusBadRequest, w.Code)
}

func TestSSOConfig_InvalidProvider_Returns400(t *testing.T) {
	tenantID := uuid.New()
	h := NewSSOConfigHandler(nil, nil, nil)
	router := newSSOTestRouter(h, tenantID)

	body := map[string]any{
		"provider": "oauth1", // not valid
		"metadata": map[string]any{"foo": "bar"},
	}
	w := doJSON(router, http.MethodPost, "/admin/tenants/"+tenantID.String()+"/sso/config", body)
	// provider validation happens inside sso.Validate → 400
	require.Equal(t, http.StatusBadRequest, w.Code)
}

func TestSSOConfig_InvalidTenantIDInPath_Returns400(t *testing.T) {
	tenantID := uuid.New()
	h := NewSSOConfigHandler(nil, nil, nil)

	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("tenant_id", tenantID.String())
		c.Next()
	})
	r.GET("/admin/tenants/:tenantId/sso/config", h.Get)

	req := httptest.NewRequest(http.MethodGet, "/admin/tenants/not-a-uuid/sso/config", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusBadRequest, w.Code)
}

// ---------------------------------------------------------------------------
// redactConfig unit test
// ---------------------------------------------------------------------------

func TestRedactConfig_RemovesSecrets(t *testing.T) {
	tid := uuid.New()

	cfg := &ssoTestConfig{
		tenantID: tid,
		metadata: map[string]any{
			"issuer":            "https://idp.example.com",
			"client_id":         "cid-123",
			"client_secret_ref": "projects/123/secrets/my-secret/versions/latest",
			"discovery_url":     "https://idp.example.com/.well-known/openid-configuration",
		},
		provider: "oidc",
	}

	result := redactConfig(cfg.toConfig())

	meta, ok := result["metadata"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "[redacted]", meta["client_secret_ref"], "client_secret_ref must be redacted")
	require.Equal(t, "https://idp.example.com", meta["issuer"])
	require.Equal(t, "cid-123", meta["client_id"])
}

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

// ssoTestConfig is a lightweight builder for sso.Config values in tests,
// avoiding direct GORM dependencies.
type ssoTestConfig struct {
	tenantID uuid.UUID
	provider sso.Provider
	metadata map[string]any
}

func (s *ssoTestConfig) toConfig() *sso.Config {
	m := datatypes.JSONMap{}
	for k, v := range s.metadata {
		m[k] = v
	}
	return &sso.Config{
		TenantID: s.tenantID,
		Provider: s.provider,
		Metadata: m,
	}
}

// ---------------------------------------------------------------------------
// Test endpoint (#820)
//
// Before this, Test re-ran sso.Validate — the same check Upsert had already
// run to accept the config — so it answered "ok" for a configuration that had
// never been tried against the IdP at all. A merchant pressing "Test
// connection" is asking about the two things that actually fail: can we reach
// your IdP, and can we read your client secret.
// ---------------------------------------------------------------------------

type stubRelyingParties struct {
	err         error
	invalidated []uuid.UUID
	built       int
}

func (s *stubRelyingParties) For(context.Context, *sso.Config) (*sso.OIDCRelyingParty, error) {
	s.built++
	return nil, s.err
}

func (s *stubRelyingParties) Invalidate(tenantID uuid.UUID) {
	s.invalidated = append(s.invalidated, tenantID)
}

func TestSSOConfigTest_BuildsTheRelyingPartyAndReportsWhatItChecked(t *testing.T) {
	rps := &stubRelyingParties{}
	w, tenantID := runSSOTest(t, oidcConfig(), rps)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.Contains(t, w.Body.String(), `"checked":"idp"`,
		"a pass must say the IdP was reached, not just that the shape was valid")
	require.Equal(t, 1, rps.built)
	// Invalidated FIRST, or Test reports on a party built before the
	// merchant's last edit — which is precisely when someone presses Test.
	require.Equal(t, []uuid.UUID{tenantID}, rps.invalidated)
}

func TestSSOConfigTest_AnUnreachableIdPFails(t *testing.T) {
	rps := &stubRelyingParties{err: errors.New("discover https://idp.acme.example: dial tcp: refused")}
	w, _ := runSSOTest(t, oidcConfig(), rps)

	require.Equal(t, http.StatusUnprocessableEntity, w.Code)
	require.Contains(t, w.Body.String(), "could not reach the identity provider")
	// The cause names the tenant's issuer; it belongs in the log, not in a
	// browser.
	require.NotContains(t, w.Body.String(), "idp.acme.example")
	require.NotContains(t, w.Body.String(), "dial tcp")
}

// The two failures need different fixes, so they must not read the same.
func TestSSOConfigTest_AMissingClientSecretSaysSo(t *testing.T) {
	rps := &stubRelyingParties{err: fmt.Errorf("read kv/acme/idp: %w", sso.ErrNoClientSecret)}
	w, _ := runSSOTest(t, oidcConfig(), rps)

	require.Equal(t, http.StatusUnprocessableEntity, w.Code)
	require.Contains(t, w.Body.String(), "client secret")
	require.NotContains(t, w.Body.String(), "kv/acme/idp", "the secret path leaked to the browser")
}

// SAML has no SP. Reporting a pass for it would tell a merchant their
// configuration works when no login can ever complete.
func TestSSOConfigTest_SAMLIsNeverAPass(t *testing.T) {
	cfg := oidcConfig()
	cfg.Provider = sso.ProviderSAML
	cfg.Metadata = datatypes.JSONMap{
		sso.SAMLKeyIDPEntityID: "urn:acme",
		sso.SAMLKeyIDPACSURL:   "https://idp.example.com/acs",
		sso.SAMLKeyIDPCertPEM:  "-----BEGIN CERTIFICATE-----",
	}

	w, _ := runSSOTest(t, cfg, &stubRelyingParties{})

	require.Equal(t, http.StatusUnprocessableEntity, w.Code)
	require.Contains(t, w.Body.String(), "not available")
}

// A build with no relying-party source can still shape-check, but it must say
// that is all it did rather than claiming the IdP was reached.
func TestSSOConfigTest_WithoutARelyingPartySourceSaysWhatItChecked(t *testing.T) {
	w, _ := runSSOTest(t, oidcConfig(), nil)

	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), `"checked":"configuration"`)
	require.NotContains(t, w.Body.String(), `"checked":"idp"`)
}
