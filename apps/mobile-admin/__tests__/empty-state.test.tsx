import { StyleSheet } from "react-native";
import { render } from "@testing-library/react-native";

import { EmptyState } from "@/components/ui/EmptyState";

/**
 * Guards the project invariant "shared primitives take ADDITIVE props, never
 * changed defaults" at the one place it is actually load-bearing and was
 * entirely unenforced: `EmptyState.align`.
 *
 * `align` was added so the two editorial screens built in this increment
 * (the Dashboard's queue empty state, and Orders) could be left-aligned
 * without restyling the ~30 pre-existing call sites that were designed
 * against the centred treatment. That promise is only worth anything if the
 * DEFAULT cannot drift: flipping `align = "center"` to `align = "left"`
 * silently re-lays-out every one of those call sites, and — measured before
 * this file existed — left all 822 tests green.
 *
 * This is the same failure mode that already cost this programme a review
 * round, when changing `Eyebrow`'s own default gutter rippled into ~15 call
 * sites including 5 screens the report claimed were untouched.
 */
describe("EmptyState — the align default is part of the contract", () => {
  it("centres by default, so the pre-existing call sites are unchanged", () => {
    const { getByTestId, getByText } = render(<EmptyState title="Nothing here" message="Try later" />);

    const container = StyleSheet.flatten(getByTestId("empty-state").props.style);
    expect(container.alignItems).toBe("center");

    // The cross-axis alignment and the TEXT alignment have to agree — a
    // centred box full of left-aligned text is its own defect. Read off
    // `className`, not `style`: `Text` maps `align` to a nativewind class,
    // and RNTL renders without the nativewind runtime, so `style` is
    // undefined here (the same reason this suite cannot assert colours).
    expect(getByText("Nothing here").props.className).toContain("text-center");
    expect(getByText("Try later").props.className).toContain("text-center");
  });

  it("left-aligns only when a caller explicitly opts in", () => {
    const { getByTestId, getByText } = render(
      <EmptyState title="Nothing here" message="Try later" align="left" />,
    );

    const container = StyleSheet.flatten(getByTestId("empty-state").props.style);
    expect(container.alignItems).toBe("flex-start");
    expect(getByText("Nothing here").props.className).toContain("text-left");
    expect(getByText("Try later").props.className).toContain("text-left");
  });
});

/**
 * The other half of that contract, and the reason the default can stay put:
 * every CALL SITE that is a full-canvas list empty state must opt in to
 * `align="left"`.
 *
 * A render test can only ever cover the screen it renders, and this defect
 * was a consistency one spread across 31 call sites — Orders and the
 * Dashboard were left-aligned while thirteen other list screens were still
 * centred, so the same moment looked like two different designs depending on
 * which tab you were on. So this scans the source instead: it fails for any
 * NEW `EmptyState` that forgets the prop, not just the ones fixed here, and
 * it is the only shape of test that can hold an invariant of the form "all of
 * them agree".
 *
 * The exceptions are enumerated, not inferred. Each one is also commented at
 * its call site; if you add to this list, say why there too.
 */
// `@types/node` is not in this project's `types` list and adding it is a
// dependency change — so the same `require` + local `declare` shape the other
// source-scanning tests in this suite already use (see
// product-detail-sections.test.tsx).
declare const __dirname: string;
// eslint-disable-next-line @typescript-eslint/no-var-requires
const fs = require("fs");
// eslint-disable-next-line @typescript-eslint/no-var-requires
const path = require("path");

const APP_ROOT = path.join(__dirname, "..");
const SCANNED_DIRS = ["app", "components"];

/** Call sites that are deliberately CENTRED. See the comment at each one. */
const CENTRED_BY_DESIGN: ReadonlyArray<{ file: string; title: string }> = [
  // A modal sheet on a 16pt gutter; `EmptyState`'s own is hardcoded 20pt, so
  // left-aligning it would invent a second left edge 4pt inside the sheet's.
  { file: "components/products/CategoryPickerSheet.tsx", title: "No categories yet" },
  // Pre-tenant gates: no header, no rows, no established left edge, and a
  // centred pill CTA as their sibling.
  { file: "components/TenantGate.tsx", title: "No access" },
  { file: "components/TenantGate.tsx", title: "No store yet" },
];

function sourceFiles(): string[] {
  const out: string[] = [];
  const walk = (dir: string) => {
    for (const entry of fs.readdirSync(dir, { withFileTypes: true }) as Array<{
      name: string;
      isDirectory(): boolean;
    }>) {
      const full = path.join(dir, entry.name);
      if (entry.isDirectory()) {
        if (entry.name !== "node_modules") walk(full);
      } else if (entry.name.endsWith(".tsx")) {
        out.push(full);
      }
    }
  };
  for (const dir of SCANNED_DIRS) walk(path.join(APP_ROOT, dir));
  return out;
}

interface CallSite {
  file: string;
  source: string;
  index: number;
}

function emptyStateCallSites(): CallSite[] {
  const sites: CallSite[] = [];
  for (const full of sourceFiles()) {
    const file = path.relative(APP_ROOT, full);
    const text: string = fs.readFileSync(full, "utf8");
    for (let at = text.indexOf("<EmptyState"); at !== -1; at = text.indexOf("<EmptyState", at + 1)) {
      // Every call site is a self-closing element with no nested JSX, so the
      // next `/>` is reliably this element's end.
      const end = text.indexOf("/>", at);
      expect(end).toBeGreaterThan(at);
      sites.push({ file, source: text.slice(at, end + 2), index: at });
    }
  }
  return sites;
}

function isCentredByDesign(site: CallSite): boolean {
  return CENTRED_BY_DESIGN.some(
    (allowed) => allowed.file === site.file && site.source.includes(`"${allowed.title}"`),
  );
}

describe("EmptyState call sites — every list screen treats the same moment the same way", () => {
  it("finds the call sites at all, so a broken scan cannot pass vacuously", () => {
    const sites = emptyStateCallSites();
    expect(sites.length).toBeGreaterThanOrEqual(30);
    // And the exceptions must all still exist — a renamed title would
    // otherwise quietly turn into an unguarded centred call site.
    for (const allowed of CENTRED_BY_DESIGN) {
      expect(
        sites.filter((s) => s.file === allowed.file && s.source.includes(`"${allowed.title}"`)),
      ).toHaveLength(1);
    }
  });

  it("passes align=\"left\" everywhere except the enumerated exceptions", () => {
    const centred = emptyStateCallSites()
      .filter((site) => !site.source.includes('align="left"') && !isCentredByDesign(site))
      .map((site) => `${site.file} → ${site.source.split("\n")[1]?.trim() ?? site.source}`);
    expect(centred).toEqual([]);
  });

  /**
   * The trap that makes the prop above insufficient on its own. A wrapper
   * with `alignItems: "center"` shrink-wraps its child and re-centres the
   * whole block, so an error state left-aligned INSIDE `styles.centered`
   * renders centred anyway — with the prop present and every test green.
   * Orders hit this and answered it with `errorSlot: { flex: 1 }`.
   */
  it("never wraps a left-aligned empty state in a re-centring container", () => {
    const offenders: string[] = [];
    for (const full of sourceFiles()) {
      const text: string = fs.readFileSync(full, "utf8");
      if (/<View style=\{styles\.centered\}>\s*\n\s*<EmptyState[\s\S]{0,200}?align="left"/.test(text)) {
        offenders.push(path.relative(APP_ROOT, full));
      }
    }
    expect(offenders).toEqual([]);
  });
});
