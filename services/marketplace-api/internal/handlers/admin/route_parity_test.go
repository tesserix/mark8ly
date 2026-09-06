package admin

import (
	"log/slog"
	"reflect"
	"sort"
	"strings"
	"testing"
	"unsafe"

	"github.com/gin-gonic/gin"

	"github.com/mark8ly/marketplace-api/internal/auth"
	"github.com/mark8ly/marketplace-api/internal/authz"
)

// ---------------------------------------------------------------------------
// Why this file exists
//
// The admin API has TWO route tables:
//
//	routes.go        → /api/v1/admin/stores/:storeId/...          (web admin)
//	mobile_routes.go → /api/v1/mobile/admin/stores/:storeId/...   (mobile app)
//
// They share handlers, DTOs and authz roles; only the auth chain differs
// (header-trust vs GIP bearer). A route added to one and not the other is a
// gin-level 404 on the other surface — the handler never runs, so no amount
// of handler-level testing can see it. The mobile client renders that 404 as
// "It no longer exists. Pull down to refresh.", which is actively misleading
// for a route that was simply never registered.
//
// This has now happened twice:
//   - gift-card enable/disable  (fixed in bb54b5c0)
//   - shipment cancel           (fixed alongside this guard)
//
// Both shipped green, both were caught by accident.
//
// This test enumerates the REAL gin trees — it never parses source and never
// keeps a hand-copied list of routes, because that anti-pattern already bit
// this codebase once (TestCodeStatus_CoversAllCodes drifted silently until it
// was rewritten to enumerate from an authoritative source). What IS hand-kept
// here is the list of deliberate EXCEPTIONS, and every exception must earn its
// place: an unused one fails the test just as loudly as a missing route.
// ---------------------------------------------------------------------------

// webOnlySubtrees exempts an entire path prefix from parity. Legitimate only
// for capabilities mobile deliberately does not implement AT ALL. The test
// enforces that: if the mobile table ever registers anything under one of
// these prefixes, the subtree exemption is rejected and must be replaced with
// exact webOnlyRoutes entries, so the half-mirrored group is scrutinised
// route-by-route (that half-mirrored state is exactly how both bugs happened).
var webOnlySubtrees = map[string]string{
	"/break-glass":                              "emergency recovery login — deliberately web-only; the flow is a browser link from a support email",
	"/tenants/:tenantId/sso":                    "enterprise SSO config (Pro-gated, tenant-wide) — configured once from a desktop, no mobile UI planned",
	"/stores/:storeId/abandoned-carts":          "no mobile screen — cart recovery is a desktop marketing workflow",
	"/stores/:storeId/api-keys":                 "developer API key management — secrets are shown once and copy/pasted into a terminal; deliberately desktop-only",
	"/stores/:storeId/app-credentials":          "white-label app signing certificate upload — file uploads from a laptop only",
	"/stores/:storeId/arbitrage-appeal":         "geo-pricing appeal — a long-form billing dispute form, web-only by design",
	"/stores/:storeId/billing":                  "trial card-add — payment card entry stays on web (PCI surface)",
	"/stores/:storeId/csv-imports":              "bulk CSV import — file picker + error-report download, web-only by design",
	"/stores/:storeId/dashboard/metrics":        "analytics tabs — no mobile analytics screen yet; the mobile Home uses GET /dashboard only",
	"/stores/:storeId/dashboard/setup-progress": "onboarding checklist incl. SSE/WebSocket streams — mobile has no setup-checklist screen",
	"/stores/:storeId/domains":                  "custom domain + DNS setup — a desktop task, web-only by design",
	"/stores/:storeId/migration-fast-path":      "platform-migration submission form — web-only by design",
	"/stores/:storeId/pages":                    "CMS page editor (rich text) — no mobile editor",
	"/stores/:storeId/returns":                  "returns/RMA workflow — not yet built on mobile; see also POST /orders/:id/returns below",
	"/stores/:storeId/settings":                 "payment/shipping/tax provider credentials — secret entry stays on web",
	"/stores/:storeId/subscription":             "plan, invoices, checkout, promo, cancel — all billing is web-only (app-store policy + PCI surface)",
	"/stores/:storeId/tax":                      "tax-ID submission + US/CA attestation — legal attestation flow, web-only by design",
	"/stores/:storeId/warehouses":               "warehouse CRUD (#177 PR 5b) — address forms + drag-to-reorder, a desktop settings task; mobile ships no warehouse screen. If mobile ever wants a read-only list, replace this subtree with exact webOnlyRoutes entries.",
	"/stores/:storeId/webhooks":                 "outbound webhook subscriptions (#562) — URL registration, secrets and delivery logs are a developer-integration task; no mobile UI planned.",
}

// webOnlyRoutes exempts a single route INSIDE a group mobile does mirror.
// These are the dangerous ones — a sibling route landing here later is how
// gift cards and shipment cancel broke — so each is listed individually and
// must be re-justified rather than covered by a blanket prefix.
var webOnlyRoutes = map[string]string{
	"GET /account":                    "mobile reads the profile from the GIP token; no profile screen",
	"PATCH /account":                  "no mobile profile editor",
	"GET /account/sessions":           "session management is a desktop security screen",
	"DELETE /account/sessions/:id":    "session management is a desktop security screen",
	"POST /account/avatar/upload-url": "avatar upload is a signed-URL browser flow",
	"POST /account/mfa/enable":        "MFA enrolment (QR code) is a desktop flow",
	"POST /account/mfa/verify":        "MFA enrolment (QR code) is a desktop flow",
	"POST /account/mfa/disable":       "MFA enrolment (QR code) is a desktop flow",

	"GET /stores/:storeId/audit-logs/export": "CSV download stream — no mobile file-save target",

	"POST /stores/:storeId/branding/upload-url": "logo upload is a signed-URL browser flow; mobile edits text basics only",

	"PATCH /stores/:storeId/categories/:id":  "mobile exposes the category picker + quick-create only; renames/deletes stay on web",
	"DELETE /stores/:storeId/categories/:id": "mobile exposes the category picker + quick-create only; renames/deletes stay on web",

	"PATCH /stores/:storeId/customers/:id/tags":  "mobile customer detail is read-only apart from block/unblock",
	"PATCH /stores/:storeId/customers/:id/notes": "mobile customer detail is read-only apart from block/unblock",

	"GET /stores/:storeId/notifications/unread-count": "mobile derives the bell badge from the list response it already holds",
	"PATCH /stores/:storeId/notifications/:id/read":   "mobile marks the whole feed read on open (PATCH /notifications/read-all); no per-item read",

	"POST /stores/:storeId/orders":             "manual/phone order creation — a back-office desktop task",
	"POST /stores/:storeId/orders/:id/returns": "returns workflow not yet built on mobile (see the /returns subtree above)",

	"DELETE /stores/:storeId/products/:id":                     "destructive and owner-only; deliberately not armed on a phone",
	"POST /stores/:storeId/products/:id/copy":                  "duplicate-product is a desktop catalogue-authoring action",
	"POST /stores/:storeId/products/bulk":                      "multi-select bulk edit — no mobile multi-select UI",
	"GET /stores/:storeId/products/export.csv":                 "CSV download stream — no mobile file-save target",
	"POST /stores/:storeId/products/:id/media/:mediaId/recrop": "crop editor is a desktop-only canvas UI",
}

// mobileOnlySubtrees / mobileOnlyRoutes: the mirror image. Same rules.
var mobileOnlySubtrees = map[string]string{
	"/platform-support":            "merchant→Tesserix support chat, bridged to otto. Web admin reaches the same otto surface through its own PlatformChat component, not through this service",
	"/stores/:storeId/push-tokens": "APNs/FCM device token registration — meaningless on web",
	"/stores/:storeId/team":        "staff/invite management proxied to platform-api; web admin calls platform-api directly rather than through this service",
}

var mobileOnlyRoutes = map[string]string{
	"POST /auth/otp/verify": "mobile email-OTP completion (#686): the second half of obtaining a bearer token, so it is unauthenticated like /auth/login above. The web admin completes the same challenge at auth-bff's cookie-based /auth/otp/verify, which its Next.js server calls directly with the internal-auth secret a device cannot hold",
	"POST /auth/login":      "mobile sign-in (#686): the app posts to marketplace-api because auth-bff is internet-reachable and its login route's only protection is the X-Internal-Auth secret, which a device cannot hold. The web admin has no equivalent — its Next.js server holds that secret and calls auth-bff directly. This is also the only mobile route mounted with NO bearer auth, since it is what produces a bearer token",
	"POST /auth/idp/start":  "mobile \"Continue with Google\" (#686 item 1), first leg: opens a Zitadel IDP intent and returns the authUrl. Unauthenticated for the same reason as /auth/login — it is how a client obtains a bearer token. The web admin has no equivalent: its Next.js server holds the internal-auth secret and calls auth-bff's /auth/zitadel/idp/start directly",
	"POST /auth/idp/finish": "mobile \"Continue with Google\" (#686 item 1), second leg: exchanges the intent id/token for tokens, resolving the tenant by the verified email in between. Same unauthenticated rationale as /auth/idp/start; the web equivalent is apps/admin/app/auth/idp/finish/route.ts, which is a Next.js route, not a route on this service",
	"GET /me/tenants":       "Zitadel tenant discovery (#686): mobile is the only surface that must ask the API which tenants it belongs to, because a Zitadel token carries no tenant claim and every other mobile route is tenant-gated. The web admin resolves the same thing server-side in its own BFF (resolveWorkspaceTenant) by calling platform-api directly, so mirroring it here would add an unused route. It is also the one mobile route deliberately mounted OUTSIDE requireTenant — see MobileMyTenantsHandler",
}

// ---------------------------------------------------------------------------

const (
	webPrefix    = "/api/v1/admin"
	mobilePrefix = "/api/v1/mobile/admin"
)

// TestAdminRouteParity is the guard. It compares the two live gin trees and
// fails when they drift in a way nobody signed off on.
func TestAdminRouteParity(t *testing.T) {
	web, mobile := buildParityRouters(t)

	webOnly := difference(web, mobile)
	mobileOnly := difference(mobile, web)

	checkSurface(t, surfaceCheck{
		missingFrom:  "mobile_routes.go (the phone gets a gin 404)",
		presentIn:    "routes.go",
		addTo:        "mobile_routes.go, inside RegisterAdminMobile",
		orphan:       webOnly,
		subtrees:     webOnlySubtrees,
		exact:        webOnlyRoutes,
		otherSurface: mobile,
		otherName:    "mobile",
		listVar:      "webOnlySubtrees / webOnlyRoutes",
	})

	checkSurface(t, surfaceCheck{
		missingFrom:  "routes.go (the web admin gets a gin 404)",
		presentIn:    "mobile_routes.go",
		addTo:        "routes.go, inside RegisterAdmin",
		orphan:       mobileOnly,
		subtrees:     mobileOnlySubtrees,
		exact:        mobileOnlyRoutes,
		otherSurface: web,
		otherName:    "web",
		listVar:      "mobileOnlySubtrees / mobileOnlyRoutes",
	})
}

type surfaceCheck struct {
	missingFrom  string
	presentIn    string
	addTo        string
	orphan       []string          // routes on one surface with no counterpart
	subtrees     map[string]string // prefix → reason
	exact        map[string]string // "METHOD /path" → reason
	otherSurface map[string]bool
	otherName    string
	listVar      string
}

func checkSurface(t *testing.T, c surfaceCheck) {
	t.Helper()

	usedSubtree := map[string]bool{}
	usedExact := map[string]bool{}

	for _, route := range c.orphan {
		if _, ok := c.exact[route]; ok {
			usedExact[route] = true
			continue
		}
		if prefix, ok := matchSubtree(c.subtrees, route); ok {
			usedSubtree[prefix] = true
			continue
		}
		t.Errorf(`route parity: %q is registered in %s but MISSING from %s.

  Pick one:
    (a) mirror it — register the same route in %s, reusing the same handler
        and the same authz role; or
    (b) declare it deliberate — add it to %s in route_parity_test.go with a
        one-line reason saying why this surface does not need it.

  Do NOT leave it as-is: a route on only one table is a silent 404 on the
  other, and the caller sees a misleading "no longer exists" error.`,
			route, c.presentIn, c.missingFrom, c.addTo, c.listVar)
	}

	// A subtree exemption is only honest while the other surface implements
	// NOTHING under it. Once it mirrors part of the group, the group is
	// half-built — precisely the state that produced the gift-card and
	// shipment-cancel 404s — so force it into exact, per-route decisions.
	for prefix := range c.subtrees {
		if !usedSubtree[prefix] {
			t.Errorf(`route parity: subtree exemption %q in %s no longer matches any route.
  Delete the entry — a stale exemption hides the next real gap.`, prefix, c.listVar)
			continue
		}
		if hit, ok := anyUnder(c.otherSurface, prefix); ok {
			t.Errorf(`route parity: subtree exemption %q claims the %s surface implements none of it, but %q is registered there.

  The group is now half-mirrored. Replace the subtree entry with one exact
  entry per still-unmirrored route in %s, so each omission is reviewed.`,
				prefix, c.otherName, hit, c.listVar)
		}
	}

	for route := range c.exact {
		if !usedExact[route] {
			t.Errorf(`route parity: exemption %q in %s no longer matches an unmirrored route.
  Either it was mirrored (delete the entry) or the path changed (update it).
  A stale exemption hides the next real gap.`, route, c.listVar)
		}
	}
}

func matchSubtree(subtrees map[string]string, route string) (string, bool) {
	path := routePath(route)
	for prefix := range subtrees {
		if underPrefix(path, prefix) {
			return prefix, true
		}
	}
	return "", false
}

func anyUnder(routes map[string]bool, prefix string) (string, bool) {
	var hits []string
	for route := range routes {
		if underPrefix(routePath(route), prefix) {
			hits = append(hits, route)
		}
	}
	if len(hits) == 0 {
		return "", false
	}
	sort.Strings(hits)
	return hits[0], true
}

// underPrefix matches on segment boundaries so "/dashboard/metrics" does not
// swallow "/dashboard" and "/team" does not swallow "/teams".
func underPrefix(path, prefix string) bool {
	return path == prefix || strings.HasPrefix(path, prefix+"/")
}

func routePath(route string) string {
	_, path, _ := strings.Cut(route, " ")
	return path
}

func difference(a, b map[string]bool) []string {
	var out []string
	for route := range a {
		if !b[route] {
			out = append(out, route)
		}
	}
	sort.Strings(out)
	return out
}

// buildParityRouters registers both tables and returns their route sets keyed
// by "METHOD /path", with the surface prefix stripped so the two are directly
// comparable.
func buildParityRouters(t *testing.T) (web, mobile map[string]bool) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	var webDeps Deps
	fillHandlers(reflect.ValueOf(&webDeps).Elem(), 0)
	webDeps.AuthzMiddleware = authz.NewMiddleware(nil, nil)
	assertNoNilDeps(t, reflect.ValueOf(webDeps), "Deps")

	webEngine := gin.New()
	RegisterAdmin(webEngine.Group("/api/v1"), webDeps)

	var mobileDeps MobileDeps
	fillHandlers(reflect.ValueOf(&mobileDeps).Elem(), 0)
	mobileDeps.AuthzMiddleware = authz.NewMiddleware(nil, nil)
	// Interface field — reflection cannot invent an implementation, and a nil
	// verifier makes RegisterAdminMobile return without registering anything.
	mobileDeps.TokenVerifier = &auth.FakeVerifier{}
	// TenantMembershipChecker/-Logger back auth.TenantFromRequest (#524
	// phase 4) — an interface and a pointer, so reflection cannot invent
	// them either. Registration only takes method values here (nothing is
	// invoked), so authz.NewFakeClient with no grants is enough.
	mobileDeps.TenantMembershipChecker = authz.NewFakeClient()
	mobileDeps.TenantMembershipLogger = slog.Default()
	assertNoNilDeps(t, reflect.ValueOf(mobileDeps), "MobileDeps")

	mobileEngine := gin.New()
	RegisterAdminMobile(mobileEngine.Group("/api/v1"), mobileDeps)

	// /api/v1/internal/... is a cluster-internal cron surface on the admin
	// engine, not part of the merchant admin API, so it is out of scope here.
	web = collect(webEngine, webPrefix)
	mobile = collect(mobileEngine, mobilePrefix)

	if len(web) == 0 || len(mobile) == 0 {
		t.Fatalf("route parity harness registered nothing (web=%d mobile=%d) — the guard would pass vacuously", len(web), len(mobile))
	}
	return web, mobile
}

func collect(engine *gin.Engine, prefix string) map[string]bool {
	out := make(map[string]bool)
	for _, r := range engine.Routes() {
		if !strings.HasPrefix(r.Path, prefix) {
			continue
		}
		out[r.Method+" "+strings.TrimPrefix(r.Path, prefix)] = true
	}
	return out
}

// fillHandlers populates every nil handler pointer and gin.HandlerFunc on a
// Deps-shaped struct so that no `if deps.XHandler != nil` guard silently hides
// a route from this test. It recurses into the handlers themselves — including
// unexported fields — because some registrars gate on inner state too (e.g.
// RegisterAPIKeys returns early when APIKeysHandler.resolver is nil), and a
// half-populated handler would make the guard blind to four real routes.
//
// Nothing is invoked here: registration only takes method values, so zero
// structs are sufficient and no service, DB or FGA client is needed.
func fillHandlers(v reflect.Value, depth int) {
	if depth > 3 || v.Kind() != reflect.Struct {
		return
	}
	for i := 0; i < v.NumField(); i++ {
		f := v.Field(i)
		if !f.CanAddr() {
			continue
		}
		// Bypasses the unexported-field write barrier; safe because the value
		// is addressable and we only ever store freshly allocated zero values.
		w := reflect.NewAt(f.Type(), unsafe.Pointer(f.UnsafeAddr())).Elem()
		switch f.Kind() {
		case reflect.Ptr:
			if f.Type().Elem().Kind() == reflect.Struct && w.IsNil() {
				allocated := reflect.New(f.Type().Elem())
				w.Set(allocated)
				fillHandlers(allocated.Elem(), depth+1)
			}
		case reflect.Struct:
			fillHandlers(w, depth+1)
		case reflect.Func:
			if f.Type() == reflect.TypeOf(gin.HandlerFunc(nil)) && w.IsNil() {
				w.Set(reflect.ValueOf(gin.HandlerFunc(func(c *gin.Context) { c.Next() })))
			}
		}
	}
}

// assertNoNilDeps fails loudly if a top-level dependency is still nil after
// filling. Reflection cannot fill interface fields, so a future
// `SomeHandler SomeInterface` on Deps would otherwise unregister its routes
// and quietly shrink what this guard can see.
func assertNoNilDeps(t *testing.T, v reflect.Value, name string) {
	t.Helper()
	for i := 0; i < v.NumField(); i++ {
		f := v.Field(i)
		if f.Kind() == reflect.Struct {
			assertNoNilDeps(t, f, name+"."+v.Type().Field(i).Name)
			continue
		}
		switch f.Kind() {
		case reflect.Ptr, reflect.Interface, reflect.Func:
			if f.IsNil() {
				t.Fatalf("route parity harness: %s.%s is nil, so any route it gates is invisible to this test — populate it in buildParityRouters",
					name, v.Type().Field(i).Name)
			}
		}
	}
}
