import { expect, test } from "@playwright/test";
import { readFileSync } from "node:fs";
import path from "node:path";

/**
 * Regression guard for GitHub issue #147.
 *
 * React keys don't stay in React. In the App Router they are serialised
 * into the RSC flight payload that ships inside the HTML document, and
 * Googlebot harvests URL-shaped strings out of that payload and crawls
 * them. The footer used to key its list items on `${href}-${label}`,
 * which produced strings like:
 *
 *     ["$","li","/blog-Journal",{...,"href":"/blog",...}]
 *
 * The href was always correct — but Google crawled the *key*. Search
 * Console's "Not found (404)" report filled up with phantom paths:
 * /blog-Journal, /cookies-Cookies, /refunds-Refunds, and every other
 * footer link was queued to produce one.
 *
 * The rule this pins down: a key that a crawler could mistake for a path
 * must not be built by concatenating an href with anything. Keying on the
 * href alone is fine — it is unique per column, and if it does leak it
 * resolves to a real page.
 */
const FOOTER_SOURCES: ReadonlyArray<string> = [
  "components/marketing/Footer.tsx",
  "components/onboarding/SlimFooter.tsx",
];

// A key built as `${something}-${somethingElse}` inside a template
// literal — the exact shape that produced the phantom URLs.
const COMPOSITE_KEY = /key=\{`[^`]*\$\{[^}]*\}[^`]*-[^`]*\$\{[^}]*\}[^`]*`\}/;

for (const relative of FOOTER_SOURCES) {
  test(`${relative} does not build a URL-shaped composite React key`, () => {
    const file = path.join(__dirname, "../..", relative);
    const source = readFileSync(file, "utf8");

    const offending = source
      .split("\n")
      .map((line, i) => ({ line: line.trim(), number: i + 1 }))
      .filter(({ line }) => COMPOSITE_KEY.test(line));

    expect(
      offending,
      `${relative} builds a React key by concatenating interpolated values. ` +
        "If either half is an href, the key serialises into the RSC payload " +
        "as a path-shaped string and Googlebot crawls it as a real URL, " +
        "producing a 404 that never existed (#147). Key on the href alone.",
    ).toEqual([]);
  });
}

test("footer link hrefs are unique, so keying on href alone is safe", () => {
  const file = path.join(__dirname, "../../components/marketing/Footer.tsx");
  const source = readFileSync(file, "utf8");

  const hrefs = [...source.matchAll(/\{\s*href:\s*"([^"]+)"/g)].map((m) => m[1]);

  expect(hrefs.length).toBeGreaterThan(0);
  expect(new Set(hrefs).size).toBe(hrefs.length);
});
