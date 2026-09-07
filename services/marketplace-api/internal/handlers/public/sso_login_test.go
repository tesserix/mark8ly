package public_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/authbffclient"
	"github.com/mark8ly/marketplace-api/internal/handlers/public"
	"github.com/mark8ly/marketplace-api/internal/sso"
)

func init() {
	gin.SetMode(gin.TestMode)
}

// ---------------------------------------------------------------------------
// Fake TenantResolver
// ---------------------------------------------------------------------------

type fakeTenantResolver struct {
	slugToID map[string]uuid.UUID
}

func (f *fakeTenantResolver) ByTenantSlug(_ context.Context, slug string) (uuid.UUID, error) {
	id, ok := f.slugToID[slug]
	if !ok {
		return uuid.Nil, public.ErrTenantNotFound
	}
	return id, nil
}

// ---------------------------------------------------------------------------
// Fake JIT provisioner
// ---------------------------------------------------------------------------

type fakeJIT struct {
	out sso.JITOutput
	err error
}

func (f *fakeJIT) Provision(_ context.Context, _ sso.JITInput) (sso.JITOutput, error) {
	return f.out, f.err
}

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

func buildTestRouter(h *public.SSOLoginHandler) *gin.Engine {
	r := gin.New()
	public.RegisterPublic(r.Group("/"), public.PublicDeps{SSOLoginHandler: h})
	return r
}

func doTestGet(r *gin.Engine, path string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func doTestForm(r *gin.Engine, path string, vals url.Values) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(vals.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// ---------------------------------------------------------------------------
// No config for slug → 404
// ---------------------------------------------------------------------------

func TestSSOLogin_UnknownSlug_Returns404(t *testing.T) {
	resolver := &fakeTenantResolver{slugToID: map[string]uuid.UUID{}}
	h := public.NewSSOLoginHandler(nil, nil, authbffclient.NoopIssuer{}, nil,
		public.NewMemStateCache(), nil, resolver, nil)

	r := buildTestRouter(h)
	w := doTestGet(r, "/sso/unknown-tenant/login")
	require.Equal(t, http.StatusNotFound, w.Code)
}

func TestSSOCallback_UnknownSlug_Returns404(t *testing.T) {
	resolver := &fakeTenantResolver{slugToID: map[string]uuid.UUID{}}
	h := public.NewSSOLoginHandler(nil, nil, authbffclient.NoopIssuer{}, nil,
		public.NewMemStateCache(), nil, resolver, nil)

	r := buildTestRouter(h)
	w := doTestForm(r, "/sso/unknown-tenant/callback", url.Values{
		"state": {"some-state"},
		"code":  {"some-code"},
	})
	require.Equal(t, http.StatusNotFound, w.Code)
}

// ---------------------------------------------------------------------------
// OIDC login: nil RP → 503 (provider not ready)
// ---------------------------------------------------------------------------

func TestSSOLogin_OIDC_NilRP_Returns503(t *testing.T) {
	tenantID := uuid.New()
	resolver := &fakeTenantResolver{slugToID: map[string]uuid.UUID{"acme": tenantID}}
	cache := public.NewMemStateCache()

	// Repo is nil; handler reaches oidcRP check before any repo call because
	// loadConfig is called first. We need a working repo stub. Since sso.Repository
	// is a concrete GORM type, use the TenantResolver-returns-not-found trick
	// to verify the 404 branch, and a separate router for the 503 branch where we
	// need the config to load. For the 503 test we use the inline config router.
	h := public.NewSSOLoginHandler(nil, &fakeJIT{},
		authbffclient.NoopIssuer{}, nil, // nil OIDCRPs map
		cache, nil, resolver, nil)

	// Without a working repo, loadConfig returns 503 (not_configured) first.
	r := buildTestRouter(h)
	w := doTestGet(r, "/sso/acme/login")
	// Repo is nil → handler returns 503 at the loadConfig step.
	require.Equal(t, http.StatusServiceUnavailable, w.Code)
}

// ---------------------------------------------------------------------------
// OIDC callback — wrong state → 401
// ---------------------------------------------------------------------------

// Rewritten by #820. It used to assert 503, with a comment explaining that a
// config could not be loaded without a database — so a test named
// "wrong state → 401" proved only that a nil repository short-circuits. Now
// that Repo is an interface it asserts the state check it was written for.
func TestSSOCallback_OIDC_WrongState_Returns401(t *testing.T) {
	tenantID := uuid.New()
	resolver := &fakeTenantResolver{slugToID: map[string]uuid.UUID{"acme": tenantID}}
	loader := &fakeConfigLoader{cfg: &sso.Config{
		TenantID: tenantID, Provider: sso.ProviderOIDC, Enabled: true,
	}}

	h := public.NewSSOLoginHandler(loader, &fakeJIT{}, authbffclient.NoopIssuer{}, nil,
		public.NewMemStateCache(), nil, resolver, nil)

	w := doTestForm(buildTestRouter(h), "/sso/acme/callback", url.Values{
		"state": {"never-stored"},
		"code":  {"some-code"},
	})

	require.Equal(t, http.StatusUnauthorized, w.Code)
	require.Contains(t, w.Body.String(), "auth_failed")
}

// A relying party that cannot be built is the tenant's configuration or their
// IdP being unreachable — recoverable without a code change, so 503 rather
// than 500. The response must not carry the reason: it names the customer's
// issuer and the path of their client secret.
func TestSSOLogin_OIDC_UnbuildableRelyingParty_Returns503WithoutDetail(t *testing.T) {
	tenantID := uuid.New()
	resolver := &fakeTenantResolver{slugToID: map[string]uuid.UUID{"acme": tenantID}}
	loader := &fakeConfigLoader{cfg: &sso.Config{
		TenantID: tenantID, Provider: sso.ProviderOIDC, Enabled: true,
	}}
	rps := &fakeRelyingParties{err: errors.New("discover https://idp.acme.example: dial tcp: refused")}

	h := public.NewSSOLoginHandler(loader, &fakeJIT{}, authbffclient.NoopIssuer{}, rps,
		public.NewMemStateCache(), nil, resolver, nil)

	w := doTestGet(buildTestRouter(h), "/sso/acme/login")

	require.Equal(t, http.StatusServiceUnavailable, w.Code)
	require.NotContains(t, w.Body.String(), "idp.acme.example",
		"the response leaked the tenant's issuer URL")
	require.NotContains(t, w.Body.String(), "dial tcp")
}

// The relying party is fetched per request from the source, not held in a map
// built at boot — which is what lets a tenant who configures SSO after startup
// actually sign in.
func TestSSOLogin_OIDC_AsksForTheRelyingPartyPerRequest(t *testing.T) {
	tenantID := uuid.New()
	resolver := &fakeTenantResolver{slugToID: map[string]uuid.UUID{"acme": tenantID}}
	loader := &fakeConfigLoader{cfg: &sso.Config{
		TenantID: tenantID, Provider: sso.ProviderOIDC, Enabled: true,
	}}
	rps := &fakeRelyingParties{err: errors.New("not ready")}

	h := public.NewSSOLoginHandler(loader, &fakeJIT{}, authbffclient.NoopIssuer{}, rps,
		public.NewMemStateCache(), nil, resolver, nil)
	r := buildTestRouter(h)

	doTestGet(r, "/sso/acme/login")
	doTestGet(r, "/sso/acme/login")

	require.Equal(t, 2, rps.calls, "the handler cached a relying party itself")
	require.Equal(t, tenantID, rps.lastCfg.TenantID)
}

// ---------------------------------------------------------------------------
// Logout — always clears the session cookie (even on unknown slug)
// ---------------------------------------------------------------------------

func TestSSOLogout_ClearsSessionCookie(t *testing.T) {
	resolver := &fakeTenantResolver{slugToID: map[string]uuid.UUID{}}
	h := public.NewSSOLoginHandler(nil, nil, authbffclient.NoopIssuer{}, nil,
		public.NewMemStateCache(), nil, resolver, nil)

	r := buildTestRouter(h)
	req := httptest.NewRequest(http.MethodPost, "/sso/acme/logout", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusNoContent, w.Code)
	cookies := w.Header().Values("Set-Cookie")
	require.NotEmpty(t, cookies, "logout must write a cookie-clearing header")
	require.Contains(t, cookies[0], "Max-Age=0", "logout cookie must be expired")
}

// ---------------------------------------------------------------------------
// StateCache unit tests
// ---------------------------------------------------------------------------

func TestMemStateCache_PutAndGet(t *testing.T) {
	cache := public.NewMemStateCache()
	tid := uuid.New()

	require.NoError(t, cache.Put("state1", "nonce1", tid))

	nonce, gotTenant, err := cache.Get("state1")
	require.NoError(t, err)
	require.Equal(t, "nonce1", nonce)
	require.Equal(t, tid, gotTenant)
}

func TestMemStateCache_GetUnknownState_ReturnsError(t *testing.T) {
	cache := public.NewMemStateCache()
	_, _, err := cache.Get("does-not-exist")
	require.Error(t, err)
}

func TestMemStateCache_GetIsOneTimeUse(t *testing.T) {
	cache := public.NewMemStateCache()
	tid := uuid.New()
	require.NoError(t, cache.Put("s", "n", tid))

	_, _, err1 := cache.Get("s")
	require.NoError(t, err1)

	// Second get must fail — entry was consumed.
	_, _, err2 := cache.Get("s")
	require.Error(t, err2)
}

func TestMemStateCache_EmptyStateOrNonce_ReturnsError(t *testing.T) {
	cache := public.NewMemStateCache()
	tid := uuid.New()

	require.Error(t, cache.Put("", "nonce", tid), "empty state must fail")
	require.Error(t, cache.Put("state", "", tid), "empty nonce must fail")
}

// ---------------------------------------------------------------------------
// Config-dependent branches (#820)
//
// These could not be written before: SSOLoginHandler.Repo was a concrete
// *sso.Repository, so every test could only reach the paths that fail BEFORE
// the config is read. That is how loginSAML shipped with an empty body — a
// green suite that never once loaded a config.
// ---------------------------------------------------------------------------

type fakeRelyingParties struct {
	rp      *sso.OIDCRelyingParty
	err     error
	calls   int
	lastCfg *sso.Config
}

func (f *fakeRelyingParties) For(_ context.Context, cfg *sso.Config) (*sso.OIDCRelyingParty, error) {
	f.calls++
	f.lastCfg = cfg
	return f.rp, f.err
}

type fakeConfigLoader struct {
	cfg *sso.Config
	err error
}

func (f *fakeConfigLoader) GetByTenant(_ context.Context, _ uuid.UUID) (*sso.Config, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.cfg, nil
}

func samlHandler(t *testing.T, tenantID uuid.UUID) *gin.Engine {
	t.Helper()
	resolver := &fakeTenantResolver{slugToID: map[string]uuid.UUID{"acme": tenantID}}
	loader := &fakeConfigLoader{cfg: &sso.Config{
		TenantID: tenantID,
		Provider: sso.ProviderSAML,
		Enabled:  true,
	}}
	h := public.NewSSOLoginHandler(loader, &fakeJIT{}, authbffclient.NoopIssuer{}, nil,
		public.NewMemStateCache(), nil, resolver, nil)
	return buildTestRouter(h)
}

// A SAML tenant used to get 200 with an empty body from the login route: a
// blank page to the merchant, a successful login to any monitor counting
// statuses. An unimplemented path has to fail like one.
func TestSSOLogin_SAML_IsRefusedNotSilentlyEmpty(t *testing.T) {
	w := doTestGet(samlHandler(t, uuid.New()), "/sso/acme/login")

	require.Equal(t, http.StatusNotImplemented, w.Code)
	require.Contains(t, w.Body.String(), "saml_not_implemented")
}

// The two SAML branches must agree. They did not: callback answered 501 while
// login answered an empty 200, so which one a merchant hit decided whether SSO
// looked broken or looked fine.
func TestSSOLogin_SAML_LoginAndCallbackAgree(t *testing.T) {
	tenantID := uuid.New()

	login := doTestGet(samlHandler(t, tenantID), "/sso/acme/login")
	callback := doTestForm(samlHandler(t, tenantID), "/sso/acme/callback", url.Values{
		"SAMLResponse": {"whatever"},
	})

	require.Equal(t, callback.Code, login.Code,
		"login and callback disagree about whether SAML works")
	require.Equal(t, http.StatusNotImplemented, login.Code)
}

// A disabled config is a 404, and must not be distinguishable from "no such
// tenant" — otherwise the route enumerates which tenants have bought SSO.
func TestSSOLogin_DisabledConfigIsIndistinguishableFromAMissingTenant(t *testing.T) {
	tenantID := uuid.New()
	resolver := &fakeTenantResolver{slugToID: map[string]uuid.UUID{"acme": tenantID}}
	loader := &fakeConfigLoader{cfg: &sso.Config{
		TenantID: tenantID, Provider: sso.ProviderOIDC, Enabled: false,
	}}
	h := public.NewSSOLoginHandler(loader, &fakeJIT{}, authbffclient.NoopIssuer{}, nil,
		public.NewMemStateCache(), nil, resolver, nil)

	disabled := doTestGet(buildTestRouter(h), "/sso/acme/login")

	unknownResolver := &fakeTenantResolver{slugToID: map[string]uuid.UUID{}}
	unknown := doTestGet(buildTestRouter(public.NewSSOLoginHandler(loader, &fakeJIT{},
		authbffclient.NoopIssuer{}, nil, public.NewMemStateCache(), nil, unknownResolver, nil)),
		"/sso/nobody/login")

	require.Equal(t, http.StatusNotFound, disabled.Code)
	require.Equal(t, unknown.Code, disabled.Code)
	require.Equal(t, unknown.Body.String(), disabled.Body.String(),
		"a disabled tenant answers differently from an unknown one — that enumerates customers")
}

// A typed nil repository must not panic. An interface holding a nil pointer is
// itself non-nil, so the handler's `Repo == nil` guard would pass and the
// first call would panic inside an unauthenticated route (#288's shape).
func TestNewSSOLoginHandler_TypedNilRepoDoesNotPanic(t *testing.T) {
	var typedNil *sso.Repository
	resolver := &fakeTenantResolver{slugToID: map[string]uuid.UUID{"acme": uuid.New()}}
	h := public.NewSSOLoginHandler(typedNil, &fakeJIT{}, authbffclient.NoopIssuer{}, nil,
		public.NewMemStateCache(), nil, resolver, nil)

	w := doTestGet(buildTestRouter(h), "/sso/acme/login")
	require.Equal(t, http.StatusServiceUnavailable, w.Code)
}
