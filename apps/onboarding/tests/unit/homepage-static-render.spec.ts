import { expect, test } from "@playwright/test";
import { readFileSync } from "node:fs";
import path from "node:path";

/**
 * Regression guard for GitHub issue #597.
 *
 * `app/page.tsx` used to call `await cookies()` to read `mk8_currency`
 * for geo-localised pricing. In the App Router one request-scoped read
 * opts the entire route out of static generation: Next marks `/`
 * dynamic, emits `cache-control: private, no-store`, and Cloudflare
 * reports `cf-cache-status: DYNAMIC`. Every visit to the highest-traffic
 * page then round-trips to a single pod in Sydney — measured TTFB
 * 380–460ms, 1,380ms of a 3.27s LCP under Lighthouse mobile throttling,
 * plus cold-start risk on `minScale: 0`.
 *
 * The failure mode is silent: the page renders correctly, the tests pass,
 * and the only symptom is a `ƒ` instead of a `○` in the build's route
 * table. Nothing about the code looks wrong. So this guard names the APIs
 * that cause it rather than waiting for someone to read the route table.
 *
 * The definitive check remains `npm run build -w @mark8ly/onboarding`:
 * `/` must be listed `○ (Static)`. This test is the cheap early warning.
 */
const REPO_ROOT = path.join(__dirname, "../../../..");
const HOMEPAGE = "apps/onboarding/app/page.tsx";
const LAYOUT = "apps/onboarding/app/layout.tsx";

function read(rel: string): string {
  return readFileSync(path.join(REPO_ROOT, rel), "utf8");
}

/**
 * Comments are stripped before scanning for the same reason as
 * pricing-surfaces-truth.spec.ts: the comments in these files name the
 * banned APIs in order to explain why they are banned, and scanning raw
 * source would make documenting the fix trip the guard enforcing it.
 */
function codeOnly(source: string): string {
  return source.replace(/\/\*[\s\S]*?\*\//g, "").replace(/^\s*\/\/.*$/gm, "");
}

// Every App Router API that forces a dynamic render. `searchParams` is
// included because awaiting it in a page has the same effect even though
// it arrives as a prop rather than an import.
const DYNAMIC_APIS: ReadonlyArray<[string, RegExp]> = [
  ["cookies()", /\bcookies\s*\(/],
  ["headers()", /\bheaders\s*\(/],
  ["draftMode()", /\bdraftMode\s*\(/],
  ["connection()", /\bconnection\s*\(/],
  ["unstable_noStore()", /\bunstable_noStore\s*\(/],
  ["searchParams", /\bsearchParams\b/],
  ["export const dynamic", /export\s+const\s+dynamic\b/],
  ["export const revalidate = 0", /export\s+const\s+revalidate\s*=\s*0\b/],
];

for (const file of [HOMEPAGE, LAYOUT]) {
  test(`${file} uses no request-scoped API that would make / dynamic (#597)`, () => {
    const src = codeOnly(read(file));
    for (const [name, pattern] of DYNAMIC_APIS) {
      expect(
        pattern.test(src),
        `${file} references ${name}, which opts / out of static generation. ` +
          "Anything that depends on the visitor belongs in a client island — " +
          "see apps/onboarding/lib/geo/use-geo-currency.ts for the currency case.",
      ).toBe(false);
    }
  });
}

test("the currency-bearing marketing sections are client islands (#597)", () => {
  // These two render prices in the visitor's currency. They can only do
  // that without the server reading a cookie if they resolve it after
  // mount, which requires the client boundary.
  const islands = [
    "apps/onboarding/components/marketing/Pricing.tsx",
    "apps/onboarding/components/marketing/Comparison.tsx",
  ];

  for (const rel of islands) {
    const src = read(rel);
    expect(
      /^\s*(['"])use client\1/.test(src),
      `${rel} must open with a "use client" directive — it calls useGeoCurrency()`,
    ).toBe(true);
    expect(src, `${rel} should resolve currency via useGeoCurrency()`).toContain(
      "useGeoCurrency",
    );
  }
});

test("the homepage JSON-LD is pinned to the prerendered currency (#597)", () => {
  const src = codeOnly(read(HOMEPAGE));
  // Only one HTML document is prerendered, so the structured data can
  // only honestly carry one currency — and it must be the same one the
  // visible prices in that document are denominated in.
  expect(src).toContain("buildHomeJsonLd(PRERENDER_CURRENCY)");
});
