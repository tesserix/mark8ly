package routemanifest_test

import (
	"fmt"
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

// sourceExts are the frontend source extensions scanned. .mts/.cts/.cjs were
// missing until round 5 and are included now: nothing in apps/* uses them
// today, but an omitted extension is a directory this scanner silently does
// not cover.
var sourceExts = map[string]bool{
	".ts": true, ".tsx": true, ".mts": true, ".cts": true,
	".js": true, ".jsx": true, ".mjs": true, ".cjs": true,
}

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
func extractInternalRefs(t *testing.T, root string) (refs []internalRef, filesScanned int, bases, unparsed []string) {
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
		// Comments are stripped PER PHYSICAL LINE, before joining, and the
		// order matters in both directions. Stripping the JOINED text instead
		// would let a comment line ending in "+" fold in front of a real
		// reference and delete it — trading one blind spot for a worse one.
		// Stripping per line also fixes the same-line case
		// (`/* eslint-disable */ return fetch(...)`) that a per-physical-line
		// PREFIX heuristic could never fix.
		lines := strings.Split(string(raw), "\n")
		for i := range lines {
			lines[i] = stripComments(lines[i])
		}
		files = append(files, scanned{rel: rel, lines: lines})
		return nil
	})
	require.NoError(t, err, "walking %s", root)

	for b := range baseSet {
		bases = append(bases, b)
	}
	sort.Strings(bases)

	for _, f := range files {
		for _, ll := range joinLogicalLines(f.lines) {
			line := ll.Text
			found := 0
			unreadableOperand := false

			for _, base := range bases {
				// SHAPE 1 — template literal: `${BASE}/internal/...`
				//
				// EVERY occurrence on the logical line, not just the first.
				// Taking only the first was a live hole: an array of two
				// URLs whose first entry was a real route swallowed a second
				// entry pointing at a route that does not exist, and the
				// residual backstop stayed quiet because something had
				// matched. It was order-dependent, which is the worst kind
				// of green.
				needle := "${" + base + "}/internal"
				for from := 0; from <= len(line); {
					rel := strings.Index(line[from:], needle)
					if rel < 0 {
						break
					}
					idx := from + rel
					raw, norm, ok := scanRefPath(line[idx+len(needle)-len("/internal"):])
					refs = append(refs, internalRef{
						File: f.rel, Line: physicalLineOf(ll, raw, needle), Raw: raw,
						Path: trimQuery(norm), Parsed: ok,
					})
					found++
					from = idx + len(needle)
				}

				// SHAPE 2 — string concatenation: BASE + "/internal/..." + id
				//
				// Also every occurrence, for the same reason.
				for from := 0; from <= len(line); {
					idx, ok := concatStart(line, base, from)
					if !ok {
						break
					}
					// The operand right after `BASE +` must be a readable
					// string literal. When it is an identifier — a path held
					// in its own const, which is how one sabotage hid a
					// nonexistent route — there is nothing to compare, so it
					// is reported rather than dropped. This fires regardless
					// of whether "/internal" appears on this line, because
					// with the path in a constant it does not.
					if !startsWithStringLiteral(line[idx:]) {
						unreadableOperand = true
						from = idx + 1
						continue
					}
					raw, norm, parsed := scanConcatPath(line[idx:])
					if strings.HasPrefix(norm, "/internal") {
						refs = append(refs, internalRef{
							File: f.rel, Line: physicalLineOf(ll, raw, base), Raw: raw,
							Path: trimQuery(norm), Parsed: parsed,
						})
						found++
					}
					from = idx + 1
				}
			}

			// RESIDUAL CHECK — fail closed on a shape no scanner read.
			//
			// Two triggers, because there are two ways to be unreadable: a
			// line that names a base AND an /internal path yet yields
			// nothing, and a base concatenated with an operand this test
			// cannot read (where the path may not be on this line at all).
			// Reporting beats passing quietly: an unrecognised shape is
			// indistinguishable from "no such caller", which is exactly how
			// both concatenation holes stayed invisible.
			if found == 0 && (unreadableOperand || mentionsInternalPath(line, bases)) {
				why := "names a marketplace-api base URL and an /internal path"
				if unreadableOperand {
					why = "concatenates a marketplace-api base URL with an operand this test cannot read"
				}
				unparsed = append(unparsed, fmt.Sprintf("%s:%d (%s): %s",
					f.rel, ll.Num, why, strings.TrimSpace(line)))
			}
		}
	}
	sort.Slice(refs, func(i, j int) bool {
		if refs[i].File != refs[j].File {
			return refs[i].File < refs[j].File
		}
		return refs[i].Line < refs[j].Line
	})
	sort.Strings(unparsed)
	return refs, len(files), bases, unparsed
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

// logicalLine is one or more physical lines joined into a single expression,
// carrying the physical span so a match can be attributed to the right line.
type logicalLine struct {
	Text     string
	Num      int      // 1-based line where the statement starts
	Physical []string // the physical lines it was built from
}

// maxJoin bounds how many physical lines are folded into one logical line.
const maxJoin = 6

// joinLogicalLines folds continuation lines together so a reference split
// across lines is visible to the scanners and to the residual check.
//
// This exists because both were previously single-line. A review sabotage
// wrote
//
//	const url =
//	  MARKETPLACE_API_URL +
//	  "/internal/definitely-not-a-route/" + id;
//
// and passed: line 2 has the base and no path, line 3 has the path and no
// base, so mentionsInternalPath — which required both on the SAME line —
// never fired, and neither did either scanner. The backstop shared the
// scanner's blind spot, which is the worst property a backstop can have.
//
// Folded lines are CONSUMED rather than also starting their own logical line.
// Overlapping windows were the first attempt and they double-counted: the
// three real references became six, because each physical line started a
// window that re-matched what the previous window had already folded in.
//
// The join rule is syntactic and deliberately loose: a line is treated as
// continued when it ends in an operator or opener, or when its backticks are
// unbalanced.
//
// Over-joining does NOT merely cost line-number precision. An earlier version
// of this comment said so, and that was wrong in the direction that matters:
// because the scanners took only the FIRST match per logical line, folding
// more lines together folded more references into a single scan and dropped
// every one after the first. A URL array put a real route and a nonexistent
// one on one logical line and the second was never seen — green, with the
// residual backstop silenced because `found > 0`. Both scanners now advance
// through every match on the line, which is what makes loose joining safe.
// Under-joining remains what let the multi-line sabotage through.
func joinLogicalLines(lines []string) []logicalLine {
	var out []logicalLine
	for i := 0; i < len(lines); {
		text := lines[i]
		last := i
		for j := i + 1; j < len(lines) && j-i < maxJoin && isContinued(text); j++ {
			text += " " + strings.TrimSpace(lines[j])
			last = j
		}
		out = append(out, logicalLine{Text: text, Num: i + 1, Physical: lines[i : last+1]})
		i = last + 1
	}
	return out
}

// physicalLineOf returns the 1-based line number within a logical line whose
// text contains the first marker that matches, so a failure points at the
// line a reader will actually find the call on rather than at the start of the
// statement. Markers are tried most-specific first: the path as written
// distinguishes two references that share the same base identifier, which a
// bare needle cannot.
func physicalLineOf(ll logicalLine, markers ...string) int {
	for _, marker := range markers {
		if marker == "" {
			continue
		}
		for offset, text := range ll.Physical {
			if strings.Contains(text, marker) {
				return ll.Num + offset
			}
		}
	}
	return ll.Num
}

// isContinued reports whether a line's expression plainly carries on.
func isContinued(text string) bool {
	if strings.Count(text, "`")%2 == 1 {
		return true // an unterminated template literal
	}
	t := strings.TrimRight(strings.TrimSpace(text), " \t")
	if t == "" {
		return false
	}
	for _, suffix := range []string{"+", "(", ",", "=", "&&", "||", "?", ":", "${"} {
		if strings.HasSuffix(t, suffix) {
			return true
		}
	}
	return false
}

// concatStart finds `BASE +` on a line and returns the index of the operand
// that follows, so scanConcatPath can read the concatenation from there.
func concatStart(line, base string, from int) (int, bool) {
	if from < 0 || from > len(line) {
		return 0, false
	}
	for {
		idx := strings.Index(line[from:], base)
		if idx < 0 {
			return 0, false
		}
		idx += from
		from = idx + len(base)
		// Reject a longer identifier that merely contains the base name.
		if idx > 0 && isIdentByte(line[idx-1]) {
			continue
		}
		if from < len(line) && isIdentByte(line[from]) {
			continue
		}
		rest := strings.TrimLeft(line[from:], " \t")
		if !strings.HasPrefix(rest, "+") {
			continue
		}
		operand := strings.TrimLeft(rest[1:], " \t")
		return len(line) - len(operand), true
	}
}

// startsWithStringLiteral reports whether the next operand is a quoted
// string, i.e. a path this test can actually read.
func startsWithStringLiteral(rest string) bool {
	t := strings.TrimLeft(rest, " \t")
	return strings.HasPrefix(t, "\"") || strings.HasPrefix(t, "'") || strings.HasPrefix(t, "`")
}

func isIdentByte(b byte) bool {
	return b == '_' || b == '$' ||
		(b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9')
}

// scanConcatPath reads a `"literal" + expr + "literal"` chain into a path,
// rendering each non-literal operand as one ":param" segment — the same
// normalisation scanRefPath applies to a `${...}` substitution, so both
// shapes compare against gin templates identically.
//
// ok is false when the chain contains an unterminated string, so a path the
// scanner cannot read is reported rather than compared in a mangled form.
func scanConcatPath(rest string) (raw, normalised string, ok bool) {
	var rawB, normB strings.Builder
	i := 0
	for i < len(rest) {
		for i < len(rest) && (rest[i] == ' ' || rest[i] == '\t') {
			i++
		}
		if i >= len(rest) {
			break
		}
		switch c := rest[i]; c {
		case '"', '\'', '`':
			j := i + 1
			for ; j < len(rest); j++ {
				if rest[j] == '\\' {
					j++
					continue
				}
				if rest[j] == c {
					break
				}
			}
			if j >= len(rest) {
				return rawB.String(), normB.String(), false // unterminated
			}
			rawB.WriteString(rest[i : j+1])
			normB.WriteString(rest[i+1 : j])
			i = j + 1
		default:
			// A non-literal operand: read to the next top-level "+" or to
			// the end of the expression.
			depth, j := 0, i
			for ; j < len(rest); j++ {
				switch rest[j] {
				case '(', '[', '{':
					depth++
				case ')', ']', '}':
					if depth == 0 {
						goto operandDone
					}
					depth--
				case '+', ';', ',':
					if depth == 0 {
						goto operandDone
					}
				}
			}
		operandDone:
			rawB.WriteString(strings.TrimSpace(rest[i:j]))
			normB.WriteString(paramSegment)
			i = j
		}
		for i < len(rest) && (rest[i] == ' ' || rest[i] == '\t') {
			i++
		}
		if i < len(rest) && rest[i] == '+' {
			rawB.WriteString(" + ")
			i++
			continue
		}
		break
	}
	return rawB.String(), normB.String(), true
}

// stripComments removes comment text so a commented-out reference is neither
// counted as a live caller nor able to silence the residual backstop.
//
// It replaces a prefix heuristic that asked whether a TRIMMED line STARTED
// with "//", "*" or "/*". That was the backstop sharing the scanner's blind
// spot — the property this file's own comments call the worst a backstop can
// have — in two ways, both observed:
//
//   - `/* eslint-disable-next-line */ return fetch(BASE.concat("/internal/x"))`
//     starts with "/*", so the whole line was skipped. Exit 0.
//   - a comment line ending in "+" folds into the next line under
//     joinLogicalLines, putting "//" at the front of a logical line that
//     contains a real reference.
//
// Stripping rather than skipping fixes both, and it is strictly better than
// applying the heuristic per physical line, which cannot help when the
// comment and the call share one line.
//
// "//" is only treated as a comment when not preceded by ":", so a "https://"
// inside a string is left alone. That is a heuristic, not a tokenizer: a "//"
// inside a string literal that is not part of a URL scheme would truncate the
// rest of the line. The failure direction is a missed reference, which the
// residual check cannot then see either — recorded in the gap list.
func stripComments(text string) string {
	// Block comments first, including an unterminated one.
	for {
		i := strings.Index(text, "/*")
		if i < 0 {
			break
		}
		rest := text[i+2:]
		j := strings.Index(rest, "*/")
		if j < 0 {
			text = text[:i]
			break
		}
		text = text[:i] + " " + rest[j+2:]
	}
	// Then a line comment, skipping "://".
	for from := 0; ; {
		i := strings.Index(text[from:], "//")
		if i < 0 {
			break
		}
		i += from
		if i > 0 && text[i-1] == ':' {
			from = i + 2
			continue
		}
		text = text[:i]
		break
	}
	return text
}

// mentionsInternalPath reports whether a line names one of the marketplace-api
// base identifiers alongside an /internal path, ignoring comment lines. It is
// the trigger for the residual check: such a line MUST yield a reference.
func mentionsInternalPath(line string, bases []string) bool {
	if !strings.Contains(line, "/internal") {
		return false
	}
	for _, base := range bases {
		if strings.Contains(line, base) {
			return true
		}
	}
	return false
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
//
// # What remains uncovered on the frontend side
//
// Observed, not reasoned about. Not a completeness claim.
//
//   - A WRAPPER HELPER that applies the base URL internally —
//     marketplaceInternalFetch("/internal/store-active-domain/" + slug) —
//     is invisible to both scanners AND to the residual check, because no
//     base identifier appears at the call site at all. Resolving it needs
//     TypeScript call-graph analysis. Nothing in apps/* does this today.
//   - A REFERENCE SPLIT ACROSS MORE THAN maxJoin (6) PHYSICAL LINES: the
//     logical line stops short and the pieces are scanned separately.
//   - HTTP METHOD IS NOT COMPARED. A frontend POST to a path the backend
//     serves only as GET passes. Deliberate: the deletion case this exists
//     for removes every method on the path.
//   - A PATH BUILT WITHOUT ANY STRING LITERAL, e.g. entirely from
//     path.join(...) or an array join. The residual check fires only when a
//     base identifier is adjacent to an unreadable operand or an /internal
//     literal is on the logical line.
//   - A "//" INSIDE A STRING LITERAL that is not part of a URL scheme:
//     stripComments truncates the rest of the line, so a reference after it
//     is lost — and the residual check, seeing the same stripped text, cannot
//     report it either.
//   - A REFERENCE INSIDE A MULTI-LINE BLOCK COMMENT is still seen as live,
//     because comments are stripped per physical line and the opening /* is
//     on a different line. That direction is a false FAILURE, not a miss.
//
// Fixed in round 5, recorded because the failure mode was subtle: only the
// FIRST reference per logical line used to be extracted, so a URL array whose
// first entry was a real route silently swallowed a second entry pointing at
// a route that did not exist — and `found > 0` kept the residual check quiet.
// Both scanners now advance through every match.
func TestFrontendInternalRoutesAreDeclared(t *testing.T) {
	root := appsDir(t)
	refs, filesScanned, bases, unparsed := extractInternalRefs(t, root)

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

	require.Emptyf(t, unparsed,
		"these apps/* call sites reference marketplace-api in a shape neither the "+
			"template-literal nor the concatenation scanner could read a route out of:\n  %s\n\n"+
			"An unrecognised call shape is indistinguishable from no caller at all, which is "+
			"how the concatenation hole stayed invisible. Teach the extractor this shape "+
			"rather than leaving it silent.", strings.Join(unparsed, "\n  "))

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
