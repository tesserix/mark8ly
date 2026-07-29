// Task 11 — the increment's own guard. Every previous task in increment 3
// could pass individually while the rollout as a whole drifted: one screen
// quietly omitting the swipe-convention suite, one screen still on the old
// header, one EmptyState re-centred by a wrapper. This file is a
// CORPUS-based meta-test, in the shape of no-touchable-opacity.test.ts and
// empty-state.test.tsx's call-site scan: it derives its own corpus by
// scanning the tree rather than hand-copying a screen list, so a screen
// added tomorrow is covered automatically and a screen removed does not
// leave a phantom entry.
//
// 🔴 Lesson recorded by the controller's own addendum for this task: "A
// COVERAGE TEST WITH A HAND-COPIED LIST IS NOT A COVERAGE TEST." Every
// corpus below is DERIVED — see the "searches a non-empty, non-trivial
// corpus" positive control on each one, which exists so a broken derivation
// (a renamed directory, a renamed import specifier) cannot make every
// assertion below pass vacuously.
//
// tsconfig scopes `types` to ["jest"], so Node's ambient globals are not
// picked up automatically — declare exactly what this file's fs/path calls
// need, matching the existing pattern in no-touchable-opacity.test.ts and
// product-detail-sections.test.tsx.
declare const __dirname: string;

// eslint-disable-next-line @typescript-eslint/no-var-requires
const fs = require("fs");
// eslint-disable-next-line @typescript-eslint/no-var-requires
const path = require("path");

const APP_ROOT = path.resolve(__dirname, "..");

/** Every `.tsx`/`.ts` file under `dir` (relative to APP_ROOT), recursively. */
function walk(dir: string): string[] {
  const out: string[] = [];
  const abs = path.join(APP_ROOT, dir);
  const visit = (full: string) => {
    for (const entry of fs.readdirSync(full, { withFileTypes: true }) as Array<{
      name: string;
      isDirectory(): boolean;
    }>) {
      const entryFull = path.join(full, entry.name);
      if (entry.isDirectory()) {
        if (entry.name === "node_modules") continue;
        visit(entryFull);
      } else if (entry.name.endsWith(".tsx") || entry.name.endsWith(".ts")) {
        out.push(path.relative(APP_ROOT, entryFull));
      }
    }
  };
  visit(abs);
  return out;
}

const APP_FILES = walk("app");
const TEST_FILES = walk("__tests__");

function read(relPath: string): string {
  return fs.readFileSync(path.join(APP_ROOT, relPath), "utf8");
}

const LIB_FILES = walk("lib");

/**
 * The Sweep A DETECTION logic — hoisted to module scope so the "Sweep A
 * guard" describe below and the "Sweep A ratchet" describe further down
 * share ONE detector. Two detectors that could disagree on what counts as a
 * violation would be worse than one: this way, tightening or loosening the
 * definition happens in exactly one place and both consumers see it.
 */

/** Every `useCallback(...)` call in `source`, as its full source text. */
function extractUseCallbacks(source: string): string[] {
  const calls: string[] = [];
  const marker = "useCallback(";
  let idx = source.indexOf(marker);
  while (idx !== -1) {
    let depth = 0;
    let i = idx + marker.length - 1;
    do {
      const ch = source[i];
      if (ch === "(") depth++;
      else if (ch === ")") depth--;
      i++;
    } while (depth > 0 && i < source.length);
    calls.push(source.slice(idx, i));
    idx = source.indexOf(marker, i);
  }
  return calls;
}

/**
 * Mutation identifiers this useCallback calls `.mutate(` on, that ALSO
 * appear bare (not as `.mutate`) in its own trailing dependency array.
 */
function bareMutationDeps(call: string): string[] {
  const depsMatch = call.match(/\[([^[\]]*)\]\s*\)\s*$/);
  if (!depsMatch) return [];
  const deps = depsMatch[1]
    .split(",")
    .map((s: string) => s.trim())
    .filter(Boolean);
  const mutateCalls = [...call.matchAll(/\b([A-Za-z_$][\w$]*)\.mutate\(/g)].map((m) => m[1]);
  return [...new Set(mutateCalls)].filter((name) => deps.includes(name));
}

/** The four screens Task 11's Sweep A actually fixed — see the describe block below. */
const SWEEP_A_FILES = [
  "app/(tabs)/customers/[id].tsx",
  "app/(tabs)/products/[id].tsx",
  "app/(tabs)/more/account.tsx",
  "app/(tabs)/orders/[id].tsx",
];

/**
 * Every offending site in `files`, as a `"path:line (mutationNames)"`
 * string — so a ratchet failure names exactly what to go fix, without
 * making anyone re-derive the scan by hand. Locates each violating
 * `useCallback` by re-finding its (already-extracted) source text within
 * the file, walking forward so two textually-identical calls in the same
 * file resolve to their own distinct lines rather than both matching the
 * first occurrence.
 */
function findViolationSites(files: string[]): string[] {
  const sites: string[] = [];
  for (const f of files) {
    const source = read(f);
    let searchFrom = 0;
    for (const call of extractUseCallbacks(source)) {
      const idx = source.indexOf(call, searchFrom);
      if (idx === -1) continue;
      searchFrom = idx + call.length;
      const violations = bareMutationDeps(call);
      if (violations.length > 0) {
        const line = source.slice(0, idx).split("\n").length;
        sites.push(`${f}:${line} (${violations.join(", ")})`);
      }
    }
  }
  return sites;
}

/**
 * Screens that MOUNT a `SwipeRow` — derived from the JSX tag itself, not an
 * import (an unused import proves nothing about what's on screen; a mounted
 * tag does). `<SwipeRow` is unambiguous: nothing else in this codebase opens
 * with that string.
 */
const SWIPE_SCREENS = APP_FILES.filter((f) => read(f).includes("<SwipeRow")).sort();

/**
 * Screens that implement the scroll-driven collapsing header — derived from
 * importing `useCollapsingScroll`, the hook every one of these screens uses
 * to feed the header its scroll position. This is a SEPARATE module from
 * `CollapsingHeader` itself, so deriving the corpus from it (rather than
 * from `CollapsingHeader` usage) keeps the corpus and the "renders
 * CollapsingHeader" assertion in test 2 from being circular: a screen that
 * wires up the scroll listener but regresses to the wrong header component
 * is exactly the drift this guards.
 */
const COLLAPSING_SCREENS = APP_FILES.filter((f) => read(f).includes("use-collapsing-scroll")).sort();

describe("rollout-invariants corpora — derived, not hand-copied", () => {
  it("finds a real, non-trivial app/ tree to scan", () => {
    expect(APP_FILES.length).toBeGreaterThan(30);
  });

  it("derives a non-empty SWIPE_SCREENS corpus", () => {
    expect(SWIPE_SCREENS.length).toBeGreaterThanOrEqual(6);
  });

  it("derives a non-empty COLLAPSING_SCREENS corpus that is a superset of SWIPE_SCREENS", () => {
    expect(COLLAPSING_SCREENS.length).toBeGreaterThanOrEqual(SWIPE_SCREENS.length);
    for (const f of SWIPE_SCREENS) expect(COLLAPSING_SCREENS).toContain(f);
  });
});

describe("every screen that mounts a SwipeRow has a suite asserting the swipe convention", () => {
  /**
   * Maps a screen's app-relative path to the bare specifier a test file
   * would import it by (matching the shape used throughout __tests__, e.g.
   * `import ProductsScreen from "../app/(tabs)/products/index";`).
   */
  function importSpecifier(screenPath: string): string {
    return `../${screenPath.replace(/\.tsx$/, "")}`;
  }

  /** Every __tests__ file that imports this screen module. */
  function testFilesFor(screenPath: string): string[] {
    const specifier = importSpecifier(screenPath);
    return TEST_FILES.filter((t) => read(t).includes(`"${specifier}"`));
  }

  it("finds test files that import at least one swiping screen (positive control)", () => {
    const total = SWIPE_SCREENS.flatMap(testFilesFor).length;
    expect(total).toBeGreaterThan(0);
  });

  it.each(SWIPE_SCREENS)("%s has a suite importing assertSwipeConvention", (screenPath: string) => {
    const suites = testFilesFor(screenPath);
    // A screen with no suite importing it at all fails here too — that is
    // its own coverage gap, not a pass.
    expect(suites.length).toBeGreaterThan(0);
    const covered = suites.some((t) => read(t).includes("assertSwipeConvention"));
    expect(covered).toBe(true);
  });
});

describe("every rolled-out list screen uses CollapsingHeader, not PageHeader or BackHeader", () => {
  it.each(COLLAPSING_SCREENS)("%s mounts CollapsingHeader and neither PageHeader nor BackHeader", (screenPath: string) => {
    const source = read(screenPath);
    // JSX-tag matches, not a bare substring grep — `PageHeader`/`BackHeader`
    // legitimately appear in COMMENTS on some of these screens (explaining
    // the header they migrated OFF), and a substring match on those comments
    // would be a false positive that never catches a real regression.
    expect(source).toContain("<CollapsingHeader");
    expect(source).not.toContain("<PageHeader");
    expect(source).not.toContain("<BackHeader");
  });
});

describe("no rolled-out screen wraps an EmptyState in a re-centring container", () => {
  // Scoped to the screens that actually render the shared `EmptyState`
  // directly — the Dashboard is COLLAPSING_SCREENS but renders its own
  // bespoke `QueueEmptyState` instead (see components/dashboard/
  // QueueEmptyState.tsx), so it has no `errorSlot` to check and would be a
  // false failure if included.
  const CORPUS = COLLAPSING_SCREENS.filter((f) => read(f).includes("<EmptyState"));

  it("finds screens that render EmptyState directly (positive control)", () => {
    expect(CORPUS.length).toBeGreaterThanOrEqual(8);
  });

  it.each(CORPUS)("%s defines errorSlot: { flex: 1 } for its list empty/error state", (screenPath: string) => {
    expect(read(screenPath)).toContain("errorSlot: { flex: 1 }");
  });

  it.each(CORPUS)("%s never wraps a left-aligned EmptyState in styles.centered", (screenPath: string) => {
    // The trap this whole assertion exists for: a wrapper with
    // `alignItems: "center"` shrink-wraps its child and re-centres the block
    // regardless of the EmptyState's own `align="left"` prop.
    const source = read(screenPath);
    expect(
      /<View style=\{styles\.centered\}>\s*\n\s*<EmptyState[\s\S]{0,200}?align="left"/.test(source),
    ).toBe(false);
  });
});

describe("no action anywhere opts into full-swipe auto-fire", () => {
  // This app has no undo, so nothing may fire from the drag gesture itself —
  // a revealed action is always tapped. Scoped to the whole app/ tree (not
  // just the rolled-out screens): the property is meant to hold everywhere,
  // and a new SwipeRow use anywhere in the app inherits the same rule.
  const AUTO_FIRE_PATTERN = /autoFireOnFullSwipe\s*[:=]\s*\{?\s*true/;

  it("finds SwipeRow action config to scan (positive control)", () => {
    const withAutoFireProp = APP_FILES.filter((f) => read(f).includes("autoFireOnFullSwipe"));
    // The prop itself is referenced (as `autoFireOnFullSwipe?:` in SwipeRow's
    // own type, and read by test-utils/swipe-convention.tsx) even though no
    // screen sets it to true — this proves the scan is searching real files
    // that mention the prop, not passing vacuously over an empty corpus.
    expect(withAutoFireProp.length).toBeGreaterThan(0);
  });

  it("no file in app/ sets autoFireOnFullSwipe to true", () => {
    const offenders = APP_FILES.filter((f) => AUTO_FIRE_PATTERN.test(read(f)));
    expect(offenders).toEqual([]);
  });
});

describe("no screen added an optimistic hide", () => {
  /**
   * The Dashboard is the ONE deliberate, pre-existing exception: its
   * `useState<Dismissed>` optimistic-hide state (app/(tabs)/index.tsx) is
   * documented and load-bearing (see useExpireHidesOnFreshAnswer) and
   * predates increment 3. Every list screen ADDED or CONVERTED in this
   * increment refetches itself instead — a hide anywhere else would be the
   * Dashboard's four-round bug (see the doc comment on
   * useExpireHidesOnFreshAnswer) re-imported into a screen that never needed
   * it.
   */
  const DASHBOARD_EXEMPTION = "app/(tabs)/index.tsx";
  const CORPUS = COLLAPSING_SCREENS.filter((f) => f !== DASHBOARD_EXEMPTION);

  it("the Dashboard exemption still names a real, currently-collapsing screen", () => {
    // Guards the exemption itself from going stale: if the Dashboard were
    // ever renamed or stopped using the collapsing-header pattern, this
    // fails loudly instead of silently exempting nothing.
    expect(COLLAPSING_SCREENS).toContain(DASHBOARD_EXEMPTION);
  });

  it("finds screens to check beyond the exemption (positive control)", () => {
    expect(CORPUS.length).toBeGreaterThanOrEqual(8);
  });

  it.each(CORPUS)("%s declares no local hidden/dismissed-id state", (screenPath: string) => {
    const source = read(screenPath);
    // The concrete shape an optimistic hide takes: component state whose
    // TYPE is (or contains) a Set of ids used to filter the rendered list —
    // exactly what the Dashboard's own `Dismissed` state is. `useBusyIds()`
    // — the sanctioned per-row gesture guard every one of these screens
    // uses instead — keeps its own Set internal to lib/use-busy-ids.ts, so a
    // clean screen never declares one locally at all.
    expect(source).not.toMatch(/useState\s*<[^>]*Set</);
    expect(source).not.toMatch(/useState\s*\(\s*\(\s*\)\s*=>\s*new Set/);
  });
});

describe("Sweep A guard — a useCallback that calls a mutation's .mutate must depend on .mutate, not the mutation object", () => {
  // `useMutation` (TanStack Query) returns a NEW OBJECT every render, so a
  // `useCallback` that lists the mutation object itself in its dependency
  // array never memoises anything — it rebuilds every render regardless.
  // Scoped to the four screens Task 11's Sweep A actually touched
  // (customers/[id].tsx, products/[id].tsx, more/account.tsx,
  // orders/[id].tsx) rather than the whole app: a repo-wide sweep found the
  // SAME pre-existing defect in 14 more sites across 12 further files that
  // predate increment 3 (campaigns/coupons/segments/tickets detail+create
  // screens, gift-cards/new, loyalty member adjust, branding, team invite,
  // and one product-media hook) — out of this task's stated scope and
  // guarded (without being fixed) by the ratchet describe block below. The
  // DETECTION logic below is
  // generic (balanced-paren useCallback extraction), so a new violation
  // added to any of these four files — not just the ones fixed today — is
  // caught automatically.
  //
  // SWEEP_A_FILES, extractUseCallbacks, and bareMutationDeps now live at
  // module scope (above) so the ratchet describe below reuses this exact
  // detector instead of a second, hand-rolled one.

  it("every named Sweep A file still exists", () => {
    for (const f of SWEEP_A_FILES) expect(APP_FILES).toContain(f);
  });

  it.each(SWEEP_A_FILES)("%s has no useCallback depending on a raw mutation object it also .mutate()s", (screenPath: string) => {
    const source = read(screenPath);
    const violations = extractUseCallbacks(source).flatMap(bareMutationDeps);
    expect(violations).toEqual([]);
  });
});

describe("Sweep A ratchet — the 14 pre-existing violations left out of Sweep A's scope must not grow", () => {
  // WHAT this constant is: the count of useCallback-depends-on-a-mutation-
  // OBJECT sites (the exact defect Sweep A fixed above) in app/ + lib/,
  // OUTSIDE the four SWEEP_A_FILES already fixed and guarded by name above.
  // Measured directly by running findViolationSites() over the repo as it
  // stood when this ratchet was added — see the failure message below for
  // the current, always-fresh file:line list rather than trusting this
  // comment to stay in sync.
  //
  // WHY these sites are out of scope here: they predate increment 3
  // (campaigns, coupons, gift-cards, segments, tickets, loyalty, branding,
  // team-invite screens, and one product-media hook) and fixing 14 sites
  // across a dozen files was explicitly deferred out of Task 11 — see the
  // Sweep A comment above and the task report. This describe block exists
  // so that deferral doesn't also mean "and nobody will notice if it gets
  // worse": every one of these sites is still a real defect (a useCallback
  // that never memoises anything, because `useMutation` hands back a fresh
  // object every render), just not one this task fixes.
  //
  // THE CORRECT DIRECTION IS DOWN. This is a ratchet, not a budget: as
  // sites get fixed (ideally by deleting this file's slice of tech debt one
  // PR at a time), lower KNOWN_VIOLATION_CEILING to match — the test below
  // fails loudly, on purpose, if the measured count drops below the
  // constant, precisely so a stale ceiling can never silently re-permit a
  // regression back up to a number that used to be true.
  const KNOWN_VIOLATION_CEILING = 14;

  const RATCHET_CORPUS = [...APP_FILES, ...LIB_FILES].filter((f) => !SWEEP_A_FILES.includes(f));

  it("finds a real, non-trivial app/+lib/ corpus outside the Sweep A files to scan (positive control)", () => {
    expect(RATCHET_CORPUS.length).toBeGreaterThan(50);
  });

  it(`known pre-existing violation count outside the Sweep A files stays exactly at the ratcheted ceiling (currently ${KNOWN_VIOLATION_CEILING})`, () => {
    const sites = findViolationSites(RATCHET_CORPUS);

    if (sites.length > KNOWN_VIOLATION_CEILING) {
      throw new Error(
        `Violation count rose from ${KNOWN_VIOLATION_CEILING} to ${sites.length}. ` +
          `A useCallback depending on a raw mutation object (instead of its .mutate) ` +
          `was added or an existing one grew. Offending sites:\n${sites.join("\n")}`,
      );
    }

    if (sites.length < KNOWN_VIOLATION_CEILING) {
      throw new Error(
        `Violation count DROPPED from ${KNOWN_VIOLATION_CEILING} to ${sites.length} — ` +
          `progress, but the ceiling is now stale and silently permits regressing back up to ` +
          `${KNOWN_VIOLATION_CEILING}. Lower KNOWN_VIOLATION_CEILING in this file (__tests__/rollout-invariants.test.ts) ` +
          `to ${sites.length} to lock the improvement in. Remaining offending sites:\n${
            sites.join("\n") || "(none left — delete this describe block's KNOWN_VIOLATION_CEILING check entirely)"
          }`,
      );
    }

    expect(sites.length).toBe(KNOWN_VIOLATION_CEILING);
  });
});
