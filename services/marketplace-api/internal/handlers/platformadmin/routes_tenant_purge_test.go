package platformadmin_test

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/audit"
	"github.com/mark8ly/marketplace-api/internal/authz"
	"github.com/mark8ly/marketplace-api/internal/handlers/admin"
	"github.com/mark8ly/marketplace-api/internal/handlers/platformadmin"
	"github.com/mark8ly/marketplace-api/internal/tenantdirectory"
)

// withCurrentTimestamp overrides the request timestamp to the real wall
// clock. Register()'s RequirePlatformAuth has no injectable Now (unlike
// the middleware-only tests in middleware_test.go, which build their own
// router with a fixed clock), so a request that goes through the real
// Register()-mounted router must be signed against the real time or it
// falls outside the freshness window.
func withCurrentTimestamp(in *platformadmin.SignatureInput) {
	in.Timestamp = strconv.FormatInt(time.Now().Unix(), 10)
}

// fullPurgeDeps returns a Deps literal with every dependency the purge
// routes require present and wired, so individual tests can start from a
// fully-mounted baseline and knock out one field at a time.
//
// TenantLifecycle is wired here too, even though none of the tests in this
// file otherwise exercise suspend/unsuspend: tenant_lifecycle.go and
// tenant_purge.go both register /admin/tenants/:id/... on the SAME
// platformadmin group, using :id in both files independently. Nothing
// enforces that the two files agree on the wildcard name — if one drifted
// (e.g. someone renamed tenant_lifecycle.go's :id to :tenantID), gin would
// panic at Register() time on the SHARED group. Mounting both here means
// every test built on fullPurgeDeps() incidentally re-proves that
// agreement on every run, rather than only the one dedicated collision
// test below.
func fullPurgeDeps() platformadmin.Deps {
	return platformadmin.Deps{
		Repo:                  &stubRepo{},
		Secret:                testSecret,
		DB:                    &gorm.DB{},
		NonceStore:            newMemNonces(),
		TenantTeardown:        &fakeTeardown{seq: &seq{}},
		Purger:                &fakePurger{seq: &seq{}},
		TenantDirectory:       &stubDirectory{detail: &tenantdirectory.TenantDetail{}},
		Emitter:               audit.NewEmitter(audit.EmitterConfig{Repo: &recordingRepo{}}),
		TenantLifecycle:       &stubLifecycle{},
	}
}

// TestRegister_MountsPurgeRoutesWhenWired: every dependency present ->
// both routes resolve (not 404) for a validly-signed request.
func TestRegister_MountsPurgeRoutesWhenWired(t *testing.T) {
	gin.SetMode(gin.TestMode)

	r := gin.New()
	platformadmin.Register(r.Group("/api/v1/platform"), fullPurgeDeps())

	req := signedRequest(t, http.MethodGet, "/api/v1/platform/admin/tenants/"+tenantID+"/purge/preview", nil, withCurrentTimestamp)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.NotEqual(t, http.StatusNotFound, rec.Code, "preview must be mounted: %s", rec.Body.String())
}

// TestRegister_PurgeRequiresOperatorAndCapability: a write with no operator
// is 401 operator_required; with no capability, 401 capability_required.
// Both cells, not one — they are different failures in the enforcement
// matrix (middleware.go).
func TestRegister_PurgeRequiresOperatorAndCapability(t *testing.T) {
	gin.SetMode(gin.TestMode)

	r := gin.New()
	platformadmin.Register(r.Group("/api/v1/platform"), fullPurgeDeps())

	target := "/api/v1/platform/admin/tenants/" + tenantID + "/purge"
	body := []byte(`{"store_slugs":[],"reason_code":"merchant_request"}`)

	reqNoOperator := signedRequest(t, http.MethodPost, target, body, withoutOperator, withCurrentTimestamp)
	recNoOperator := httptest.NewRecorder()
	r.ServeHTTP(recNoOperator, reqNoOperator)
	require.Equal(t, http.StatusUnauthorized, recNoOperator.Code)
	require.Equal(t, "operator_required", errorCode(t, recNoOperator))

	reqNoCapability := signedRequest(t, http.MethodPost, target, body, withoutCapability, withCurrentTimestamp)
	recNoCapability := httptest.NewRecorder()
	r.ServeHTTP(recNoCapability, reqNoCapability)
	require.Equal(t, http.StatusUnauthorized, recNoCapability.Code)
	require.Equal(t, "capability_required", errorCode(t, recNoCapability))
}

// TestRegister_PreviewDoesNotRequireOperator: the preview is a READ —
// signature only, no operator, no capability required.
func TestRegister_PreviewDoesNotRequireOperator(t *testing.T) {
	gin.SetMode(gin.TestMode)

	r := gin.New()
	platformadmin.Register(r.Group("/api/v1/platform"), fullPurgeDeps())

	target := "/api/v1/platform/admin/tenants/" + tenantID + "/purge/preview"
	req := signedRequest(t, http.MethodGet, target, nil, withoutOperator, withoutCapability, withCurrentTimestamp)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.NotEqual(t, http.StatusUnauthorized, rec.Code, "preview must not require operator/capability: %s", rec.Body.String())
}

// TestRegister_DoesNotMountPurgeWithoutAnEmitter: a handler that cannot
// audit must not exist on this surface at all — #287's rule, mattering
// more here because the action is irreversible.
func TestRegister_DoesNotMountPurgeWithoutAnEmitter(t *testing.T) {
	gin.SetMode(gin.TestMode)

	deps := fullPurgeDeps()
	deps.Emitter = nil

	r := gin.New()
	platformadmin.Register(r.Group("/api/v1/platform"), deps)

	req := signedRequest(t, http.MethodGet, "/api/v1/platform/admin/tenants/"+tenantID+"/purge/preview", nil, withCurrentTimestamp)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusNotFound, rec.Code, "route must not mount without an Emitter")
}

// TestRegister_DoesNotMountPurgeWithoutTeardownOrPurger: each of
// TenantTeardown and Purger, nil in turn, must leave both routes
// unmounted.
func TestRegister_DoesNotMountPurgeWithoutTeardownOrPurger(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cases := map[string]func(*platformadmin.Deps){
		"nil TenantTeardown": func(d *platformadmin.Deps) { d.TenantTeardown = nil },
		"nil Purger":         func(d *platformadmin.Deps) { d.Purger = nil },
	}

	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			deps := fullPurgeDeps()
			mutate(&deps)

			r := gin.New()
			platformadmin.Register(r.Group("/api/v1/platform"), deps)

			previewReq := signedRequest(t, http.MethodGet, "/api/v1/platform/admin/tenants/"+tenantID+"/purge/preview", nil, withCurrentTimestamp)
			previewRec := httptest.NewRecorder()
			r.ServeHTTP(previewRec, previewReq)
			require.Equal(t, http.StatusNotFound, previewRec.Code, "preview must not mount")

			purgeReq := signedRequest(t, http.MethodPost, "/api/v1/platform/admin/tenants/"+tenantID+"/purge",
				[]byte(`{"store_slugs":[],"reason_code":"merchant_request"}`), withCurrentTimestamp)
			purgeRec := httptest.NewRecorder()
			r.ServeHTTP(purgeRec, purgeReq)
			require.Equal(t, http.StatusNotFound, purgeRec.Code, "purge must not mount")
		})
	}
}

// TestRegister_BogusSiblingUnderTenantsStays404: a bogus sibling under the
// same prefix must stay 404. This is what makes "the route is mounted"
// mean something rather than "this prefix answers".
func TestRegister_BogusSiblingUnderTenantsStays404(t *testing.T) {
	gin.SetMode(gin.TestMode)

	r := gin.New()
	platformadmin.Register(r.Group("/api/v1/platform"), fullPurgeDeps())

	req := signedRequest(t, http.MethodPost, "/api/v1/platform/admin/tenants/"+tenantID+"/incinerate", []byte(`{}`), withCurrentTimestamp)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusNotFound, rec.Code)
}

// TestRegister_BothTenantRouteTreesCoexistOnOneEngine asserts only that the
// merchant admin tree and the platformadmin tree can both be registered on
// ONE gin engine at their CURRENT, production prefixes
// (/api/v1/admin/tenants/:tenantId/... and /api/v1/platform/admin/tenants/
// :id/...) without panicking.
//
// It does NOT and CANNOT detect a wildcard-name collision (Trap 2): the two
// trees mount at SIBLING prefixes, so they never share a route node, and no
// choice of wildcard name on either side can make this test panic. This was
// previously named TestRegister_RouterBuildsWithBothTenantRouteSets with a
// comment claiming it "pins Trap 2" — a reviewer mutated platformadmin's
// wildcard from :id to :tid and this test kept passing, proving the claim
// false. See TestRegister_SharingTheMerchantPrefixPanicsAtRouterBuild below
// for the test that actually exercises the collision, by putting both trees
// on the SAME prefix.
func TestRegister_BothTenantRouteTreesCoexistOnOneEngine(t *testing.T) {
	gin.SetMode(gin.TestMode)

	require.NotPanics(t, func() {
		r := gin.New()

		// Mirrors cmd/marketplace-api/main.go exactly: the merchant admin
		// tree mounts at /api/v1, and platformadmin mounts at the SIBLING
		// /api/v1/platform prefix — never under /api/v1/admin. The prefix
		// separation, not any property this test checks, is what prevents
		// the wildcard collision in production; see routes.go's Register
		// doc comment.
		admin.RegisterAdmin(r.Group("/api/v1"), admin.Deps{
			SSOConfigHandler: admin.NewSSOConfigHandler(nil, nil, nil, nil),
			AuthzMiddleware:  authz.NewMiddleware(authz.NewFakeClient(), nil),
		})

		platformadmin.Register(r.Group("/api/v1/platform"), fullPurgeDeps())
	})
}

// TestRegister_SharingTheMerchantPrefixPanicsAtRouterBuild pins the actual
// Trap 2 hazard: the merchant tree registers /admin/tenants/:tenantId/...
// and this surface uses :id at the same path position. Two DIFFERENT
// wildcard names at one path position make gin panic at ROUTER BUILD TIME
// — the service fails to start, and no request-level test catches it.
//
// This test pins WHY the two surfaces mount at sibling prefixes in
// production: put them on the SAME prefix and the router refuses to
// build. routes.go's Register doc comment says not to "tidy" the prefixes
// back together onto /api/v1/admin; this is that warning made executable.
//
// A bare stub route stands in for the merchant admin tree's
// /admin/tenants/:tenantId registration — the collision is a property of
// gin's router, not of any particular handler behind the route, so a stub
// proves it exactly as well as the real admin.RegisterAdmin tree and keeps
// the test cheap.
func TestRegister_SharingTheMerchantPrefixPanicsAtRouterBuild(t *testing.T) {
	gin.SetMode(gin.TestMode)

	require.Panics(t, func() {
		r := gin.New()
		g := r.Group("/api/v1")

		// Stands in for the merchant admin tree's
		// /admin/tenants/:tenantId/... group (internal/handlers/admin/
		// routes.go's ssoTenant), claiming the :tenantId wildcard at this
		// path position.
		g.GET("/admin/tenants/:tenantId/sso/config", func(c *gin.Context) {})

		// platformadmin's purge routes use :id at the SAME position
		// (/admin/tenants/:id/...) — mounted on the SAME "/api/v1" group
		// this time, not the sibling "/api/v1/platform" prefix production
		// actually uses. Two different wildcard names at this one node is
		// exactly what gin refuses to build.
		platformadmin.Register(g, fullPurgeDeps())
	})
}
