package platformadmin_test

import (
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/handlers/platformadmin"
)

// TestRegisterMountsTrialExtendWhenFullyWired is the positive counterpart:
// TrialExtender + DB + Emitter all present mounts the route. Uses hasRoute
// (routes_tenant_lifecycle_test.go) rather than issuing a request — route
// registration happens at Register() time regardless of what a real request
// would do, so a bogus-sibling-path check adds nothing here and is dropped;
// hasRoute inspects the router's route table directly, which is a stronger
// assertion than "not 404" and can't be satisfied by a catch-all handler.
func TestRegisterMountsTrialExtendWhenFullyWired(t *testing.T) {
	gin.SetMode(gin.TestMode)

	r := gin.New()
	platformadmin.Register(r.Group("/api/v1/platform"), platformadmin.Deps{
		Repo:          &stubRepo{},
		Secret:        "test-secret",
		DB:            &gorm.DB{},
		TrialExtender: &stubExtender{},
		Emitter:       mustEmitter(t, &recordingRepo{}),
	})

	require.True(t, hasRoute(r, http.MethodPost, "/api/v1/platform/admin/billing/trials/:storeID/extend"),
		"the route must mount when TrialExtender, DB and Emitter are all wired")
}

// Nil leaves it unmounted, matching every other optional route here.
func TestRegisterLeavesTrialExtendUnmountedWhenNil(t *testing.T) {
	gin.SetMode(gin.TestMode)

	r := gin.New()
	platformadmin.Register(r.Group("/api/v1/platform"), platformadmin.Deps{Repo: &stubRepo{}})

	require.False(t, hasRoute(r, http.MethodPost, "/api/v1/platform/admin/billing/trials/:storeID/extend"),
		"the route must not mount when TrialExtender is nil")
}

// TestRegisterRequiresEmitterToMountTrialExtend pins F1 (#287): a
// TrialExtender with no Emitter must NOT get the route mounted. A trial
// extension is a billing decision made against a merchant; an unattributed
// one should not be reachable, even though the handler itself is nil-safe
// on a nil Emitter (NewOperatorActionAuditFunc tolerates it) — the point is
// that the route must not be reachable at all, not merely that it would
// behave safely if reached. Modeled on
// TestRegisterRequiresEmitterToMountTenantLifecycle.
func TestRegisterRequiresEmitterToMountTrialExtend(t *testing.T) {
	gin.SetMode(gin.TestMode)

	r := gin.New()
	platformadmin.Register(r.Group("/api/v1/platform"), platformadmin.Deps{
		Repo:          &stubRepo{},
		Secret:        "test-secret",
		DB:            &gorm.DB{},
		TrialExtender: &stubExtender{},
		Emitter:       nil, // the point of this test
	})

	require.False(t, hasRoute(r, http.MethodPost, "/api/v1/platform/admin/billing/trials/:storeID/extend"),
		"the route must not mount without an Emitter")
}
