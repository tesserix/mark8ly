package routemanifest_test

import (
	"flag"
	"go/ast"
	"go/parser"
	"go/token"
	"log/slog"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/auth"
	"github.com/mark8ly/marketplace-api/internal/authz"
	"github.com/mark8ly/marketplace-api/internal/depsfill"
	"github.com/mark8ly/marketplace-api/internal/handlers/admin"
	"github.com/mark8ly/marketplace-api/internal/handlers/platformadmin"
	"github.com/mark8ly/marketplace-api/internal/handlers/public"
	"github.com/mark8ly/marketplace-api/internal/handlers/storefront"
	"github.com/mark8ly/marketplace-api/internal/routemanifest"
)

// update regenerates route-manifest.json instead of asserting against it.
// The failure messages below name the exact command, because a guard whose
// fix is undiscoverable gets deleted rather than regenerated.
var update = flag.Bool("update", false, "rewrite route-manifest.json from the real gin trees")

// apiPrefix is the router group main.go passes to every registrar except
// platformadmin, which owns its own MountPrefix.
const apiPrefix = "/api/v1"

// internalPrefixes are the cluster-internal surfaces this manifest
// deliberately excludes. They are not part of the frontend-facing API: the
// callers are cron jobs, platform-api, auth-bff, Resend and the storefront
// gate Worker, all of which authenticate with a shared secret rather than a
// user session.
//
// admin.RegisterAdmin mounts some of them itself, which is why the filter
// lives here rather than being assumed away — and why
// TestManifestDeclaresNoInternalRoutes asserts none survived. Without that
// assertion the scope boundary would rot into a blind spot the first time a
// registrar moved a route under /internal.
var internalPrefixes = []string{"/internal", apiPrefix + "/internal"}

// buildSurfaces registers every in-scope subtree on its OWN fresh
// gin.New(), reads gin's own route table, and returns the manifest that
// describes it.
//
// Routes come from the REAL gin trees, never from parsing source and never
// from a hand-kept list — the anti-pattern internal/handlers/admin/
// route_parity_test.go rejects at length, having already bitten this
// codebase once.
//
// func main() cannot be called: every registrar call site is inside it,
// wrapped in config loading, a database connection and a Stripe client. So
// each subtree is built here the way main.go builds it, and
// TestMainMountsExactlyTheseSurfaces uses the AST to check this list has
// not drifted from what main.go actually mounts.
func buildSurfaces(t *testing.T) []routemanifest.Surface {
	t.Helper()
	gin.SetMode(gin.TestMode)

	surfaces := []routemanifest.Surface{
		{
			Name:      "admin",
			Registrar: "admin.RegisterAdmin",
			Mount:     apiPrefix,
			Routes:    collect(t, "admin", func(g *gin.RouterGroup) { admin.RegisterAdmin(g, adminDeps(t)) }, apiPrefix),
		},
		{
			Name:      "mobile-admin",
			Registrar: "admin.RegisterAdminMobile",
			Mount:     apiPrefix,
			Routes:    collect(t, "mobile-admin", func(g *gin.RouterGroup) { admin.RegisterAdminMobile(g, adminMobileDeps(t)) }, apiPrefix),
		},
		{
			Name:      "storefront",
			Registrar: "storefront.RegisterStorefront",
			Mount:     apiPrefix,
			Routes:    collect(t, "storefront", func(g *gin.RouterGroup) { storefront.RegisterStorefront(g, storefrontDeps(t)) }, apiPrefix),
		},
		{
			Name:      "public",
			Registrar: "public.RegisterPublic",
			Mount:     apiPrefix,
			Routes:    collect(t, "public", func(g *gin.RouterGroup) { public.RegisterPublic(g, publicDeps(t)) }, apiPrefix),
		},
		{
			Name:      "platform",
			Registrar: "platformadmin.Register",
			Mount:     platformadmin.MountPrefix,
			Routes: collect(t, "platform", func(g *gin.RouterGroup) {
				platformadmin.Register(g, platformDeps(t))
			}, platformadmin.MountPrefix),
		},
	}

	sort.Slice(surfaces, func(i, j int) bool { return surfaces[i].Name < surfaces[j].Name })
	return surfaces
}

// collect mounts one registrar on a fresh engine and returns its routes as
// sorted "METHOD PATH" strings, dropping the cluster-internal surfaces.
//
// It fails rather than returning an empty list when a registrar mounts
// nothing in scope: an empty surface would reconcile cleanly against an
// empty declaration and the guard would pass while seeing nothing, which is
// the vacuous-pass hazard every route test in this service warns about.
func collect(t *testing.T, name string, register func(*gin.RouterGroup), mount string) []string {
	t.Helper()

	engine := gin.New()
	register(engine.Group(mount))

	var out []string
	for _, r := range engine.Routes() {
		if isInternal(r.Path) {
			continue
		}
		out = append(out, r.Method+" "+r.Path)
	}
	sort.Strings(out)

	require.NotEmptyf(t, out, "surface %q mounted no in-scope routes — its Deps wiring is "+
		"incomplete, and every assertion against it would pass vacuously", name)
	return out
}

func isInternal(path string) bool {
	for _, p := range internalPrefixes {
		if path == p || strings.HasPrefix(path, p+"/") {
			return true
		}
	}
	return false
}

// manifestPath resolves route-manifest.json relative to THIS TEST FILE, not
// the working directory `go test` happens to run from. This package sits
// four directories below the repo root.
func manifestPath(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	require.True(t, ok, "runtime.Caller(0) failed to report this test file's own path")
	return filepath.Join(filepath.Dir(thisFile), "..", "..", "..", "..", "route-manifest.json")
}

// TestRouteManifestMatchesMountedRoutes is #834's guard. It reconciles
// route-manifest.json against the REAL gin trees in BOTH directions:
//
//   - DECLARED BUT NOT MOUNTED — someone deleted a route. This is #826: the
//     arbitrage-appeal endpoint went, apps/admin kept its page, form, proxy,
//     banner, client, hooks and schemas, and tsc, vitest, next build and an
//     e2e spec that mocked the deleted endpoint all stayed green.
//   - MOUNTED BUT NOT DECLARED — someone added a route without regenerating,
//     so the manifest stops being the thing a frontend guard can trust.
//
// A one-directional check would let the deletion pass silently, which is the
// whole bug.
func TestRouteManifestMatchesMountedRoutes(t *testing.T) {
	path := manifestPath(t)
	built := buildSurfaces(t)

	if *update {
		require.NoError(t, routemanifest.Save(path, routemanifest.Doc{Surfaces: built}))
		t.Logf("wrote %s (%d surfaces, %d routes)", path, len(built), countRoutes(built))
		return
	}

	doc, err := routemanifest.Load(path)
	require.NoError(t, err)
	require.NotEmpty(t, doc.Surfaces,
		"route-manifest.json declares no surfaces — every assertion below would "+
			"pass vacuously. Regenerate it: "+routemanifest.UpdateCommand)

	declared := make(map[string]routemanifest.Surface, len(doc.Surfaces))
	for _, s := range doc.Surfaces {
		_, dup := declared[s.Name]
		require.Falsef(t, dup, "route-manifest.json declares surface %q twice — the second "+
			"copy shadows the first, so half its routes are unchecked", s.Name)
		declared[s.Name] = s
	}

	for _, mounted := range built {
		s, ok := declared[mounted.Name]
		require.Truef(t, ok, "surface %q is MOUNTED but route-manifest.json does not declare it "+
			"at all — regenerate the manifest: %s", mounted.Name, routemanifest.UpdateCommand)
		delete(declared, mounted.Name)

		missing, extra := routemanifest.Diff(s.Routes, mounted.Routes)

		require.Emptyf(t, missing,
			"DECLARED BUT NOT MOUNTED on surface %q: %v\n\n"+
				"route-manifest.json declares these routes and %s no longer mounts them. "+
				"This is #826's bug shape: a backend route was deleted while the frontend "+
				"kept calling it, and tsc, vitest, next build and e2e all stayed green.\n\n"+
				"If the deletion is intended, find and remove every frontend caller FIRST, "+
				"then regenerate: %s",
			mounted.Name, missing, s.Registrar, routemanifest.UpdateCommand)

		require.Emptyf(t, extra,
			"MOUNTED BUT NOT DECLARED on surface %q: %v\n\n"+
				"%s mounts these routes and route-manifest.json does not declare them, so "+
				"the frontend guard that reads this file cannot see them. Regenerate: %s",
			mounted.Name, extra, s.Registrar, routemanifest.UpdateCommand)

		require.Equalf(t, mounted.Registrar, s.Registrar,
			"surface %q declares registrar %q but is built from %q — regenerate: %s",
			mounted.Name, s.Registrar, mounted.Registrar, routemanifest.UpdateCommand)
	}

	for name := range declared {
		t.Errorf("route-manifest.json declares surface %q that no test builds — either it "+
			"was removed from buildSurfaces without regenerating the manifest, or it is a "+
			"surface nothing mounts any more. Regenerate: %s", name, routemanifest.UpdateCommand)
	}
}

// TestManifestDeclaresNoInternalRoutes keeps the scope boundary from rotting
// into a blind spot. /internal is deliberately out of scope — its callers are
// cron jobs and sibling services holding a shared secret, not a browser — but
// admin.RegisterAdmin mounts part of it, so the exclusion is an active filter
// rather than a happy accident. If a registrar ever moves a frontend-facing
// route under /internal, collect() would drop it and the manifest would shrink
// silently; this is the assertion that says so out loud.
func TestManifestDeclaresNoInternalRoutes(t *testing.T) {
	doc, err := routemanifest.Load(manifestPath(t))
	require.NoError(t, err)

	for _, s := range doc.Surfaces {
		for _, route := range s.Routes {
			_, path, found := strings.Cut(route, " ")
			require.Truef(t, found, "surface %q has malformed entry %q — expected \"METHOD PATH\"", s.Name, route)
			require.Falsef(t, isInternal(path),
				"surface %q declares %q, which is on the cluster-internal surface this "+
					"manifest deliberately excludes. Either the route moved and should move "+
					"back, or /internal is now frontend-facing and internalPrefixes needs "+
					"revisiting — do not just regenerate.", s.Name, route)
		}
	}
}

// TestMainMountsExactlyTheseSurfaces answers the one question gin cannot:
// whether cmd/marketplace-api/main.go actually mounts the registrars this
// package builds.
//
// Parsing source is forbidden for "which routes does a subtree register" —
// gin answers that, above. It is the ONLY way to answer "does main.go mount
// this subtree", because all wiring lives inside func main() and cannot be
// called. Keeping the two questions apart is what makes this not a
// contradiction; wiring_test.go in cmd/marketplace-api does the same for
// platformadmin.Deps field parity.
//
// storefront.RegisterMobileStorefront is the reason this test exists. It is
// defined, exported and complete, and main.go never calls it — see its doc
// comment: its only client shipped and was deleted in #792, and it has no
// customer bearer verifier left. Declaring its routes would fill the
// manifest with an entire subtree the service does not serve, which is the
// over-declaring failure: a frontend guard would then bless calls that 404.
func TestMainMountsExactlyTheseSurfaces(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	require.True(t, ok, "runtime.Caller(0) failed to report this test file's own path")
	mainPath := filepath.Join(filepath.Dir(thisFile), "..", "..", "cmd", "marketplace-api", "main.go")

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, mainPath, nil, 0)
	require.NoError(t, err)

	called := map[string]bool{}
	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		pkg, ok := sel.X.(*ast.Ident)
		if !ok {
			return true
		}
		called[pkg.Name+"."+sel.Sel.Name] = true
		return true
	})
	require.NotEmpty(t, called, "parsed no calls out of main.go — the AST walk is broken, "+
		"and both assertions below would pass vacuously")

	for _, s := range buildSurfaces(t) {
		require.Truef(t, called[s.Registrar],
			"buildSurfaces declares surface %q from %s, but main.go never calls it. Either "+
				"the mount was removed — in which case the surface must come out of the "+
				"manifest, not stay in it — or the registrar was renamed.",
			s.Name, s.Registrar)
	}

	require.Falsef(t, called["storefront.RegisterMobileStorefront"],
		"main.go now calls storefront.RegisterMobileStorefront, so /api/v1/mobile/storefront "+
			"is a real surface. Add it to buildSurfaces and regenerate the manifest (%s), or "+
			"the whole mobile storefront subtree stays outside this guard.",
		routemanifest.UpdateCommand)
}

// TestPlatformDepsWiresEveryMountGuard is the vacuous-pass guard for the one
// surface depsfill cannot drive. platformadmin.Register gates nearly every
// route on an interface field, and reflection cannot invent an
// implementation, so platformDeps hand-wires them — and a field forgotten
// there silently removes real routes from the manifest without any
// assertion failing.
func TestPlatformDepsWiresEveryMountGuard(t *testing.T) {
	// Exceptions must earn their place, exactly as in route_parity_test.go:
	// each names a field Register itself substitutes for, so leaving it nil
	// mounts strictly more than setting it would.
	exceptions := map[string]string{
		"NonceStore":           "Register builds a Postgres-backed store from DB when nil; supplying one changes nothing about which routes mount",
		"InboxActionExecutors": "an empty executor list still mounts the action route — it answers 501 per kind, which is the state most kinds are in",
	}

	v := reflect.ValueOf(platformDeps(t))
	seen := map[string]bool{}
	for i := 0; i < v.NumField(); i++ {
		name := v.Type().Field(i).Name
		seen[name] = true
		if _, exempt := exceptions[name]; exempt {
			continue
		}
		switch v.Field(i).Kind() {
		case reflect.Ptr, reflect.Interface, reflect.Func, reflect.Slice, reflect.Map:
			require.Falsef(t, v.Field(i).IsNil(),
				"platformDeps leaves platformadmin.Deps.%s nil — every route Register gates "+
					"on it is invisible to the manifest, which would then look authoritative "+
					"and be incomplete. Wire it, or add it to this test's exceptions with a "+
					"reason.", name)
		}
	}
	for name := range exceptions {
		require.Truef(t, seen[name],
			"this test exempts platformadmin.Deps.%s, which no longer exists — a stale "+
				"exception hides whatever replaced it", name)
	}
}

func countRoutes(surfaces []routemanifest.Surface) int {
	n := 0
	for _, s := range surfaces {
		n += len(s.Routes)
	}
	return n
}

// ---------------------------------------------------------------------------
// Deps wiring, one helper per registrar.
//
// depsfill fills the handler POINTERS so no `if deps.XHandler != nil` guard
// hides a route; anything reflection cannot invent — interfaces, and the
// middleware constructors — is supplied by hand below, mirroring what
// main.go passes. It is the same helper internal/handlers/admin's route
// parity test uses, deliberately: two divergent copies of that reflection
// walk would leave one guard blind to a subset of routes.
// ---------------------------------------------------------------------------

func adminDeps(t *testing.T) admin.Deps {
	t.Helper()
	var deps admin.Deps
	depsfill.Struct(&deps)
	deps.AuthzMiddleware = authz.NewMiddleware(nil, nil)
	return deps
}

func adminMobileDeps(t *testing.T) admin.MobileDeps {
	t.Helper()
	var deps admin.MobileDeps
	depsfill.Struct(&deps)
	deps.AuthzMiddleware = authz.NewMiddleware(nil, nil)
	// Interface field — reflection cannot invent an implementation, and a
	// nil verifier makes RegisterAdminMobile return without registering
	// anything at all.
	deps.TokenVerifier = &auth.FakeVerifier{}
	// TenantMembershipChecker/-Logger back auth.TenantFromRequest.
	// Registration only takes method values, so a grant-less fake client is
	// enough.
	deps.TenantMembershipChecker = authz.NewFakeClient()
	deps.TenantMembershipLogger = slog.Default()
	return deps
}

func storefrontDeps(t *testing.T) storefront.Deps {
	t.Helper()
	var deps storefront.Deps
	depsfill.Struct(&deps)
	deps.Logger = slog.Default()
	// Interface fields; CountryHandler gates
	// GET /api/v1/public/supported-countries, which is registered by THIS
	// registrar rather than by package public.
	deps.CountryHandler = stubCountryLister{}
	deps.CustomerService = stubCustomerProfileService{}
	return deps
}

func publicDeps(t *testing.T) public.PublicDeps {
	t.Helper()
	var deps public.PublicDeps
	depsfill.Struct(&deps)
	return deps
}
