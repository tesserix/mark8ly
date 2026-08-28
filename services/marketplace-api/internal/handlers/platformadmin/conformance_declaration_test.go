package platformadmin_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/handlers/platformadmin"
)

// contractEndpointIDs mirrors design-system/packages/admin-conformance/src/
// contract.ts:13-126's ENDPOINT_IDS — the closed, nine-entry vocabulary of
// ids a product's admin-conformance.json may declare. That file is the
// source of truth; this slice is a cross-repo, cross-language MIRROR of it,
// checked out in a sibling repo this package cannot import. Whoever adds a
// tenth endpoint to contract.ts must update this slice in the same change,
// or this guard silently stops covering the new id — there is no compiler
// to enforce that link across repos and languages, only this comment.
var contractEndpointIDs = []string{
	"kpis",
	"inbox",
	"audit-logs",
	"entities",
	"health",
	"billing/subscriptions",
	"billing/trials",
	"tenant-lifecycle",
	"lifecycle/reason-codes",
}

// routeToContractID maps every route TEMPLATE (as gin's own route table
// reports it, MountPrefix included) that implements one of the nine
// contract ids above to that id. Built by hand against
// design-system/packages/admin-conformance/src/contract.ts's per-endpoint
// `path` (and, for `entities`, the subtypes this product actually declares
// in admin-conformance.json: tenants and users) and against
// routes.go/each handler's own g.GET/g.POST call — not against a REST
// convention guess.
//
// Routes this surface mounts that have NO entry here (outbox, email-sends,
// notifications, tickets, break-glass, conversions, onboarding/funnel,
// onboarding/sessions, the inbox actions write, tenant purge) are exactly
// the mark8ly-specific reads and writes docs/admin-conformance.md documents
// as structurally undeclarable — this test deliberately has nothing to say
// about them, because the nine-id vocabulary has nowhere to put them.
var routeToContractID = map[string]string{
	platformadmin.MountPrefix + "/admin/kpis":                   "kpis",
	platformadmin.MountPrefix + "/admin/inbox":                  "inbox",
	platformadmin.MountPrefix + "/admin/audit-logs":             "audit-logs",
	platformadmin.MountPrefix + "/admin/entities/tenants":       "entities",
	platformadmin.MountPrefix + "/admin/entities/tenants/:id":   "entities",
	platformadmin.MountPrefix + "/admin/entities/users":         "entities",
	platformadmin.MountPrefix + "/admin/health":                 "health",
	platformadmin.MountPrefix + "/admin/billing/subscriptions":  "billing/subscriptions",
	platformadmin.MountPrefix + "/admin/billing/trials":         "billing/trials",
	platformadmin.MountPrefix + "/admin/tenants/:id/suspend":    "tenant-lifecycle",
	platformadmin.MountPrefix + "/admin/tenants/:id/unsuspend":  "tenant-lifecycle",
	platformadmin.MountPrefix + "/admin/lifecycle/reason-codes": "lifecycle/reason-codes",
}

// conformanceDeclarationPath resolves admin-conformance.json relative to
// THIS TEST FILE, not the working directory `go test` happens to run from —
// runtime.Caller(0) gives this file's own path regardless of caller cwd.
// This package lives at services/marketplace-api/internal/handlers/
// platformadmin, five directories below the repo root, so five ".." gets
// there.
func conformanceDeclarationPath(t *testing.T) string {
	t.Helper()

	_, thisFile, _, ok := runtime.Caller(0)
	require.True(t, ok, "runtime.Caller(0) failed to report this test file's own path")

	root := filepath.Join(filepath.Dir(thisFile), "..", "..", "..", "..", "..")
	path := filepath.Join(root, "admin-conformance.json")

	// Fail loudly, not skip: a guard whose path silently stops resolving
	// (e.g. this package moves) must not quietly stop running — that is
	// the exact failure mode #415 exists to prevent, one level up.
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("admin-conformance.json not found at resolved path %q (resolved from test file %q): %v — "+
			"this guard's path resolution has drifted from the repo layout and needs fixing, "+
			"not skipping", path, thisFile, err)
	}

	return path
}

// readDeclaredEndpointIDs reads admin-conformance.json and returns the set
// of endpoint ids it declares, after asserting every key is one of the nine
// contract ids. This mirrors design-system/packages/admin-conformance/src/
// declaration.ts's parseEndpoints, which THROWS on an unknown key and fails
// the entire nightly conformance run rather than just that entry. Failing
// here on the same condition means a typo lands as a red build in mark8ly's
// own CI, immediately, instead of surfacing overnight against production.
func readDeclaredEndpointIDs(t *testing.T, path string) map[string]bool {
	t.Helper()

	raw, err := os.ReadFile(path)
	require.NoError(t, err, "failed to read %s", path)

	var doc struct {
		Endpoints map[string]json.RawMessage `json:"endpoints"`
	}
	require.NoError(t, json.Unmarshal(raw, &doc), "%s is not valid JSON", path)

	validIDs := make(map[string]bool, len(contractEndpointIDs))
	for _, id := range contractEndpointIDs {
		validIDs[id] = true
	}

	declared := make(map[string]bool, len(doc.Endpoints))
	for key := range doc.Endpoints {
		require.Truef(t, validIDs[key],
			"admin-conformance.json declares unknown endpoint id %q; valid ids are: %v — "+
				"design-system's declaration.ts:parseEndpoints throws on exactly this, "+
				"which would fail the ENTIRE nightly conformance run, not just this entry",
			key, contractEndpointIDs)
		declared[key] = true
	}

	return declared
}

// declarationRouterDeps wires every dependency platformadmin.Register needs
// to mount every route this surface has today — reusing the same
// allReadRoutesDeps helper routes_capability_coverage_test.go defines for
// exactly this reason (its own doc comment explains the vacuous-pass trap:
// wiring only some dependencies mounts only some routes, and any assertion
// over "what's mounted" would silently pass on the routes it never saw).
func declarationRouterDeps() platformadmin.Deps {
	return allReadRoutesDeps()
}

// TestConformanceDeclarationMatchesMountedRoutes is #415's guard: it proves
// admin-conformance.json and the REAL, currently-mounted router agree on
// which of the nine contract endpoints this surface exposes.
//
// It verifies DECLARED vs MOUNTED only, in both directions, and
// deliberately says nothing about WIRED IN THE DEPLOYED ENVIRONMENT — that
// third state (whether platform-api's federation registry and the
// deployed Deps wiring actually reach this endpoint) is the nightly
// CronJob's and platform-api's job, not this test's. See
// docs/admin-conformance.md, "Implemented != declared != wired".
//
//   - A contract id whose route IS mounted but is NOT declared fails,
//     naming the id and the route. This is #415's actual bug: `inbox`
//     shipped and mounted, but nobody added it to admin-conformance.json,
//     so the nightly suite never checked it and the console's queue stayed
//     invisible with nothing erroring anywhere.
//   - A contract id that IS declared but whose route is NOT mounted fails,
//     naming the id. This is the over-declaring direction: declaring an
//     endpoint mark8ly does not actually serve turns the nightly
//     conformance job permanently red against production — worse than
//     under-declaring, since operators are meant to trust this surface.
func TestConformanceDeclarationMatchesMountedRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)

	declaredIDs := readDeclaredEndpointIDs(t, conformanceDeclarationPath(t))

	r := gin.New()
	platformadmin.Register(r.Group(platformadmin.MountPrefix), declarationRouterDeps())

	routes := r.Routes()
	require.NotEmpty(t, routes,
		"platformadmin.Register mounted zero routes — declarationRouterDeps is "+
			"missing a dependency; every assertion below would pass vacuously "+
			"against an empty route table")

	mountedIDs := make(map[string][]string) // contract id -> mounted route templates that matched it
	for _, route := range routes {
		id, known := routeToContractID[route.Path]
		if !known {
			// Not part of the nine-id contract vocabulary at all — one of
			// the mark8ly-specific reads/writes docs/admin-conformance.md
			// documents as structurally undeclarable. Nothing to assert.
			continue
		}
		mountedIDs[id] = append(mountedIDs[id], route.Method+" "+route.Path)
	}

	require.NotEmpty(t, mountedIDs,
		"none of the mounted routes matched any of the nine contract ids in "+
			"routeToContractID — either the route table changed shape or "+
			"routeToContractID has drifted from routes.go; without at least "+
			"one match this test cannot exercise either direction")

	for _, id := range contractEndpointIDs {
		mountedRoutes, isMounted := mountedIDs[id]
		isDeclared := declaredIDs[id]

		require.Falsef(t, isMounted && !isDeclared,
			"contract endpoint %q is MOUNTED (%v) but NOT DECLARED in admin-conformance.json — "+
				"this is #415's bug shape: the route shipped and stayed invisible to the "+
				"conformance suite with nothing erroring anywhere. Add %q to "+
				"admin-conformance.json's endpoints map.",
			id, mountedRoutes, id)

		require.Falsef(t, isDeclared && !isMounted,
			"contract endpoint %q is DECLARED in admin-conformance.json but NOT MOUNTED by "+
				"platformadmin.Register — this is the over-declaring direction: the nightly "+
				"conformance job will run permanently red against production for an endpoint "+
				"mark8ly does not actually serve. Remove %q from admin-conformance.json, or "+
				"wire the dependency that mounts it.",
			id, id)
	}
}
