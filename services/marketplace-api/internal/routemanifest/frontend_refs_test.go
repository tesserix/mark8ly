package routemanifest_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/routemanifest"
)

// ---------------------------------------------------------------------------
// Why this file replaced an assertion that /internal is out of scope
//
// The original brief for #834 said no frontend references /internal, and
// asked for an assertion that no manifest entry begins with it. That claim
// was false, and the way it failed is the reason this file works the way it
// does: the check behind it grepped apps/admin/{lib,app} and
// apps/storefront/{lib,app}, but middleware.ts sits at the app ROOT, outside
// all four — so two of the three real callers were unreachable by
// construction, and the third was pushed out of a `head -5` window by
// comment-only hits.
//
// An assertion that the scope is correct cannot catch a scope that is wrong.
// So this asserts the opposite direction: every /internal path apps/* points
// at marketplace-api MUST be in the manifest. The references are DERIVED from
// source, never hand-listed, so a new frontend call to an uncovered internal
// route fails here instead of quietly widening the gap.
// ---------------------------------------------------------------------------

// appsDir is the frontend workspace root, resolved from this test file rather
// than the working directory. Walking from here — not from a hand-written
// list of subdirectories — is what makes app-root files like middleware.ts
// reachable.
func appsDir(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	require.True(t, ok, "runtime.Caller(0) failed to report this test file's own path")
	return filepath.Join(filepath.Dir(thisFile), "..", "..", "..", "..", "apps")
}

// skipDirs are build outputs and vendored trees. Everything else under apps/
// is scanned, at every depth.
var skipDirs = map[string]bool{
	"node_modules": true, ".next": true, "dist": true, "build": true,
	"coverage": true, ".turbo": true, "playwright-report": true, "test-results": true,
}

var sourceExts = map[string]bool{".ts": true, ".tsx": true, ".js": true, ".jsx": true, ".mjs": true}

// testFileSuffixes are excluded from the scan. A mocked path in a unit or e2e
// spec is not a live caller, and treating it as one gets the direction of
// #826 exactly backwards: the e2e spec that kept the suite green after the
// arbitrage-appeal endpoint was deleted did so by MOCKING the endpoint that
// had just gone. Counting that as evidence a route is in use would make this
// guard agree with the artifact that hid the bug.
var testFileSuffixes = []string{
	".test.ts", ".test.tsx", ".test.js", ".test.jsx", ".test.mjs",
	".spec.ts", ".spec.tsx", ".spec.js", ".spec.jsx", ".spec.mjs",
}

func isTestFile(name string) bool {
	lower := strings.ToLower(name)
	for _, suffix := range testFileSuffixes {
		if strings.HasSuffix(lower, suffix) {
			return true
		}
	}
	return false
}

// baseIdentDecl finds an identifier bound to marketplace-api's base URL, e.g.
//
//	const MARKETPLACE_API_URL =
//	  process.env.MARKETPLACE_API_URL ?? "http://localhost:8088";
//
// The declaration wraps across lines in most apps/* files, hence (?s) and the
// bounded gap. Matching the NAME is what makes this work across files: an
// identifier imported from a lib module keeps its name at the use site.
var baseIdentDecl = regexp.MustCompile(`(?s)(?:const|let|var)\s+([A-Za-z_$][\w$]*)\s*=[^;]{0,200}?process\.env\.(?:NEXT_PUBLIC_)?MARKETPLACE_API_URL`)

// paramSegment is what an interpolated segment becomes once normalised, so a
// written path can be compared against a gin route template.
const paramSegment = ":param"

// internalRef is one frontend call site pointing at marketplace-api /internal.
type internalRef struct {
	File   string // repo-relative
	Line   int
	Raw    string // the path exactly as written
	Path   string // normalised: `${...}` -> ":param", query string dropped
	Parsed bool   // false when an interpolation never closed — an extractor bug
}

// extractInternalRefs walks the whole of apps/ and returns every reference to
// a marketplace-api /internal path.
//
// Two passes. The first collects the identifiers bound to marketplace-api's
// base URL anywhere under apps/; the second finds `${THAT_IDENT}/internal/...`
// anywhere under apps/. Splitting them is what lets a module define the base
// and a different file use it.
//
// Only marketplace-api bases count. apps/* also calls PLATFORM_API_URL
// /internal routes — a different service, whose routes this manifest does not
// and should not describe.
func extractInternalRefs(t *testing.T, root string) (refs []internalRef, filesScanned int, bases []string) {
	t.Helper()

	type scanned struct {
		rel   string
		lines []string
	}
	var files []scanned
	baseSet := map[string]bool{
		// The env var names themselves, for a direct `process.env.X` use.
		"MARKETPLACE_API_URL":             true,
		"NEXT_PUBLIC_MARKETPLACE_API_URL": true,
	}

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if skipDirs[d.Name()] {
				return fs.SkipDir
			}
			return nil
		}
		if !sourceExts[strings.ToLower(filepath.Ext(d.Name()))] || isTestFile(d.Name()) {
			return nil
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(filepath.Dir(root), path)
		if err != nil {
			rel = path
		}
		for _, m := range baseIdentDecl.FindAllStringSubmatch(string(raw), -1) {
			baseSet[m[1]] = true
		}
		files = append(files, scanned{rel: rel, lines: strings.Split(string(raw), "\n")})
		return nil
	})
	require.NoError(t, err, "walking %s", root)

	for b := range baseSet {
		bases = append(bases, b)
	}
	sort.Strings(bases)

	for _, f := range files {
		for i, line := range f.lines {
			for _, base := range bases {
				// The search shape, and its one real blind spot.
				//
				// This finds a base URL interpolated IMMEDIATELY before the
				// path — the shape every apps/* caller uses today. It does
				// NOT find a path passed through a wrapper, e.g.
				//
				//	marketplaceInternalFetch(`/internal/store-active-domain/${slug}`)
				//
				// where the base is applied inside the helper. Resolving
				// that needs call-graph analysis of TypeScript, which is a
				// different tool from a Go test. Nothing in apps/* does it
				// today; the day a helper like that appears, every caller
				// behind it becomes invisible here, and this comment is the
				// warning.
				needle := "${" + base + "}/internal"
				idx := strings.Index(line, needle)
				if idx < 0 {
					continue
				}
				raw, norm, ok := scanRefPath(line[idx+len(needle)-len("/internal"):])
				refs = append(refs, internalRef{
					File: f.rel, Line: i + 1, Raw: raw,
					Path: trimQuery(norm), Parsed: ok,
				})
			}
		}
	}
	sort.Slice(refs, func(i, j int) bool {
		if refs[i].File != refs[j].File {
			return refs[i].File < refs[j].File
		}
		return refs[i].Line < refs[j].Line
	})
	return refs, len(files), bases
}

// scanRefPath reads the URL path out of the remainder of a template literal,
// returning it both as written and normalised for comparison.
//
// It tracks `${...}` nesting instead of stopping at the first terminator
// character, and that is a correction, not caution. The first version split
// on a fixed character set that included ")", so
// `/internal/store-active-domain/${encodeURIComponent(slug)}` truncated at
// the ")" inside the interpolation to
// `/internal/store-active-domain/${encodeURIComponent(slug`. The test still
// PASSED, because gin's own `:slug` segment matches anything — so the
// mangled segment compared equal by accident. A path with a segment AFTER a
// function-call interpolation would have lost every trailing segment and
// failed for a reason that had nothing to do with the manifest.
//
// ok is false when an interpolation never closes, so a path the extractor
// cannot read is reported as an extractor bug rather than silently compared
// in a mangled form.
func scanRefPath(rest string) (raw, normalised string, ok bool) {
	var rawB, normB strings.Builder
	for i := 0; i < len(rest); {
		if strings.HasPrefix(rest[i:], "${") {
			depth, j := 0, i
			for ; j < len(rest); j++ {
				switch rest[j] {
				case '{':
					depth++
				case '}':
					depth--
					if depth == 0 {
						j++
						goto closed
					}
				}
			}
			return rawB.String(), normB.String(), false // unterminated
		closed:
			rawB.WriteString(rest[i:j])
			normB.WriteString(paramSegment)
			i = j
			continue
		}
		if strings.ContainsRune("`\"'\n\r\t ,)", rune(rest[i])) {
			break
		}
		rawB.WriteByte(rest[i])
		normB.WriteByte(rest[i])
		i++
	}
	return rawB.String(), normB.String(), true
}

// trimQuery drops a query string or fragment and any trailing slash, so
// `/internal/tenants/:param/me?uid=:param` compares as a path.
func trimQuery(p string) string {
	if i := strings.IndexAny(p, "?#"); i >= 0 {
		p = p[:i]
	}
	return strings.TrimSuffix(p, "/")
}

// matchOutcome is why a reference path did or did not resolve to a gin route
// template. The middle case is the point of the type.
type matchOutcome int

const (
	// matchNone: the paths genuinely describe different routes.
	matchNone matchOutcome = iota
	// matchExact: every segment lines up, with gin's own :name segments
	// absorbing whatever the frontend interpolates into them.
	matchExact
	// matchUnresolvable: the reference interpolates a segment where the
	// route template has a LITERAL, so no static comparison can say whether
	// this reference reaches this route. It is not a match, and it is
	// reported differently, because "we cannot tell" is not "it is fine".
	matchUnresolvable
)

// matchesTemplate reports whether a normalised reference path is served by a
// gin route template.
//
// Only gin's OWN :name is a wildcard, and that asymmetry is the whole
// correctness of this function. The first version also treated the
// REFERENCE's ":param" as a wildcard against a literal template segment, so
// any interpolated non-final segment matched any declared route of the same
// length. A review sabotage added
// fetch(`${MARKETPLACE_API_URL}/internal/${k}/${id}`) — a route that exists
// nowhere in this service — and the guard PASSED: it normalised to
// /internal/:param/:param and matched /internal/store-active-domain/:slug
// because segment 2's literal was compared against the reference's wildcard.
// That is the unsafe direction. A base-URL misattribution errs toward a
// false failure; this errs toward blessing a call to a route that does not
// exist, which is precisely what this instrument is for.
//
// Multi-segment catch-alls (gin's *name) are rejected up front by
// requireNoCatchAllRoutes rather than half-handled here — the equal-segment
// -count rule below is wrong for them, and none exists in the manifest.
func matchesTemplate(ref, template string) matchOutcome {
	r := strings.Split(strings.TrimPrefix(ref, "/"), "/")
	g := strings.Split(strings.TrimPrefix(template, "/"), "/")
	if len(r) != len(g) {
		return matchNone
	}

	outcome := matchExact
	for i := range r {
		switch {
		case strings.HasPrefix(g[i], ":"):
			// gin's own param absorbs any single segment, interpolated or
			// literal. This is the ONLY wildcard.
			continue
		case r[i] == paramSegment:
			// Interpolated against a literal. Cannot be decided statically;
			// keep scanning so a later mismatching literal still downgrades
			// this to a plain non-match rather than a reported ambiguity.
			outcome = matchUnresolvable
		case r[i] != g[i]:
			return matchNone
		}
	}
	return outcome
}

// requireNoCatchAllRoutes fails if any declared path uses gin's multi-segment
// catch-all (*name). matchesTemplate compares segment counts, which is
// simply wrong for a segment that matches several — /internal/*rest would
// serve /internal/a/b/c and compare equal to nothing.
//
// There is no such route in the manifest today, so this is a deliberate
// refusal rather than an implementation: made explicit here instead of left
// as a silently wrong comparison that would surface as a confusing
// not-declared failure years from now.
func requireNoCatchAllRoutes(t *testing.T, declared []string) {
	t.Helper()
	for _, path := range declared {
		for _, seg := range strings.Split(path, "/") {
			require.Falsef(t, strings.HasPrefix(seg, "*"),
				"route-manifest.json declares %q, which uses gin's multi-segment catch-all "+
					"(%s). matchesTemplate compares segment counts and cannot reason about "+
					"it — teach it to, or exclude the route deliberately. Do not leave the "+
					"comparison silently wrong.", path, seg)
		}
	}
}

// TestFrontendInternalRoutesAreDeclared is the replacement for the assertion
// that no manifest entry begins /internal. That one asserted the scope was
// right; this one tests whether it is.
//
// Every /internal path apps/* calls on marketplace-api must be declared in
// route-manifest.json. A new frontend call to an internal route this manifest
// does not cover fails HERE, naming the file and line, rather than silently
// widening the gap the original brief opened.
//
// Matching is on PATH, ignoring method: the question is whether the route
// exists at all. Deleting a route removes every method on it, so the
// deletion this guards against still fails.
func TestFrontendInternalRoutesAreDeclared(t *testing.T) {
	root := appsDir(t)
	refs, filesScanned, bases := extractInternalRefs(t, root)

	// Vacuous-pass guards. A walk that silently reaches nothing — a moved
	// apps/ directory, an over-broad skipDirs entry, a renamed base
	// identifier — would otherwise report zero references and pass, which is
	// precisely how the original scope claim came to be believed.
	require.Greaterf(t, filesScanned, 100,
		"scanned only %d source files under %s — the walk is not reaching apps/, "+
			"and this test would pass while checking nothing", filesScanned, root)
	require.Greaterf(t, len(bases), 2,
		"found only the two built-in base identifiers %v — baseIdentDecl matched no "+
			"declaration in any apps/* file, so no reference can be found and this "+
			"test would pass vacuously", bases)
	require.NotEmpty(t, refs,
		"found no marketplace-api /internal references in apps/ at all. Three existed "+
			"when this test was written (apps/admin/middleware.ts, "+
			"apps/admin/lib/auth/cross-domain-handoff.ts, apps/storefront/middleware.ts). "+
			"If they were genuinely all removed, delete the internal surface from "+
			"buildSurfaces too; otherwise the extractor has stopped matching.")

	// The specific regression guard for how the original claim failed: a
	// reference in a file at an APP ROOT, outside lib/ and app/. Both
	// middleware.ts callers live there. If the walk ever stops covering app
	// roots, this fails instead of quietly shrinking the reference set.
	foundAtAppRoot := false
	for _, r := range refs {
		// "apps/<app>/<file>" — three segments means directly in the app root.
		if len(strings.Split(filepath.ToSlash(r.File), "/")) == 3 {
			foundAtAppRoot = true
			break
		}
	}
	require.Truef(t, foundAtAppRoot,
		"no reference was found in a file at an app ROOT (apps/<app>/<file>), only in "+
			"subdirectories. That is exactly the shape of the search bug this test "+
			"replaced — middleware.ts lives at the app root, outside lib/ and app/. "+
			"References found: %v", refs)

	doc, err := routemanifest.Load(manifestPath(t))
	require.NoError(t, err)

	var declared []string
	for _, s := range doc.Surfaces {
		for _, route := range s.Routes {
			_, path, found := strings.Cut(route, " ")
			require.Truef(t, found, "surface %q has malformed entry %q — expected \"METHOD PATH\"", s.Name, route)
			declared = append(declared, path)
		}
	}

	requireNoCatchAllRoutes(t, declared)

	for _, ref := range refs {
		// The extractor's own correctness, asserted before its output is
		// trusted. A half-read path compares against gin templates whose
		// params match anything, so a truncated segment can pass by accident
		// — which is exactly what the first version of scanRefPath did.
		require.Truef(t, ref.Parsed,
			"could not read the path at %s:%d: an interpolation never closes in %q. "+
				"Fix scanRefPath — do not adjust the manifest to suit a path the "+
				"extractor cannot read.", ref.File, ref.Line, ref.Raw)
		require.NotContainsf(t, ref.Path, "$",
			"normalised path %q from %s:%d still contains an interpolation, so it was "+
				"only partly read. Fix scanRefPath, not the manifest.",
			ref.Path, ref.File, ref.Line)
		require.NotContainsf(t, ref.Path, "(",
			"normalised path %q from %s:%d contains a bare \"(\", which means an "+
				"interpolation was split across it. Fix scanRefPath, not the manifest.",
			ref.Path, ref.File, ref.Line)

		matched := false
		var ambiguous []string
		for _, path := range declared {
			switch matchesTemplate(ref.Path, path) {
			case matchExact:
				matched = true
			case matchUnresolvable:
				ambiguous = append(ambiguous, path)
			}
			if matched {
				break
			}
		}

		// Reported separately from a plain non-match, because the fix is
		// different: an ambiguous reference is not a missing route, it is a
		// call site this search shape cannot resolve, and blessing it is the
		// one thing that must not happen.
		require.Falsef(t, !matched && len(ambiguous) > 0,
			"%s:%d calls marketplace-api %q (normalised %q). It cannot be resolved "+
				"statically: it interpolates a segment where the manifest has a literal, so "+
				"it is UNDECIDABLE whether it reaches %v.\n\n"+
				"This is not a pass. Either give the call site a literal path segment, or "+
				"teach this test how to resolve it — do not widen matchesTemplate to treat "+
				"the reference's own interpolation as a wildcard, which is what previously "+
				"let a call to a nonexistent route pass.",
			ref.File, ref.Line, ref.Raw, ref.Path, ambiguous)

		require.Truef(t, matched,
			"%s:%d calls marketplace-api %q (normalised %q) and route-manifest.json does "+
				"not declare it.\n\n"+
				"A frontend caller of a route this manifest does not cover is a route whose "+
				"deletion nothing catches — #826's bug shape, and the one it was actually "+
				"observed in. Either add the registrar that mounts it to buildSurfaces and "+
				"regenerate (%s), or remove the frontend caller.",
			ref.File, ref.Line, ref.Raw, ref.Path, routemanifest.UpdateCommand)
	}

	t.Logf("checked %d marketplace-api /internal reference(s) across %d source files under apps/", len(refs), filesScanned)
}
