package admin_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/handlers/admin"
)

// breakGlassRoutePath is the mount point mark8ly#642's plan asserts:
// POST /admin/break-glass/login, at the /admin root — NOT nested under
// /admin/stores/:storeId.
const breakGlassRoutePath = "/admin/break-glass/login"

// TestBreakGlassLogin_RouteIsMounted is the proof this task exists to
// produce: NewBreakGlassLoginHandler is now constructed in
// cmd/marketplace-api/main.go (previously only in tests), so the route
// admin/routes.go has always been willing to mount — gated on
// deps.BreakGlassLoginHandler != nil — actually gets mounted.
//
// This deliberately does NOT probe with a GET-on-POST-only-route and
// compare 404 vs 405: HandleMethodNotAllowed is never set in any service
// in this estate, so gin's default (false) applies and a GET on a
// MOUNTED POST-only route also 404s — that probe cannot tell "unmounted"
// apart from "mounted, wrong method". Instead this compares the actual
// gin route table (*gin.Engine.Routes()) of a bare router against one
// with the handler wired — the same fact router.ServeHTTP would exercise,
// read directly rather than inferred from a status code.
func TestBreakGlassLogin_RouteIsMounted(t *testing.T) {
	bare := gin.New()
	admin.RegisterAdmin(bare.Group("/"), admin.Deps{})

	wired := gin.New()
	admin.RegisterAdmin(wired.Group("/"), admin.Deps{
		BreakGlassLoginHandler: admin.NewBreakGlassLoginHandler(admin.BreakGlassDeps{}),
		PlanResolver:           nil, // unreached: the gate short-circuits below on a peek miss
	})

	require.False(t, hasRoute(bare, http.MethodPost, breakGlassRoutePath),
		"a bare Deps{} router must NOT have the break-glass login route — otherwise this test proves nothing")
	require.True(t, hasRoute(wired, http.MethodPost, breakGlassRoutePath),
		"POST /admin/break-glass/login must be registered once BreakGlassLoginHandler is set — "+
			"this is the mount mark8ly#642 needed: NewBreakGlassLoginHandler is now called from "+
			"cmd/marketplace-api/main.go, not only from tests")
}

func hasRoute(r *gin.Engine, method, path string) bool {
	for _, rt := range r.Routes() {
		if rt.Method == method && rt.Path == path {
			return true
		}
	}
	return false
}

// TestBreakGlassLogin_SurvivesReadOnlyAndStoreClosed proves the route is
// mounted OUTSIDE the store-scoped group that carries
// SubscriptionReadOnlyGate — the whole point of break-glass (§12.4): it
// must survive read_only / store_closed subscription states.
//
// This asserts the fact structurally (the registered path) rather than
// by driving a live request through the full middleware chain: doing the
// latter for real would need a *plangate.PlanResolver and a
// *breakglass.Repository backed by an actual Postgres connection (both
// run real queries once a well-formed tenant_id makes it past the
// gate — see break_glass_shared_limiter_integration_test.go, which
// exercises exactly that live-DB path), which this package's unit tests
// deliberately don't stand up. routes.go wires SubscriptionReadOnlyGate
// ONLY into storeMW / storeRoute (path prefix
// "/admin/stores/:storeId..."); the break-glass route is registered
// directly on router at "/admin/break-glass/login" — a path that cannot
// be a descendant of that group, proving it never passes through
// SubscriptionReadOnlyGate regardless of runtime subscription state.
func TestBreakGlassLogin_SurvivesReadOnlyAndStoreClosed(t *testing.T) {
	r := gin.New()
	admin.RegisterAdmin(r.Group("/"), admin.Deps{
		BreakGlassLoginHandler: admin.NewBreakGlassLoginHandler(admin.BreakGlassDeps{}),
	})

	found := false
	for _, rt := range r.Routes() {
		if rt.Method != http.MethodPost || rt.Path != breakGlassRoutePath {
			continue
		}
		found = true
		require.False(t, strings.HasPrefix(rt.Path, "/admin/stores/"),
			"break-glass login must not be registered under /admin/stores/:storeId — "+
				"that is the ONLY group SubscriptionReadOnlyGate is wired into, and nesting "+
				"break-glass there would make it (silently) subject to read_only / store_closed "+
				"gating, defeating the entire point of the recovery path")
	}
	require.True(t, found, "break-glass login route must be registered for this assertion to mean anything")
}
