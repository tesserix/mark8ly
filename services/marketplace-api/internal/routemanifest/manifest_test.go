package routemanifest_test

import (
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"log/slog"
	"os"
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
			Routes:    collect(t, func(g *gin.RouterGroup) { admin.RegisterAdmin(g, adminDeps(t)) }, apiPrefix, excludeInternal),
		},
		{
			Name:      "mobile-admin",
			Registrar: "admin.RegisterAdminMobile",
			Mount:     apiPrefix,
			Routes:    collect(t, func(g *gin.RouterGroup) { admin.RegisterAdminMobile(g, adminMobileDeps(t)) }, apiPrefix, excludeInternal),
		},
		{
			Name:      "storefront",
			Registrar: "storefront.RegisterStorefront",
			Mount:     apiPrefix,
			Routes:    collect(t, func(g *gin.RouterGroup) { storefront.RegisterStorefront(g, storefrontDeps(t)) }, apiPrefix, excludeInternal),
		},
		{
			Name:      "public",
			Registrar: "public.RegisterPublic",
			Mount:     apiPrefix,
			Routes:    collect(t, func(g *gin.RouterGroup) { public.RegisterPublic(g, publicDeps(t)) }, apiPrefix, excludeInternal),
		},
		{
			Name:      "platform",
			Registrar: "platformadmin.Register",
			Mount:     platformadmin.MountPrefix,
			Routes: collect(t, func(g *gin.RouterGroup) {
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
			Routes: collect(t, func(g *gin.RouterGroup) {
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
func collect(t *testing.T, register func(*gin.RouterGroup), mount string, internal internalPolicy) []string {
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
		//
		// The LIMIT of this check, stated plainly so nobody infers more from
		// it than it does: it only fires when a surface reaches ZERO routes.
		// Deleting ONE route from a multi-route surface regenerates
		// successfully and leaves the suite green — by design, since a
		// regenerable manifest must let intended deletions through. What
		// catches that is the MANIFEST DIFF in code review: -1 line in
		// route-manifest.json is the review signal, and there is no
		// mechanism here that makes it fail instead.
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

// mountsOutsideManifest are the registrars cmd/marketplace-api's package main
// mounts on a router group that this manifest deliberately does NOT describe.
// Every one must earn its place with a NON-EMPTY reason — the emptiness is
// checked, because a reason nobody checks is a comment, and
// `"admin.RegisterAdmin": "   "` dropped 190 routes with exit 0.
//
// A stale entry — one naming a mount that no longer exists — fails just as
// loudly as an unclassified mount, exactly as route_parity_test.go treats its
// exemptions.
var mountsOutsideManifest = map[string]string{
	"brandingSeeder.Register": "/api/v1/test — e2e/visual seeding, gated by MARKETPLACE_API_ENABLE_TEST_ROUTES and never enabled in a deployed environment, so no frontend can call it",

	"emailevents.NewHandler.Register": "Resend delivery webhooks, mounted at the root group. The caller is Resend and authenticates with the svix-signature over the raw body — not a browser",

	// The cluster-internal surface. One registrar IS in the manifest
	// (internalsvc.NewStoreActiveDomainHandler.Register) because three
	// apps/* files fetch its route; the rest have no frontend caller.
	// TestFrontendInternalRoutesAreDeclared is what will say so if that
	// changes — a new frontend call to any of these fails there by name.
	//
	// The stated callers come from main.go's own comments and each handler's
	// doc, not from observing production traffic. A route labelled here as
	// service-to-service that a frontend also fetches would be wrongly
	// exempt; that specific mistake is what the frontend guard catches.
	"vendorHandler.RegisterRoutes":                             "/internal vendor sync — called by platform-api at onboarding completion with a shared secret",
	"templateHandler.Register":                                 "/internal email-template refresh + test-send — operator/cron surface",
	"internalDomainsHandler.Register":                          "/internal domain re-verify + cert refresh — super-admin actions via the console's own backend, not the browser",
	"stores.NewInternalHandler.RegisterRoutes":                 "/internal stores mirror — written by platform-api at onboarding completion",
	"subscription.NewInternalHandler.WithPromo.RegisterRoutes": "/internal signup subscription + promo callback — called by onboarding's backend with MARKETPLACE_INTERNAL_AUTH_SECRET",
	"ticketInternalHandler.RegisterRoutes":                     "/internal ticket-from-conversation — posted by slm-router on AI-to-human handoff",
	"migrationHandler.RegisterInternalRoutes":                  "/internal CSM migration fast-path review — operator action through the console backend",
	"internalsvc.NewAuditIngestHandler.Register":               "/internal audit ingest — auth-bff and platform-api post login/logout and staff invite events",
	"internalsvc.NewStorefrontStatusHandler.Register":          "/internal storefront status — read at the edge by the storefront-gate Cloudflare Worker",
	"internalsvc.NewActiveDomainsHandler.Register":             "/internal active domains list — read by the OpenPanel CORS reconciler",
	"internalsvc.NewTenantPurgeHandler.Register":               "/internal tenant hard-delete — posted by platform-api's outbox drainer",
}

// directRoutesOutsideManifest are routes registered STRAIGHT onto a router
// (no registrar, no subtree) that the manifest does not describe. Keyed
// "METHOD /path", same non-empty-reason and staleness rules as
// mountsOutsideManifest.
//
// This list exists because two of package main's six direct registrations
// ARE frontend-facing — GET /api/v1/public/supported-countries and
// GET /api/v1/storefront/resolve-domain, registered inline on the split-mode
// engines where the mode.Both engine gets them from RegisterStorefront. Both
// are already in the manifest, and checking them here is what proves the
// split-mode engines agree with it rather than assuming they do.
var directRoutesOutsideManifest = map[string]string{
	"POST /pubsub/merchant-push":    "Pub/Sub push delivery, OIDC-verified in-handler. The caller is Google, not a browser",
	"POST /webhooks/stripe-billing": "Stripe billing webhook, verified by signature over the raw body",
}

// routeMethods are the gin router methods whose FIRST argument is the path.
//
// gin's Handle is deliberately absent: its signature is
// Handle(httpMethod, relativePath string, ...), so the path is the SECOND
// argument — see directRouteFromCall, which reads it correctly. Treating
// Handle as path-first matched net/http's ServeMux.Handle("/metrics", …) in
// metrics_server.go instead, which is a different server on a different
// listener (cfg.MetricsPort, pods annotated prometheus.io/port) and not part
// of the gin API surface this manifest describes. The argument shape is what
// tells the two apart, since this walk is receiver-agnostic by design.
var routeMethods = map[string]bool{
	"GET": true, "POST": true, "PUT": true, "PATCH": true, "DELETE": true,
	"HEAD": true, "OPTIONS": true, "Any": true,
	"Static": true, "StaticFile": true, "StaticFS": true,
}

func isRouteMethod(name string) bool { return routeMethods[name] || name == "Handle" }

// directRouteFromCall renders a direct registration as "METHOD /path",
// reading the path from whichever argument actually holds it: the first for
// the per-verb methods, the second for gin's Handle(method, path, …).
//
// ok is false when no literal path is there — a computed path cannot be
// compared with the manifest. That is a gap rather than a failure, and a
// narrow one: nothing in package main computes a route path today, and the
// group analysis (which DOES fail closed) covers every subtree mount.
func directRouteFromCall(method string, call *ast.CallExpr) (string, bool) {
	if method == "Handle" {
		if len(call.Args) < 2 {
			return "", false
		}
		verb, okV := stringLit(call.Args[0])
		path, okP := stringLit(call.Args[1])
		if !okV || !okP || !strings.HasPrefix(path, "/") {
			return "", false
		}
		return verb + " " + path, true
	}
	if len(call.Args) == 0 {
		return "", false
	}
	path, ok := stringLit(call.Args[0])
	if !ok || !strings.HasPrefix(path, "/") {
		return "", false
	}
	return method + " " + path, true
}

// groupPassThroughMethods are the methods that consume a router group and
// hand back something still group-shaped, so the value stays tracked rather
// than counting as a mount.
var groupPassThroughMethods = map[string]bool{"Use": true, "Group": true}

// groupMount describes one registrar package main mounts on a router group.
type groupMount struct {
	Registrar string // canonical dotted name, e.g. "admin.RegisterAdmin"
	Prefix    string // the group prefix as written, for the failure message
	Where     string // file:line of the mount
}

// mainWiring is everything the AST pass extracts from package main.
type mainWiring struct {
	// CalledNames is every X.Y(...) call name found, WITHOUT regard to its
	// arguments. Deliberately independent of the router-group analysis: the
	// RegisterMobileStorefront assertion is built on this, and when it was
	// briefly rebuilt on the group analysis instead, wiring that registrar
	// through a local variable made the assertion stop firing. A call-name
	// check has no such escape.
	CalledNames map[string]bool
	// Mounts are the registrars that receive a router group.
	Mounts map[string]groupMount
	// DirectRoutes are "METHOD /path" registered straight onto a router.
	DirectRoutes map[string]string // route -> file:line
}

// analyseMainPackage extracts the wiring from EVERY non-test file of
// cmd/marketplace-api's package main — not just main.go, so a helper in a
// sibling file cannot hide a mount.
//
// # Fail closed
//
// This is the design rule, and it is a correction. The previous version
// recognised a mount only when the group was an INLINE `*.Group(...)`
// argument and the call target was `Ident.Sel` or `Call().Sel`. Anything else
// it SKIPPED, silently, and two stylistic variants walked straight through:
//
//	mobileGroup := r.Group("/api/v1")            // group via local variable
//	storefront.RegisterMobileStorefront(mobileGroup, deps)
//
//	sabHolder.Inner.Reg(r.Group("/api/v1"), ...) // nested field-selector target
//
// Both mounted a whole subtree, appeared in neither list, and left
// `go test ./...` at exit 0. Rewriting both admin.RegisterAdmin call sites to
// the local-variable form and dropping the admin surface took the manifest
// from 403 routes to 213 with a green suite — the same 190-route silent drop
// this guard was built to stop, reachable by a refactor that changes no
// behaviour.
//
// Chasing shapes is a losing game, so the invariant is inverted: EVERY
// `.Group(...)` expression in the package must be accounted for, and anything
// this walker cannot classify is a loud failure rather than a skip. It
// accounts for a group expression when it is
//
//   - an argument to a call whose target it can name (a mount), or
//   - bound to a local variable, whose every use is then itself accounted, or
//   - the receiver of a route method (a direct registration), or
//   - the receiver of Use/Group (still group-shaped, keeps being tracked).
//
// Anything else fails, naming the file and line.
//
// # The hole that remains
//
// A router group produced by a function in ANOTHER PACKAGE — `g :=
// httpserver.SomeGroup()` — is invisible: without type information there is
// no way to know the value is a group, and type-checking package main means
// resolving ~100 imports, which is a different tool from a Go test. No such
// shape exists in this repo. This is stated rather than claimed away, because
// the last version of this comment claimed a totality it did not have.
func analyseMainPackage(t *testing.T, dir string) mainWiring {
	t.Helper()

	entries, err := os.ReadDir(dir)
	require.NoError(t, err)

	w := mainWiring{
		CalledNames:  map[string]bool{},
		Mounts:       map[string]groupMount{},
		DirectRoutes: map[string]string{},
	}
	filesParsed := 0

	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}
		analyseMainFile(t, filepath.Join(dir, e.Name()), &w)
		filesParsed++
	}

	require.Greaterf(t, filesParsed, 5,
		"parsed only %d non-test files out of %s — package main had 12 when this test was "+
			"written, and every assertion below would be weakened by a file that never "+
			"got read", filesParsed, dir)
	return w
}

func analyseMainFile(t *testing.T, path string, w *mainWiring) {
	t.Helper()

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, 0)
	require.NoError(t, err)

	where := func(pos token.Pos) string {
		p := fset.Position(pos)
		return fmt.Sprintf("%s:%d", filepath.Base(p.Filename), p.Line)
	}

	// --- pass 1: every .Group(...) expression, and the vars bound to one ---
	groupExprs := map[token.Pos]string{} // position -> prefix
	accounted := map[token.Pos]bool{}
	ast.Inspect(file, func(n ast.Node) bool {
		if call, ok := n.(*ast.CallExpr); ok {
			if prefix, ok := groupCallPrefix(call); ok {
				groupExprs[call.Lparen] = prefix
			}
		}
		return true
	})

	groupVars := map[string]string{} // var name -> prefix
	varUsed := map[string]bool{}
	bindGroupVar := func(lhs, rhs ast.Expr) {
		ident, ok := lhs.(*ast.Ident)
		if !ok {
			return
		}
		call, ok := unwrapGroupChain(rhs)
		if !ok {
			return
		}
		prefix, ok := groupCallPrefix(call)
		if !ok {
			return
		}
		groupVars[ident.Name] = prefix
		accounted[call.Lparen] = true
	}
	ast.Inspect(file, func(n ast.Node) bool {
		switch st := n.(type) {
		case *ast.AssignStmt:
			for i := range st.Rhs {
				if i < len(st.Lhs) {
					bindGroupVar(st.Lhs[i], st.Rhs[i])
				}
			}
		case *ast.ValueSpec:
			for i := range st.Values {
				if i < len(st.Names) {
					bindGroupVar(st.Names[i], st.Values[i])
				}
			}
		}
		return true
	})

	// isGroupArg reports whether an argument carries a router group, and its
	// prefix. Covers the inline call and the one-hop local variable — the
	// variable form being common enough that failing closed on it would be
	// constant friction.
	isGroupArg := func(arg ast.Expr) (string, bool) {
		if call, ok := unwrapGroupChain(arg); ok {
			if prefix, ok := groupCallPrefix(call); ok {
				accounted[call.Lparen] = true
				return prefix, true
			}
		}
		if ident, ok := arg.(*ast.Ident); ok {
			if prefix, ok := groupVars[ident.Name]; ok {
				varUsed[ident.Name] = true
				return prefix, true
			}
		}
		return "", false
	}

	// --- pass 2: classify every call ---
	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}

		// Call names, argument-agnostic (see mainWiring.CalledNames).
		if name, ok := selectorChainName(call.Fun); ok {
			w.CalledNames[name] = true
		}

		// A route method invoked on anything, with a literal path: a direct
		// registration. Receiver-agnostic on purpose — it catches the engine
		// (r.POST(...)) as well as a group, without needing to resolve which.
		if sel, ok := call.Fun.(*ast.SelectorExpr); ok && isRouteMethod(sel.Sel.Name) {
			if route, ok := directRouteFromCall(sel.Sel.Name, call); ok {
				w.DirectRoutes[route] = where(call.Lparen)
			}
			// The receiver is consumed as a router; if it was a group
			// expression or a tracked variable, that use is accounted.
			markReceiverAccounted(sel.X, accounted, groupVars, varUsed)
			return true
		}

		// Use/Group on a group: still group-shaped, keeps being tracked. The
		// resulting Group() call has its own accounting entry.
		if sel, ok := call.Fun.(*ast.SelectorExpr); ok && groupPassThroughMethods[sel.Sel.Name] {
			markReceiverAccounted(sel.X, accounted, groupVars, varUsed)
			return true
		}

		// Any other call receiving a router group is a mount.
		for _, arg := range call.Args {
			prefix, isGroup := isGroupArg(arg)
			if !isGroup {
				continue
			}
			name, named := selectorChainName(call.Fun)
			require.Truef(t, named,
				"%s: a call receives a router group (prefix %q) but this test cannot name "+
					"its target, so it cannot be checked against buildSurfaces or "+
					"mountsOutsideManifest.\n\n"+
					"Failing rather than skipping is deliberate: a mount this walker cannot "+
					"see is a whole subtree whose deletion nothing catches, which is the "+
					"defect this instrument exists to eliminate. Either name the registrar "+
					"conventionally (pkg.Func / pkg.New().Method / localVar.Method), or teach "+
					"selectorChainName this shape.",
				where(call.Lparen), prefix)
			w.Mounts[name] = groupMount{Registrar: name, Prefix: prefix, Where: where(call.Lparen)}
		}
		return true
	})

	// --- pass 3: fail closed on anything left unexplained ---
	for pos, prefix := range groupExprs {
		require.Truef(t, accounted[pos],
			"%s: a .Group(%q) value is created here and this test cannot tell what happens "+
				"to it.\n\n"+
				"Every router group in package main must be traceable to a mount, a direct "+
				"route registration, or a local variable this walker follows. An unexplained "+
				"group is a possible subtree outside the manifest — exactly the silent drop "+
				"this check exists to stop — so it fails instead of being skipped.",
			where(pos), prefix)
	}
	for name := range groupVars {
		require.Truef(t, varUsed[name],
			"%s holds a router group in package main but this test never saw it used. Either "+
				"it is dead (remove it) or it is consumed by a shape this walker does not "+
				"follow, which would hide whatever it mounts.",
			name)
	}
}

// markReceiverAccounted records that a group expression or tracked group
// variable was consumed as a router.
func markReceiverAccounted(recv ast.Expr, accounted map[token.Pos]bool, groupVars map[string]string, varUsed map[string]bool) {
	if call, ok := unwrapGroupChain(recv); ok {
		if _, ok := groupCallPrefix(call); ok {
			accounted[call.Lparen] = true
			return
		}
	}
	if ident, ok := recv.(*ast.Ident); ok {
		if _, ok := groupVars[ident.Name]; ok {
			varUsed[ident.Name] = true
		}
	}
}

// unwrapGroupChain peels Use() calls off an expression to reach the
// underlying Group() call, so `r.Group("/x").Use(mw)` resolves to the
// Group("/x") call.
func unwrapGroupChain(e ast.Expr) (*ast.CallExpr, bool) {
	call, ok := e.(*ast.CallExpr)
	if !ok {
		return nil, false
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return nil, false
	}
	if sel.Sel.Name == "Group" {
		return call, true
	}
	if sel.Sel.Name == "Use" {
		return unwrapGroupChain(sel.X)
	}
	return nil, false
}

// groupCallPrefix reports the prefix of a `*.Group(...)` call. ok is false
// when the call is not a Group call at all.
//
// A prefix this cannot read as a string returns "<unresolved>", which is
// still a group — failing safe, because an unreadable prefix must not turn
// into an exemption. platformadmin.MountPrefix is the one that lands here.
func groupCallPrefix(call *ast.CallExpr) (string, bool) {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "Group" {
		return "", false
	}
	if len(call.Args) == 0 {
		return "", true // Group() with no prefix — root
	}
	if lit, ok := stringLit(call.Args[0]); ok {
		return lit, true
	}
	return "<unresolved>", true
}

// stringLit returns the contents of a string literal expression.
func stringLit(e ast.Expr) (string, bool) {
	lit, ok := e.(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return "", false
	}
	// Strips the surrounding quotes of an interpreted or raw string literal.
	// A cutset is enough: only the delimiters can appear at either end of a
	// Go string literal token.
	return strings.Trim(lit.Value, "\"`"), true
}

// selectorChainName renders a call target as a dotted name, walking down
// through constructor calls and field selectors so a registrar gets a stable,
// checkable name rather than being silently exempt:
//
//	admin.RegisterAdmin                                      -> "admin.RegisterAdmin"
//	internalsvc.NewStoreActiveDomainHandler(db).Register     -> "internalsvc.NewStoreActiveDomainHandler.Register"
//	subscription.NewInternalHandler(x).WithPromo(y).Register -> "subscription.NewInternalHandler.WithPromo.Register"
//	someLocalVar.Register                                    -> "someLocalVar.Register"
//	holder.Inner.Reg                                         -> "holder.Inner.Reg"
//
// The last two are local variables and struct fields, not packages. The AST
// cannot resolve their types without type-checking, and the written name is
// stable enough to key an exemption on.
func selectorChainName(fn ast.Expr) (string, bool) {
	sel, ok := fn.(*ast.SelectorExpr)
	if !ok {
		return "", false
	}
	switch recv := sel.X.(type) {
	case *ast.Ident:
		return recv.Name + "." + sel.Sel.Name, true
	case *ast.SelectorExpr:
		inner, ok := selectorChainName(recv)
		if !ok {
			return "", false
		}
		return inner + "." + sel.Sel.Name, true
	case *ast.CallExpr:
		inner, ok := selectorChainName(recv.Fun)
		if !ok {
			return "", false
		}
		return inner + "." + sel.Sel.Name, true
	}
	return "", false
}

// requireReasons fails on an exemption whose reason is blank. A reason nobody
// checks is a comment: `"admin.RegisterAdmin": "   "` previously dropped 190
// routes at exit 0.
func requireReasons(t *testing.T, listName string, m map[string]string) {
	t.Helper()
	for key, reason := range m {
		require.NotEmptyf(t, strings.TrimSpace(reason),
			"%s[%q] has a blank reason. An exemption is a decision on the record; without "+
				"the reason it is just a hole with a name.", listName, key)
	}
}

// TestMainMountsEveryFrontendFacingSurface answers the one question gin
// cannot: whether package main actually mounts the registrars this package
// builds — and, in the other direction, whether every registrar and every
// direct route main mounts is accounted for.
//
// Parsing source is forbidden for "which routes does a subtree register" —
// gin answers that, above. It is the ONLY way to answer "does main mount this
// subtree", because all wiring lives inside func main() and cannot be called.
// Keeping the two questions apart is what makes this not a contradiction;
// wiring_test.go in cmd/marketplace-api does the same for platformadmin.Deps
// field parity.
//
// See analyseMainPackage for why the group analysis fails closed, and for the
// one hole that remains.
func TestMainMountsEveryFrontendFacingSurface(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	require.True(t, ok, "runtime.Caller(0) failed to report this test file's own path")
	mainDir := filepath.Join(filepath.Dir(thisFile), "..", "..", "cmd", "marketplace-api")

	requireReasons(t, "mountsOutsideManifest", mountsOutsideManifest)
	requireReasons(t, "directRoutesOutsideManifest", directRoutesOutsideManifest)

	w := analyseMainPackage(t, mainDir)
	require.Greaterf(t, len(w.Mounts), 10,
		"found only %d group mounts in package main — the AST walk is broken (it found 19 "+
			"when this test was written), and every assertion below would pass vacuously",
		len(w.Mounts))
	require.NotEmpty(t, w.CalledNames, "parsed no call names out of package main — the walk is broken")

	built := buildSurfaces(t)
	inManifest := map[string]string{} // registrar -> surface name
	declaredRoutes := map[string]bool{}
	for _, s := range built {
		inManifest[s.Registrar] = s.Name
		for _, r := range s.Routes {
			declaredRoutes[r] = true
		}
	}

	// Direction 1: everything buildSurfaces claims is actually mounted.
	for _, s := range built {
		_, mounted := w.Mounts[s.Registrar]
		require.Truef(t, mounted,
			"buildSurfaces declares surface %q from %s, but package main does not mount it on "+
				"any router group. Either the mount was removed — in which case the surface "+
				"must come out of the manifest, not stay in it — or the registrar was renamed.",
			s.Name, s.Registrar)
	}

	// Direction 2: everything mounted is accounted for. This is the direction
	// whose absence let a whole surface be dropped silently.
	for name, m := range w.Mounts {
		if _, ok := inManifest[name]; ok {
			continue
		}
		if _, ok := mountsOutsideManifest[name]; ok {
			continue
		}
		t.Errorf("%s: package main mounts %s on group %q, and it is neither in buildSurfaces "+
			"nor in mountsOutsideManifest.\n\n"+
			"If it serves a frontend, add it to buildSurfaces and regenerate the manifest "+
			"(%s) — a subtree outside the manifest is a subtree whose deletion nothing "+
			"catches. If it does not, add it to mountsOutsideManifest with the reason, so "+
			"the exclusion is a decision on the record rather than a gap.",
			m.Where, name, m.Prefix, routemanifest.UpdateCommand)
	}

	// Direction 2b: the same, for routes registered straight onto a router
	// with no registrar in between. Two of these are frontend-facing.
	for route, at := range w.DirectRoutes {
		if declaredRoutes[route] {
			continue
		}
		if _, ok := directRoutesOutsideManifest[route]; ok {
			continue
		}
		t.Errorf("%s: package main registers %q directly on a router, and route-manifest.json "+
			"does not declare it nor does directRoutesOutsideManifest exempt it.\n\n"+
			"A route mounted outside every registrar is invisible to the gin-tree "+
			"reconciliation, because no Register* function builds it. Declare it (if a "+
			"registrar can own it, move it there and regenerate: %s) or exempt it with a "+
			"reason.", at, route, routemanifest.UpdateCommand)
	}

	// Stale exemptions hide whatever replaced them.
	for name := range mountsOutsideManifest {
		_, mounted := w.Mounts[name]
		require.Truef(t, mounted,
			"mountsOutsideManifest exempts %s, which package main no longer mounts on any "+
				"router group. Remove the entry — an exemption for a mount that does not "+
				"exist hides whatever took its place.", name)
	}
	for route := range directRoutesOutsideManifest {
		_, present := w.DirectRoutes[route]
		require.Truef(t, present,
			"directRoutesOutsideManifest exempts %q, which package main no longer registers. "+
				"Remove the entry.", route)
	}

	// And no registrar may be in both lists, which would make the exemption
	// look like the reason it is out of the manifest when it is not.
	for name := range mountsOutsideManifest {
		surface, ok := inManifest[name]
		require.Falsef(t, ok,
			"%s is in BOTH buildSurfaces (as surface %q) and mountsOutsideManifest — one of "+
				"the two is wrong, and whichever it is, the exemption is misleading",
			name, surface)
	}

	// RegisterMobileStorefront is asserted on CALL NAMES, independent of the
	// group analysis. When this briefly read from the mount map instead,
	// wiring the registrar through a local variable made the assertion stop
	// firing — it only caught mounts that passed the inline-.Group() filter.
	// A call-name check has no such escape: the call cannot be made without
	// naming the function.
	require.Falsef(t, w.CalledNames["storefront.RegisterMobileStorefront"],
		"package main now calls storefront.RegisterMobileStorefront, so "+
			"/api/v1/mobile/storefront is a real surface. Add it to buildSurfaces and "+
			"regenerate the manifest (%s), or the whole mobile storefront subtree stays "+
			"outside this guard.",
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
