package platformadmin_test

import (
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/audit"
	"github.com/mark8ly/marketplace-api/internal/handlers/platformadmin"
	"github.com/mark8ly/marketplace-api/internal/tenantdirectory"
)

// allWriteRoutesDeps wires every dependency Register (routes.go) needs to
// mount ALL FOUR of this surface's write routes at once: POST
// /admin/billing/trials/:storeID/extend, POST /admin/tenants/:id/suspend,
// POST /admin/tenants/:id/unsuspend, and POST /admin/tenants/:id/purge.
//
// Register's switch statements (routes.go) mount each write route group
// only when THAT group's own dependency set is fully non-nil —
// TrialExtender needs DB+Emitter, TenantLifecycle needs DB+Emitter, and the
// purge pair needs DB+Emitter+TenantDirectory+TenantTeardown+Purger.
// Wiring only some of these would mount only some write routes, and
// TestAllWriteRoutesDeclareACapability below would pass VACUOUSLY on the
// ones it never saw. That is why this helper wires every dependency, and
// why the test separately asserts the mounted write-route count is exactly
// 4 rather than trusting an empty failure list alone.
func allWriteRoutesDeps() platformadmin.Deps {
	return platformadmin.Deps{
		Repo:            &stubRepo{},
		Secret:          testSecret,
		DB:              &gorm.DB{},
		NonceStore:      newMemNonces(),
		TenantDirectory: &stubDirectory{detail: &tenantdirectory.TenantDetail{}},
		Emitter:         audit.NewEmitter(audit.EmitterConfig{Repo: &recordingRepo{}}),
		TenantLifecycle: &stubLifecycle{},
		TrialExtender:   &stubExtender{},
		TenantTeardown:  &fakeTeardown{seq: &seq{}},
		Purger:          &fakePurger{seq: &seq{}},
	}
}

// isWriteMethod mirrors isWrite's classification in middleware.go (POST,
// PUT, PATCH, DELETE). Deliberately duplicated here rather than exported
// from the production package: this test independently derives "which
// routes are writes" from the SAME rule the gate itself uses, so if the
// two definitions ever drift apart from each other, that drift is exactly
// what should make this test fail — not something to hide by sharing one
// implementation.
func isWriteMethod(method string) bool {
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	default:
		return false
	}
}

// TestAllWriteRoutesDeclareACapability is #364's real deliverable. #364's
// own premise: there is ZERO operator traffic on this surface to shake out
// a missing capability declaration, so without this test the first symptom
// of one is a 403 in production the day #333 turns enforcement on.
//
// It builds the REAL router via platformadmin.Register with every
// write-route dependency wired (allWriteRoutesDeps), enumerates gin's OWN
// route table (not a hand-maintained list), and fails — naming the
// offending route — if any write-method route has no entry in
// RequiredWriteCapabilities.
//
// This test is proven to catch both failure modes it claims to catch (see
// the 364-report.md mutation log):
//   - a write route added without a capability declaration, and
//   - a declared route renamed (c.FullPath() / route.Path changes, so the
//     old lookup key silently stops matching).
func TestAllWriteRoutesDeclareACapability(t *testing.T) {
	gin.SetMode(gin.TestMode)

	r := gin.New()
	platformadmin.Register(r.Group("/api/v1/platform"), allWriteRoutesDeps())

	writeRouteCount := 0
	for _, route := range r.Routes() {
		if !isWriteMethod(route.Method) {
			continue
		}
		writeRouteCount++

		key := platformadmin.CapabilityKey(route.Method, route.Path)
		_, declared := platformadmin.RequiredWriteCapabilities[key]
		require.True(t, declared,
			"write route %s %s has no entry in RequiredWriteCapabilities — "+
				"a route added without declaring its capability fails OPEN "+
				"today and will 403 in production the day #333 turns "+
				"enforcement on",
			route.Method, route.Path)
	}

	// Pins the count so a route that silently fails to MOUNT (e.g. a stray
	// nil in allWriteRoutesDeps' wiring) is caught too — without this, the
	// loop above would just see fewer routes and pass on the ones it never
	// saw, exactly the vacuous-pass hazard this file's doc comments warn
	// about.
	require.Equal(t, 4, writeRouteCount,
		"expected exactly the 4 known write routes to be mounted; got %d — "+
			"either allWriteRoutesDeps is missing a dependency, or a new "+
			"write route was added and this test's expected count needs "+
			"updating alongside its capability declaration in "+
			"RequiredWriteCapabilities",
		writeRouteCount)
}
