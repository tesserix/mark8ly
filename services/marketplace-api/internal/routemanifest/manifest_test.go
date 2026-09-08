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
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/auth"
	"github.com/mark8ly/marketplace-api/internal/authz"
	"github.com/mark8ly/marketplace-api/internal/depsfill"
	"github.com/mark8ly/marketplace-api/internal/handlers/admin"
	"github.com/mark8ly/marketplace-api/internal/handlers/internalsvc"
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

// internalPrefixes marks the cluster-internal path space. Most of it is out
// of scope for this manifest — cron jobs, platform-api, auth-bff and Resend
// call those routes with a shared secret, not a browser session — and
// admin.RegisterAdmin mounts part of it, so surfaces that are NOT the
// internal surface filter it out.
//
// It is emphatically NOT a claim that no frontend calls /internal. An
// earlier version of this file asserted exactly that, and it was wrong:
// three apps/* files fetch GET /internal/store-active-domain/:slug. That
// assertion would have locked the blind spot in permanently, because it
// asserted the scope was right instead of testing whether it was. What
// tests it now is TestFrontendInternalRoutesAreDeclared, which derives the
// frontend's internal references from source and requires every one of
// them to be in the manifest.
var internalPrefixes = []string{"/internal", apiPrefix + "/internal"}

// internalMount is the router group main.go passes to the internal
// registrars (see the internalsvc calls around cmd/marketplace-api/main.go
// :2680-2745).
const internalMount = "/internal"

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
			Routes:    collect(t, "admin", func(g *gin.RouterGroup) { admin.RegisterAdmin(g, adminDeps(t)) }, apiPrefix, excludeInternal),
		},
		{
			Name:      "mobile-admin",
			Registrar: "admin.RegisterAdminMobile",
			Mount:     apiPrefix,
			Routes:    collect(t, "mobile-admin", func(g *gin.RouterGroup) { admin.RegisterAdminMobile(g, adminMobileDeps(t)) }, apiPrefix, excludeInternal),
		},
		{
			Name:      "storefront",
			Registrar: "storefront.RegisterStorefront",
			Mount:     apiPrefix,
			Routes:    collect(t, "storefront", func(g *gin.RouterGroup) { storefront.RegisterStorefront(g, storefrontDeps(t)) }, apiPrefix, excludeInternal),
		},
		{
			Name:      "public",
			Registrar: "public.RegisterPublic",
			Mount:     apiPrefix,
			Routes:    collect(t, "public", func(g *gin.RouterGroup) { public.RegisterPublic(g, publicDeps(t)) }, apiPrefix, excludeInternal),
		},
		{
			Name:      "platform",
			Registrar: "platformadmin.Register",
			Mount:     platformadmin.MountPrefix,
			Routes: collect(t, "platform", func(g *gin.RouterGroup) {
				platformadmin.Register(g, platformDeps(t))
			}, platformadmin.MountPrefix, excludeInternal),
		},
		{
			// The ONE cluster-internal registrar in scope, because three
			// apps/* files fetch its route: apps/admin/middleware.ts,
			// apps/admin/lib/auth/cross-domain-handoff.ts and
			// apps/storefront/middleware.ts all GET
			// /internal/store-active-domain/:slug, and all three fail OPEN
			// on a non-2xx (`return null`) — so deleting it would be
			// invisible at runtime as well as in CI. That is the exact
			// silent-deletion shape this instrument exists to catch, which
			// makes it the last route that should have been left out.
			//
			// Scoped to this ONE registrar rather than the whole /internal
			// tree: the other internal mounts have no frontend caller, and
			// several need dependencies (a live *gorm.DB behind
			// tenantpurge.Purge, a subscription.Service, an audit emitter
			// bound to real secrets) that cannot be constructed here. See
			// TestFrontendInternalRoutesAreDeclared for what keeps that
			// narrow scope honest as apps/* changes.
			Name:      "internal-store-active-domain",
			Registrar: "internalsvc.NewStoreActiveDomainHandler.Register",
			Mount:     internalMount,
			Routes: collect(t, "internal-store-active-domain", func(g *gin.RouterGroup) {
				internalsvc.NewStoreActiveDomainHandler(&gorm.DB{}).
					Register(g, "route-manifest-unused-secret")
			}, internalMount, keepInternal),
		},
	}

	sort.Slice(surfaces, func(i, j int) bool { return surfaces[i].Name < surfaces[j].Name })
	return surfaces
}

// internalPolicy says whether a surface's own routes live in the
// cluster-internal path space. Every surface but one excludes it; naming the
// choice per surface — rather than deriving it from the mount prefix — keeps
// admin's /api/v1/internal cron routes out while letting the internal
// surface in.
type internalPolicy bool

const (
	excludeInternal internalPolicy = false
	keepInternal    internalPolicy = true
)

// collect mounts one registrar on a fresh engine and returns its routes as
// sorted "METHOD PATH" strings, dropping the cluster-internal path space for
// every surface that does not itself live there.
//
// It deliberately does NOT fail on an empty result, and that is a correction
// rather than an omission. It used to, and the first sabotage against the
// single-route internal surface proved the check was in the wrong place:
// deleting that route emptied the surface, the emptiness check fired FIRST,
// and the guard went red saying "its Deps wiring is incomplete" — sending
// whoever deleted the route to debug a test helper instead of telling them a
// frontend calls it. A guard that fails for the wrong reason reads exactly
// like one that works.
//
// The vacuous-pass hazard is real, so the check moved rather than
// disappearing, to the two places where an empty surface is genuinely
// indistinguishable from a broken harness: writing the manifest (never write
// an empty surface) and asserting against it (a surface empty on BOTH sides
// is seeing nothing). Everything else is a route diff, and gets a route
// diff's message.
func collect(t *testing.T, name string, register func(*gin.RouterGroup), mount string, internal internalPolicy) []string {
	t.Helper()

	engine := gin.New()
	register(engine.Group(mount))

	var out []string
	for _, r := range engine.Routes() {
		if internal == excludeInternal && isInternal(r.Path) {
			continue
		}
		out = append(out, r.Method+" "+r.Path)
	}
	sort.Strings(out)
	_ = name // named for the caller's benefit; emptiness is judged by the caller
	return out
}

// wiringMessage is the failure for a surface that is seeing nothing at all.
// Shared so the write path and the assert path cannot describe it differently.
const wiringMessage = "surface %q mounted no in-scope routes and declares none — its Deps " +
	"wiring is incomplete (an interface field left nil unmounts every route it gates), " +
	"so every assertion against it would pass vacuously"

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
		// Checked before writing, not after: an empty surface written into the
		// manifest is a blind spot that then reconciles cleanly forever.
		for _, s := range built {
			require.NotEmptyf(t, s.Routes,
				"refusing to write route-manifest.json: surface %q mounted no in-scope "+
					"routes, so regenerating would erase whatever it used to declare and "+
					"the guard would pass against nothing. Fix the Deps wiring in this "+
					"package first.", s.Name)
		}
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

		// A surface empty on BOTH sides is the only case a route diff cannot
		// describe, because there is no route to name. Every other emptiness
		// shows up below as a full set of missing or extra routes, with the
		// message that actually tells the reader what happened.
		require.Falsef(t, len(mounted.Routes) == 0 && len(s.Routes) == 0,
			wiringMessage, mounted.Name)

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
		switch recv := sel.X.(type) {
		case *ast.Ident:
			// pkg.Func(...) — the four package-level registrars.
			called[recv.Name+"."+sel.Sel.Name] = true
		case *ast.CallExpr:
			// pkg.NewThing(...).Method(...) — how the internal surface is
			// mounted. Recorded as "pkg.NewThing.Method" so a surface whose
			// registrar is a method on a constructor result is checkable
			// too, rather than silently exempt from this test.
			if inner, ok := recv.Fun.(*ast.SelectorExpr); ok {
				if pkg, ok := inner.X.(*ast.Ident); ok {
					called[pkg.Name+"."+inner.Sel.Name+"."+sel.Sel.Name] = true
				}
			}
		}
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

// TestEveryDepsWiresEveryMountGuard is the vacuous-pass guard for the
// manifest's contents, and the reason it covers ALL FIVE Deps types rather
// than just platformadmin's: a manifest that can silently SHRINK is #826
// restated. Every registrar in this service gates routes on
// `if deps.X != nil`, so one unwired field removes real routes from the
// manifest with nothing failing — the file simply looks authoritative and is
// incomplete, which the brief for this task rightly called worse than not
// building it at all.
//
// depsfill covers the handler pointers. What it cannot cover is any INTERFACE
// field, because reflection cannot invent an implementation — so a new
// route-gating interface on storefront.Deps or public.PublicDeps would slip
// through with no assertion failing. That is the hole this closes.
// internal/handlers/admin has its own assertNoNilDeps for its two Deps types;
// they are checked here as well, because this package builds its own wiring
// for them and a divergence between the two is exactly what would go unseen.
func TestEveryDepsWiresEveryMountGuard(t *testing.T) {
	// Exceptions must earn their place, exactly as in route_parity_test.go:
	// each names a field the registrar itself substitutes for or treats as
	// genuinely optional, so leaving it nil mounts strictly MORE routes than
	// setting it would — never fewer. Any other nil is a bug in the wiring
	// above, not a case for a new entry here.
	cases := []struct {
		name       string
		deps       any
		exceptions map[string]string
	}{
		{
			name: "admin.Deps",
			deps: adminDeps(t),
		},
		{
			name: "admin.MobileDeps",
			deps: adminMobileDeps(t),
		},
		{
			name: "storefront.Deps",
			deps: storefrontDeps(t),
		},
		{
			name: "public.PublicDeps",
			deps: publicDeps(t),
		},
		{
			name: "platformadmin.Deps",
			deps: platformDeps(t),
			exceptions: map[string]string{
				"NonceStore":           "Register builds a Postgres-backed store from DB when nil; supplying one changes nothing about which routes mount",
				"InboxActionExecutors": "an empty executor list still mounts the action route — it answers 501 per kind, which is the state most kinds are in",
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			v := reflect.ValueOf(tc.deps)
			require.Equal(t, reflect.Struct, v.Kind(), "%s is not a struct", tc.name)

			seen := map[string]bool{}
			for i := 0; i < v.NumField(); i++ {
				name := v.Type().Field(i).Name
				seen[name] = true
				if _, exempt := tc.exceptions[name]; exempt {
					continue
				}
				switch v.Field(i).Kind() {
				case reflect.Ptr, reflect.Interface, reflect.Func, reflect.Slice, reflect.Map:
					require.Falsef(t, v.Field(i).IsNil(),
						"%s.%s is nil in this package's wiring — every route its registrar "+
							"gates on it is invisible to the manifest, which would then look "+
							"authoritative and be incomplete. Wire it (an interface field must "+
							"be wired by hand; depsfill cannot invent an implementation), or "+
							"add it to this case's exceptions with a reason.", tc.name, name)
				}
			}
			for name := range tc.exceptions {
				require.Truef(t, seen[name],
					"this case exempts %s.%s, which no longer exists — a stale exception "+
						"hides whatever replaced it", tc.name, name)
			}
		})
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
