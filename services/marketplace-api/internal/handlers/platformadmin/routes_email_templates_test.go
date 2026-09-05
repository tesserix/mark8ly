package platformadmin_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/handlers/platformadmin"
)

func emailTemplateRoutes(t *testing.T, deps platformadmin.Deps) map[string]bool {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	platformadmin.Register(r.Group(platformadmin.MountPrefix), deps)

	mounted := map[string]bool{}
	for _, route := range r.Routes() {
		mounted[route.Method+" "+route.Path] = true
	}
	return mounted
}

func baseTemplateDeps() platformadmin.Deps {
	return platformadmin.Deps{
		Repo:                  &stubRepo{},
		Secret:                testSecret,
		EmailTemplates:        newStubTemplateStore(),
		EmailTemplateRegistry: newStubRegistry(nil),
	}
}

func TestRegisterMountsEmailTemplatesWhenStoreAndRegistryPresent(t *testing.T) {
	deps := baseTemplateDeps()
	deps.DB = &gorm.DB{}
	mounted := emailTemplateRoutes(t, deps)

	for _, want := range []string{
		"GET " + platformadmin.MountPrefix + "/admin/email-templates",
		"GET " + platformadmin.MountPrefix + "/admin/email-templates/:key",
		"PUT " + platformadmin.MountPrefix + "/admin/email-templates/:key",
		"POST " + platformadmin.MountPrefix + "/admin/email-templates/:key/test-send",
	} {
		require.True(t, mounted[want], "%s must be mounted", want)
	}
}

// A nil store leaves everything unmounted, matching the nil-safe pattern
// every other client-backed route uses.
func TestRegisterLeavesEmailTemplatesUnmountedWithoutStore(t *testing.T) {
	deps := baseTemplateDeps()
	deps.EmailTemplates = nil
	mounted := emailTemplateRoutes(t, deps)
	require.False(t, mounted["GET "+platformadmin.MountPrefix+"/admin/email-templates"])
}

// A nil REGISTRY also leaves everything unmounted, and that is not the
// usual "needs its collaborators" pairing. The registry is the only view
// of the registered-but-unseeded keys (mark8ly#717) — the twelve billing
// keys are registered and deliberately never seeded — so a list built from
// database rows alone silently omits them while looking complete.
// Mounting the read without it would ship exactly the bug the endpoint
// exists to fix.
func TestRegisterLeavesEmailTemplatesUnmountedWithoutRegistry(t *testing.T) {
	deps := baseTemplateDeps()
	deps.EmailTemplateRegistry = nil
	mounted := emailTemplateRoutes(t, deps)
	require.False(t, mounted["GET "+platformadmin.MountPrefix+"/admin/email-templates"],
		"a list that cannot see the registered keys is mark8ly#717, not a degraded read")
}

// Without a DB the change cannot be recorded against an operator, so the
// PUT must not mount — the same rule the other writes on this surface
// apply with Emitter (#287, F1). The reads keep working: losing sight of
// what is sending because the change could not be recorded would be a
// worse trade than the one the guard exists to make.
func TestRegisterLeavesTheEmailTemplateWriteUnmountedWithoutDB(t *testing.T) {
	mounted := emailTemplateRoutes(t, baseTemplateDeps())
	require.False(t, mounted["PUT "+platformadmin.MountPrefix+"/admin/email-templates/:key"],
		"an unattributable template edit must not be reachable")
	require.True(t, mounted["GET "+platformadmin.MountPrefix+"/admin/email-templates"])
}

// The routes must sit under /api/v1/platform and never /api/v1/admin: an
// Istio AuthorizationPolicy denies un-JWT'd requests to that prefix and
// this surface authenticates by HMAC, so the mesh would answer 403 before
// the app saw the request — invisible in local dev and CI.
func TestEmailTemplateRoutesAreNotUnderTheAdminPrefix(t *testing.T) {
	deps := baseTemplateDeps()
	deps.DB = &gorm.DB{}
	gin.SetMode(gin.TestMode)
	r := gin.New()
	platformadmin.Register(r.Group(platformadmin.MountPrefix), deps)

	for _, route := range r.Routes() {
		require.NotContains(t, route.Path, "/api/v1/admin/",
			"route %s %s must not live under the mesh-gated admin prefix", route.Method, route.Path)
	}
}

// Every route on this surface sits behind RequirePlatformAuth, so an
// unconfigured secret leaves them inert rather than open.
func TestEmailTemplateRoutesFailClosedWithoutASecret(t *testing.T) {
	deps := baseTemplateDeps()
	deps.DB = &gorm.DB{}
	deps.Secret = ""
	gin.SetMode(gin.TestMode)
	r := gin.New()
	platformadmin.Register(r.Group(platformadmin.MountPrefix), deps)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet,
		platformadmin.MountPrefix+"/admin/email-templates", nil))
	require.Equal(t, http.StatusServiceUnavailable, rec.Code)
	require.Equal(t, "not_configured", errorCode(t, rec))
}
