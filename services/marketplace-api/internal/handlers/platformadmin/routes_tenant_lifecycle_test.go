package platformadmin_test

import (
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/audit"
	"github.com/mark8ly/marketplace-api/internal/handlers/platformadmin"
)

// hasRoute reports whether r has a registered route for method+path.
// Route registration happens at Register() time regardless of whether
// RequirePlatformAuth would later accept or reject a real request, so this
// is checked without ever sending one — mirroring TestRegisterMountsHealth
// in routes_test.go.
func hasRoute(r *gin.Engine, method, path string) bool {
	for _, route := range r.Routes() {
		if route.Method == method && route.Path == path {
			return true
		}
	}
	return false
}

// TestRegisterRequiresEmitterToMountTenantLifecycle pins F1 (#287): a
// TenantLifecycle client with no Emitter must NOT get the two write routes
// mounted. Register's nil-safe pattern for every OTHER client-backed route
// is "mount in a degraded state" (see e.g. TenantDirectory alone unlocking
// /admin/entities/tenants); tenant suspend/unsuspend is different — a
// write endpoint that cannot be attributed to an operator must not exist
// on this surface at all, so Emitter == nil is a hard "do not mount", not
// a degraded mount.
func TestRegisterRequiresEmitterToMountTenantLifecycle(t *testing.T) {
	gin.SetMode(gin.TestMode)

	r := gin.New()
	platformadmin.Register(r.Group("/api/v1/platform"), platformadmin.Deps{
		Repo:            &stubRepo{},
		Secret:          "test-secret",
		DB:              &gorm.DB{},
		TenantLifecycle: &stubLifecycle{},
		Emitter:         nil, // the point of this test
	})

	require.False(t, hasRoute(r, http.MethodPost, "/api/v1/platform/admin/tenants/:id/suspend"),
		"suspend must not mount without an Emitter")
	require.False(t, hasRoute(r, http.MethodPost, "/api/v1/platform/admin/tenants/:id/unsuspend"),
		"unsuspend must not mount without an Emitter")
}

// TestRegisterMountsTenantLifecycleWhenFullyWired is the positive
// counterpart: TenantLifecycle + DB + Emitter all present mounts both
// routes.
func TestRegisterMountsTenantLifecycleWhenFullyWired(t *testing.T) {
	gin.SetMode(gin.TestMode)

	r := gin.New()
	platformadmin.Register(r.Group("/api/v1/platform"), platformadmin.Deps{
		Repo:            &stubRepo{},
		Secret:          "test-secret",
		DB:              &gorm.DB{},
		TenantLifecycle: &stubLifecycle{},
		Emitter:         audit.NewEmitter(audit.EmitterConfig{Repo: &recordingRepo{}}),
	})

	require.True(t, hasRoute(r, http.MethodPost, "/api/v1/platform/admin/tenants/:id/suspend"),
		"suspend must mount when TenantLifecycle, DB and Emitter are all wired")
	require.True(t, hasRoute(r, http.MethodPost, "/api/v1/platform/admin/tenants/:id/unsuspend"),
		"unsuspend must mount when TenantLifecycle, DB and Emitter are all wired")
}
