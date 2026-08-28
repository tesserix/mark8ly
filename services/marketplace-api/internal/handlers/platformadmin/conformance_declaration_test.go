package platformadmin_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

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
func declarationRouterDeps(t *testing.T) platformadmin.Deps {
	return allReadRoutesDeps(t)
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
	platformadmin.Register(r.Group(platformadmin.MountPrefix), declarationRouterDeps(t))

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

// chartValuesPath resolves the SIBLING tesserix-k8s repo's Helm values for
// this service, relative to THIS TEST FILE (not the working directory `go
// test` happens to run from), the same way conformanceDeclarationPath does
// for admin-conformance.json. This package sits five directories below
// mark8ly's repo root; one more ".." reaches the directory that is
// expected to hold mark8ly and tesserix-k8s as siblings (the layout this
// task was run against: .../tesserix-new/{mark8ly,tesserix-k8s}).
func chartValuesPath(t *testing.T) string {
	t.Helper()

	_, thisFile, _, ok := runtime.Caller(0)
	require.True(t, ok, "runtime.Caller(0) failed to report this test file's own path")

	workspaceRoot := filepath.Join(filepath.Dir(thisFile), "..", "..", "..", "..", "..", "..")
	return filepath.Join(workspaceRoot, "tesserix-k8s", "charts", "apps",
		"mark8ly-marketplace-api-admin", "values.yaml")
}

// chartDeclaration is the shape of adminConformanceCron.declaration.endpoints
// this test cares about, from charts/apps/mark8ly-marketplace-api-admin/
// values.yaml. It deliberately does not model the rest of that values file
// — only this one block is the second copy of the declaration that
// admin-conformance.json duplicates (mark8ly#290). Endpoint values are kept
// as interface{} for the same reason conformanceDoc keeps them as
// json.RawMessage: most ids are a bare bool, `entities` and `inbox` are
// small maps.
type chartDeclaration struct {
	AdminConformanceCron struct {
		Declaration struct {
			Endpoints map[string]interface{} `yaml:"endpoints"`
		} `yaml:"declaration"`
	} `yaml:"adminConformanceCron"`
}

func readChartDeclaration(t *testing.T, path string) chartDeclaration {
	t.Helper()

	raw, err := os.ReadFile(path)
	require.NoError(t, err, "failed to read %s", path)

	var v chartDeclaration
	require.NoErrorf(t, yaml.Unmarshal(raw, &v),
		"%s is not valid YAML, or its shape does not match "+
			"adminConformanceCron.declaration.endpoints", path)
	return v
}

func chartDeclaredEndpointIDs(v chartDeclaration) map[string]bool {
	ids := make(map[string]bool, len(v.AdminConformanceCron.Declaration.Endpoints))
	for id := range v.AdminConformanceCron.Declaration.Endpoints {
		ids[id] = true
	}
	return ids
}

// chartDeclaredEntityTypes parses the chart's entities.types list, the
// direct counterpart to readDeclaredEntityTypes for admin-conformance.json.
func chartDeclaredEntityTypes(t *testing.T, v chartDeclaration) map[string]bool {
	t.Helper()

	raw, present := v.AdminConformanceCron.Declaration.Endpoints["entities"]
	types := map[string]bool{}
	if !present {
		return types
	}

	entry, ok := raw.(map[string]interface{})
	require.Truef(t, ok, "chart's entities declaration is not a {types: [...]} map: %#v", raw)

	rawTypes, ok := entry["types"].([]interface{})
	require.Truef(t, ok, "chart's entities.types is not a list: %#v", entry["types"])

	for _, ty := range rawTypes {
		s, ok := ty.(string)
		require.Truef(t, ok, "chart's entities.types contains a non-string entry: %#v", ty)
		types[s] = true
	}
	return types
}

// inboxSLADeclaration is one side's answer to "which inbox kinds carry an
// SLA" — design-system's declaration.ts accepts either slaKinds (a per-kind
// set) or slaDeclared (a per-queue boolean), and treats them as mutually
// exclusive: declaring both is a parse error there. usesSlaKinds records
// which shape this side actually used, so the comparison below can catch
// "one side moved to slaKinds and the other is still on slaDeclared" as its
// own disagreement, not silently compare an empty set against another.
type inboxSLADeclaration struct {
	usesSlaKinds bool
	kinds        map[string]bool // meaningful only when usesSlaKinds is true
}

func inboxSLAModeName(usesSlaKinds bool) string {
	if usesSlaKinds {
		return "slaKinds"
	}
	return "slaDeclared"
}

// chartInboxSLA reads the chart's inbox declaration and reports which shape
// it used. Only meaningful when the caller has already confirmed "inbox" is
// declared on both sides — see the id-set comparison in
// TestConformanceDeclarationMatchesChartCopy, which catches "declared on one
// side only" before this is ever called.
func chartInboxSLA(t *testing.T, v chartDeclaration) inboxSLADeclaration {
	t.Helper()

	raw, present := v.AdminConformanceCron.Declaration.Endpoints["inbox"]
	require.True(t, present, "chartInboxSLA called but the chart does not declare \"inbox\"")

	entry, ok := raw.(map[string]interface{})
	require.Truef(t, ok, "chart's inbox declaration is not a map: %#v", raw)

	if rawKinds, present := entry["slaKinds"]; present {
		list, ok := rawKinds.([]interface{})
		require.Truef(t, ok, "chart's inbox.slaKinds is not a list: %#v", rawKinds)

		kinds := make(map[string]bool, len(list))
		for _, k := range list {
			s, ok := k.(string)
			require.Truef(t, ok, "chart's inbox.slaKinds contains a non-string entry: %#v", k)
			kinds[s] = true
		}
		return inboxSLADeclaration{usesSlaKinds: true, kinds: kinds}
	}

	_, present = entry["slaDeclared"]
	require.Truef(t, present,
		"chart's inbox declaration has neither slaKinds nor slaDeclared: %#v", entry)
	return inboxSLADeclaration{usesSlaKinds: false}
}

// repoInboxSLA is the admin-conformance.json counterpart to chartInboxSLA.
func repoInboxSLA(t *testing.T, doc conformanceDoc) inboxSLADeclaration {
	t.Helper()

	raw, present := doc.Endpoints["inbox"]
	require.True(t, present, "repoInboxSLA called but admin-conformance.json does not declare \"inbox\"")

	var parsed map[string]json.RawMessage
	require.NoErrorf(t, json.Unmarshal(raw, &parsed),
		"admin-conformance.json's \"inbox\" declaration is not a JSON object: %s", string(raw))

	if rawKinds, present := parsed["slaKinds"]; present {
		var list []string
		require.NoErrorf(t, json.Unmarshal(rawKinds, &list),
			"admin-conformance.json's \"inbox\".\"slaKinds\" is not an array of strings: %s",
			string(rawKinds))

		kinds := make(map[string]bool, len(list))
		for _, k := range list {
			kinds[k] = true
		}
		return inboxSLADeclaration{usesSlaKinds: true, kinds: kinds}
	}

	_, present = parsed["slaDeclared"]
	require.Truef(t, present,
		"admin-conformance.json's \"inbox\" declaration has neither slaKinds nor "+
			"slaDeclared: %s", string(raw))
	return inboxSLADeclaration{usesSlaKinds: false}
}

// TestConformanceDeclarationMatchesChartCopy is mark8ly#290's stopgap, not
// its fix.
//
// admin-conformance.json (checked by TestConformanceDeclarationMatchesMountedRoutes
// above) is NOT the declaration the nightly conformance CronJob reads. That
// job reads a second, hand-maintained copy: adminConformanceCron.declaration
// in the SIBLING tesserix-k8s repo's
// charts/apps/mark8ly-marketplace-api-admin/values.yaml, rendered into a
// ConfigMap by that chart's admin-conformance-configmap.yaml template. That
// template's own comment says, in bold, "If you change
// mark8ly/admin-conformance.json, change this too" and names mark8ly#290 as
// the follow-up to actually enforce it. Nothing did, until now — and even
// this does not really enforce it.
//
// Two repos, two languages, no shared CI: mark8ly's own test suite cannot
// see tesserix-k8s unless it happens to be checked out as a sibling
// directory, and mark8ly's CI never has it checked out. So this test can
// only be a REAL check for a developer who has both repos on disk locally
// — typically the person editing one copy of the declaration, who is
// exactly the person who most needs to be told the other copy now
// disagrees. In mark8ly's own CI (and for anyone without the sibling
// checked out) the chart file is absent and this test SKIPS: it reports
// "not checked" rather than failing, because a hard failure here would make
// every mark8ly CI run depend on a checkout this repo cannot control.
//
// That means this test cannot be the last line of defence against drift —
// it says nothing to whoever only ever looks at mark8ly CI, and nothing
// stops someone from editing the chart copy without ever running mark8ly's
// tests at all. The true fix is removing the duplication entirely
// (mark8ly#290: generate the ConfigMap from admin-conformance.json instead
// of hand-copying it, or have the CronJob fetch admin-conformance.json
// directly). Until that lands, this test's only honest claim is: it makes
// drift visible to the person creating it, at authoring time, instead of
// leaving it to whoever next reads a red CronJob three weeks later.
func TestConformanceDeclarationMatchesChartCopy(t *testing.T) {
	chartPath := chartValuesPath(t)
	if _, err := os.Stat(chartPath); err != nil {
		t.Skipf("sibling repo tesserix-k8s not found at %s (resolved relative to "+
			"this test file, assuming mark8ly and tesserix-k8s are sibling "+
			"directories) — NOT CHECKED: whether "+
			"charts/apps/mark8ly-marketplace-api-admin/values.yaml's "+
			"adminConformanceCron.declaration (the copy the nightly CronJob "+
			"actually reads) agrees with this repo's admin-conformance.json. "+
			"This is expected in mark8ly's own CI, which never has tesserix-k8s "+
			"checked out — see this test's doc comment for why that makes it a "+
			"stopgap, not a guarantee: %v", chartPath, err)
	}

	doc := readConformanceDoc(t, conformanceDeclarationPath(t))
	repoIDs := readDeclaredEndpointIDs(t, doc)
	repoEntityTypes := readDeclaredEntityTypes(t, doc)

	chart := readChartDeclaration(t, chartPath)
	chartIDs := chartDeclaredEndpointIDs(chart)
	chartEntityTypes := chartDeclaredEntityTypes(t, chart)

	for id := range repoIDs {
		require.Truef(t, chartIDs[id],
			"admin-conformance.json declares endpoint %q but "+
				"charts/apps/mark8ly-marketplace-api-admin/values.yaml's "+
				"adminConformanceCron.declaration.endpoints does not — the nightly "+
				"CronJob reads the chart copy, so it will never check %q until the "+
				"chart is updated to match (mark8ly#290)", id, id)
	}
	for id := range chartIDs {
		require.Truef(t, repoIDs[id],
			"the chart's adminConformanceCron.declaration.endpoints declares "+
				"endpoint %q but admin-conformance.json does not — the nightly "+
				"CronJob will check %q against production even though this repo's "+
				"own declaration doesn't claim to serve it (mark8ly#290)", id, id)
	}

	for ty := range repoEntityTypes {
		require.Truef(t, chartEntityTypes[ty],
			"admin-conformance.json's entities.types declares %q but the chart's "+
				"entities.types does not (mark8ly#290)", ty)
	}
	for ty := range chartEntityTypes {
		require.Truef(t, repoEntityTypes[ty],
			"the chart's entities.types declares %q but admin-conformance.json's "+
				"entities.types does not (mark8ly#290)", ty)
	}

	if repoIDs["inbox"] && chartIDs["inbox"] {
		repoSLA := repoInboxSLA(t, doc)
		chartSLA := chartInboxSLA(t, chart)

		// A mode mismatch (one side on slaKinds, the other still on
		// slaDeclared) is a disagreement in its own right, independent of
		// which kinds either side names — comparing an empty kind set
		// against a populated one would otherwise silently pass.
		require.Equalf(t, repoSLA.usesSlaKinds, chartSLA.usesSlaKinds,
			"admin-conformance.json's inbox declares its SLA via %q but the chart's "+
				"adminConformanceCron.declaration.endpoints.inbox declares it via %q "+
				"(mark8ly#290)", inboxSLAModeName(repoSLA.usesSlaKinds), inboxSLAModeName(chartSLA.usesSlaKinds))

		if repoSLA.usesSlaKinds && chartSLA.usesSlaKinds {
			for kind := range repoSLA.kinds {
				require.Truef(t, chartSLA.kinds[kind],
					"admin-conformance.json's inbox.slaKinds declares %q but the chart's "+
						"adminConformanceCron.declaration.endpoints.inbox.slaKinds does not "+
						"(mark8ly#290)", kind)
			}
			for kind := range chartSLA.kinds {
				require.Truef(t, repoSLA.kinds[kind],
					"the chart's adminConformanceCron.declaration.endpoints.inbox.slaKinds "+
						"declares %q but admin-conformance.json's inbox.slaKinds does not "+
						"(mark8ly#290)", kind)
			}
		}
	}
}
