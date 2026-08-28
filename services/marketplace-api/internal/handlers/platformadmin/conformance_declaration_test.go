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
// reports it, MountPrefix included) that implements one of the contract ids
// above to that id — EXCEPT `entities`, whose routes are checked separately
// at subtype granularity by checkEntitiesDeclaration below, because a
// single "is entities mounted" bit is too coarse (see that function's doc
// comment). Built by hand against design-system/packages/admin-conformance/
// src/contract.ts's per-endpoint `path` and against routes.go/each
// handler's own g.GET/g.POST call — not against a REST convention guess.
//
// tenant-lifecycle maps to BOTH of its routes even though it is a single
// contract id, because routes.go mounts them atomically: both suspend and
// unsuspend are registered by the same NewTenantLifecycleHandler(...).
// Register(group) call inside one switch case gated on
// TenantLifecycle/DB/Emitter all being non-nil (routes.go). There is no way
// for one to mount without the other, so — unlike entities — a single
// mounted/declared bit for the id is accurate here.
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
	platformadmin.MountPrefix + "/admin/health":                 "health",
	platformadmin.MountPrefix + "/admin/billing/subscriptions":  "billing/subscriptions",
	platformadmin.MountPrefix + "/admin/billing/trials":         "billing/trials",
	platformadmin.MountPrefix + "/admin/tenants/:id/suspend":    "tenant-lifecycle",
	platformadmin.MountPrefix + "/admin/tenants/:id/unsuspend":  "tenant-lifecycle",
	platformadmin.MountPrefix + "/admin/lifecycle/reason-codes": "lifecycle/reason-codes",
}

// entitySubtypeRoutes maps each `entities` subtype this product can declare
// under admin-conformance.json's `"entities": {"types": [...]}` to the
// route template(s) that serve it. Unlike every other contract id, entities
// is checked per subtype (see checkEntitiesDeclaration) because its two
// subtypes mount on INDEPENDENT conditions in routes.go: `tenants` is gated
// on `deps.TenantDirectory != nil` alone, `users` on `deps.EstateUsers !=
// nil` alone — two separate `if` blocks, not one. A regression that leaves
// only one of those two dependencies nil unmounts only that subtype's
// route while the other stays live, which a single "is entities mounted at
// all" check cannot see: it would still report "mounted" as long as either
// route survived, even while the JSON's declared `types` list claims both.
var entitySubtypeRoutes = map[string][]string{
	"tenants": {
		platformadmin.MountPrefix + "/admin/entities/tenants",
		platformadmin.MountPrefix + "/admin/entities/tenants/:id",
	},
	"users": {
		platformadmin.MountPrefix + "/admin/entities/users",
	},
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

// conformanceDoc is the shape of admin-conformance.json this test cares
// about. Endpoints is kept as raw messages: most ids are declared as bare
// booleans, but `entities` carries a `{"types": [...]}` object that needs
// its own parse (see readDeclaredEntityTypes).
type conformanceDoc struct {
	Endpoints map[string]json.RawMessage `json:"endpoints"`
}

func readConformanceDoc(t *testing.T, path string) conformanceDoc {
	t.Helper()

	raw, err := os.ReadFile(path)
	require.NoError(t, err, "failed to read %s", path)

	var doc conformanceDoc
	require.NoError(t, json.Unmarshal(raw, &doc), "%s is not valid JSON", path)
	return doc
}

// readDeclaredEndpointIDs returns the set of endpoint ids admin-conformance.json
// declares, after asserting every key is one of the nine contract ids. This
// mirrors design-system/packages/admin-conformance/src/declaration.ts's
// parseEndpoints, which THROWS on an unknown key and fails the entire
// nightly conformance run rather than just that entry. Failing here on the
// same condition means a typo lands as a red build in mark8ly's own CI,
// immediately, instead of surfacing overnight against production.
func readDeclaredEndpointIDs(t *testing.T, doc conformanceDoc) map[string]bool {
	t.Helper()

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

// readDeclaredEntityTypes parses the `types` array under the `entities` key,
// if declared. An entities declaration with no parseable `types` field is
// itself a conformance bug — the contract requires subtypes for this id
// (contract.ts's `requiresSubtypes: true`) — so this fails loudly rather
// than treating it as "no subtypes declared".
func readDeclaredEntityTypes(t *testing.T, doc conformanceDoc) map[string]bool {
	t.Helper()

	raw, present := doc.Endpoints["entities"]
	if !present {
		return map[string]bool{}
	}

	var parsed struct {
		Types []string `json:"types"`
	}
	require.NoErrorf(t, json.Unmarshal(raw, &parsed),
		"admin-conformance.json's \"entities\" declaration is not the expected "+
			"{\"types\": [...]} shape: %s", string(raw))

	types := make(map[string]bool, len(parsed.Types))
	for _, ty := range parsed.Types {
		types[ty] = true
	}
	return types
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

// mountedRouteSet returns, from gin's own route table, the set of route
// templates (method-agnostic — MountPrefix included) that are actually
// mounted. Reading gin's live route table rather than a hand-maintained
// list is what makes this test trustworthy: it fails the moment routes.go's
// mount conditions and this file's expectations disagree.
func mountedRouteSet(routes gin.RoutesInfo) map[string]bool {
	set := make(map[string]bool, len(routes))
	for _, route := range routes {
		set[route.Path] = true
	}
	return set
}

// checkEntitiesDeclaration is the `entities` counterpart to the generic
// per-id loop in TestConformanceDeclarationMatchesMountedRoutes, split out
// because entities must be checked at SUBTYPE granularity, not as one
// mounted/declared bit.
//
// Why: `/admin/entities/tenants` mounts on `deps.TenantDirectory != nil`
// alone, and `/admin/entities/users` mounts on `deps.EstateUsers != nil`
// alone — two independent `if` blocks in routes.go, not one gate covering
// both. If a regression left EstateUsers nil while TenantDirectory stayed
// wired, `/admin/entities/users` would silently stop mounting while
// `/admin/entities/tenants` kept working. A coarse "is entities mounted"
// check (true because tenants still mounts) would pass right through that
// regression even though admin-conformance.json's declared `types: ["users",
// ...]` would now be a lie — declared-but-not-served, the exact shape this
// whole test exists to catch, just missed at the wrong granularity.
//
// tenant-lifecycle does NOT have this problem: routes.go mounts its two
// routes (suspend, unsuspend) from the SAME switch case, gated on the same
// three-way non-nil check, so they can only ever mount or not mount
// together — a single bit is accurate there, which is why it stays in the
// generic per-id loop instead of getting its own subtype treatment.
func checkEntitiesDeclaration(t *testing.T, mounted map[string]bool, declaredTypes map[string]bool) {
	t.Helper()

	for subtype, routes := range entitySubtypeRoutes {
		isMounted := false
		var mountedTemplates []string
		for _, route := range routes {
			if mounted[route] {
				isMounted = true
				mountedTemplates = append(mountedTemplates, route)
			}
		}
		isDeclared := declaredTypes[subtype]

		require.Falsef(t, isMounted && !isDeclared,
			"entities subtype %q is MOUNTED (%v) but admin-conformance.json's "+
				"\"entities\".\"types\" does not declare it — add %q to that "+
				"types array",
			subtype, mountedTemplates, subtype)

		require.Falsef(t, isDeclared && !isMounted,
			"admin-conformance.json declares entities type %q but its route(s) "+
				"%v are not mounted — either this is a real regression (e.g. a "+
				"nil dependency in production wiring) or %q should be removed "+
				"from the declared types array",
			subtype, routes, subtype)
	}
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
//
// `entities` is checked separately, at subtype granularity, by
// checkEntitiesDeclaration — see its doc comment for why a single bit is
// not enough for that id.
func TestConformanceDeclarationMatchesMountedRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)

	doc := readConformanceDoc(t, conformanceDeclarationPath(t))
	declaredIDs := readDeclaredEndpointIDs(t, doc)
	declaredEntityTypes := readDeclaredEntityTypes(t, doc)

	r := gin.New()
	platformadmin.Register(r.Group(platformadmin.MountPrefix), declarationRouterDeps())

	routes := r.Routes()
	require.NotEmpty(t, routes,
		"platformadmin.Register mounted zero routes — declarationRouterDeps is "+
			"missing a dependency; every assertion below would pass vacuously "+
			"against an empty route table")

	mounted := mountedRouteSet(routes)

	mountedIDs := make(map[string][]string) // contract id -> mounted route templates that matched it
	for _, route := range routes {
		id, known := routeToContractID[route.Path]
		if !known {
			// Not part of the nine-id contract vocabulary at all (or is
			// `entities`, checked separately above) — one of the
			// mark8ly-specific reads/writes docs/admin-conformance.md
			// documents as structurally undeclarable. Nothing to assert.
			continue
		}
		mountedIDs[id] = append(mountedIDs[id], route.Method+" "+route.Path)
	}

	require.NotEmpty(t, mountedIDs,
		"none of the mounted routes matched any of the contract ids in "+
			"routeToContractID — either the route table changed shape or "+
			"routeToContractID has drifted from routes.go; without at least "+
			"one match this test cannot exercise either direction")

	for _, id := range contractEndpointIDs {
		if id == "entities" {
			continue // checked at subtype granularity below
		}

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

	checkEntitiesDeclaration(t, mounted, declaredEntityTypes)
}
