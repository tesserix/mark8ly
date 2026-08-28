package platformadmin_test

import (
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

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
func allWriteRoutesDeps(t *testing.T) platformadmin.Deps {
	return platformadmin.Deps{
		Repo:            &stubRepo{},
		Secret:          testSecret,
		DB:              &gorm.DB{},
		NonceStore:      newMemNonces(),
		TenantDirectory: &stubDirectory{detail: &tenantdirectory.TenantDetail{}},
		Emitter:         mustEmitter(t, &recordingRepo{}),
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
	platformadmin.Register(r.Group("/api/v1/platform"), allWriteRoutesDeps(t))

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

// allReadRoutesDeps wires EVERY dependency Register (routes.go) needs to
// mount every read route this surface has today, break-glass (#333)
// included, alongside every write route (allWriteRoutesDeps' set, embedded
// here too). Wiring only some read dependencies would leave some read
// routes unmounted, and both assertions in
// TestBreakGlassIsTheOnlyDeclaredReadCapability below would pass VACUOUSLY
// on any route it never saw — the same hazard allWriteRoutesDeps' doc
// comment describes for the write side. That is why this helper wires
// everything, and why the test separately requires the break-glass route
// was genuinely found rather than trusting an empty failure list alone.
func allReadRoutesDeps(t *testing.T) platformadmin.Deps {
	deps := allWriteRoutesDeps(t)
	deps.OnboardingFunnel = &stubFunnelClient{}
	deps.EstateCounts = &stubEstateCounts{}
	deps.Subscriptions = &stubSubscriptions{}
	deps.Trials = &stubTrialLister{}
	deps.AllSubscriptions = &stubSubscriptionLister{}
	deps.Tickets = &stubTicketLister{}
	deps.Notifications = &stubNotificationLister{}
	deps.Outbox = &stubOutboxLister{}
	deps.EmailSends = &stubSendLister{}
	deps.EstateUsers = &stubUserDirectory{}
	deps.Inbox = &routeInboxAggregator{}
	deps.InboxItems = stubItemSource{}
	deps.BreakGlass = &stubBreakGlassLister{}
	return deps
}

// TestBreakGlassIsTheOnlyDeclaredReadCapability is the read-side counterpart
// to TestAllWriteRoutesDeclareACapability, and closes the gap the previous
// task's reviewer flagged: RequiredReadCapabilities is keyed on the LITERAL
// STRING "GET /api/v1/platform/admin/break-glass". If the mount prefix or
// the route path ever changed, that key would silently stop matching, the
// read-capability gate would stop applying, and break-glass would become an
// ungated read with NO TEST FAILING — the write side has
// TestAllWriteRoutesDeclareACapability as a loud, by-name failure for the
// equivalent drift; the read side had nothing.
//
// It builds the REAL router via platformadmin.Register with every read
// (and write) dependency wired (allReadRoutesDeps), enumerates gin's OWN
// route table — never a hand-written route string — and asserts two
// things:
//
//  1. GET /api/v1/platform/admin/break-glass is actually mounted, and its
//     CapabilityKey (built with the exported platformadmin.CapabilityKey,
//     not a hand-written string) is present in
//     platformadmin.RequiredReadCapabilities. Asserting the route was
//     FOUND — not merely that a not-found route is absent from the map —
//     is what keeps this from passing vacuously if the mount itself
//     regresses.
//  2. Every OTHER GET route this surface mounts has NO entry in
//     RequiredReadCapabilities. A future read must not pick up an
//     unintended gate by accident — this is what would catch it. The four
//     reads already live in production today (/admin/audit-logs,
//     /admin/billing/subscriptions, /admin/billing/trials, /admin/health)
//     are asserted absent by name, since a stray entry on any of them
//     would 403 real operator traffic.
func TestBreakGlassIsTheOnlyDeclaredReadCapability(t *testing.T) {
	gin.SetMode(gin.TestMode)

	r := gin.New()
	platformadmin.Register(r.Group(platformadmin.MountPrefix), allReadRoutesDeps(t))

	const breakGlassPath = platformadmin.MountPrefix + "/admin/break-glass"

	breakGlassFound := false
	otherReadRouteCount := 0
	for _, route := range r.Routes() {
		if route.Method != http.MethodGet {
			continue
		}

		key := platformadmin.CapabilityKey(route.Method, route.Path)

		if route.Path == breakGlassPath {
			breakGlassFound = true
			required, declared := platformadmin.RequiredReadCapabilities[key]
			require.True(t, declared,
				"break-glass route %s %s has no entry in RequiredReadCapabilities — "+
					"an undeclared read is ungated by design (see the map's doc "+
					"comment), so this would silently reopen the emergency-account "+
					"inventory to any valid signature",
				route.Method, route.Path)
			require.Equal(t, "rotate-credentials", required,
				"break-glass must require exactly the capability value the "+
					"console's break-glass module sends")
			continue
		}

		otherReadRouteCount++
		_, declared := platformadmin.RequiredReadCapabilities[key]
		require.Falsef(t, declared,
			"read route %s %s unexpectedly has an entry in RequiredReadCapabilities — "+
				"only break-glass (#333) should be gated on this surface today; a "+
				"stray entry here would 403 real operator traffic on this route",
			route.Method, route.Path)
	}

	require.True(t, breakGlassFound,
		"GET %s was never found in the route table — either it failed to "+
			"mount (allReadRoutesDeps is missing a dependency) or its route "+
			"template changed; without this the assertions above would pass "+
			"vacuously",
		breakGlassPath)

	// Pins that this test actually exercised other mounted read routes too
	// — including the four already live in production
	// (/admin/audit-logs, /admin/billing/subscriptions,
	// /admin/billing/trials, /admin/health) — rather than the "every OTHER
	// route is absent" loop above passing vacuously because nothing else
	// mounted.
	require.Greater(t, otherReadRouteCount, 0,
		"expected other read routes besides break-glass to be mounted; got "+
			"none — allReadRoutesDeps is likely missing a dependency, which "+
			"would make the absence assertions above vacuous")

	for _, knownProductionRead := range []string{
		"/api/v1/platform/admin/audit-logs",
		"/api/v1/platform/admin/billing/subscriptions",
		"/api/v1/platform/admin/billing/trials",
		"/api/v1/platform/admin/health",
	} {
		key := platformadmin.CapabilityKey(http.MethodGet, knownProductionRead)
		_, declared := platformadmin.RequiredReadCapabilities[key]
		require.Falsef(t, declared,
			"production read route GET %s must have no entry in "+
				"RequiredReadCapabilities — an entry here would 403 real "+
				"operator traffic that has never needed a capability before",
			knownProductionRead)
	}
}
