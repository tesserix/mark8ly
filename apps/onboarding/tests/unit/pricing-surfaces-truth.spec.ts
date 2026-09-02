import { expect, test } from "@playwright/test";
import { readFileSync } from "node:fs";
import path from "node:path";

/**
 * Cross-surface guard for GitHub issue #564.
 *
 * Plan copy lives in three places and they drifted apart:
 *
 *   apps/onboarding/components/marketing/Pricing.tsx   → mark8ly.com/#pricing
 *   apps/admin/lib/copy/pricing.ts                     → admin.mark8ly.com/pricing (public, indexed)
 *   apps/onboarding/public/llms-full.txt               → mark8ly.com/llms-full.txt
 *
 * #413 caught the admin page quoting prices the billing catalog would not
 * charge. #418 fixed the prices and left the feature bullets, which turned
 * out to describe a different product: invented product/order/staff caps,
 * storefront counts wrong in both directions, and "dedicated
 * infrastructure" / "SLA-backed uptime" on a shared Cloud SQL micro.
 *
 * The authority is the Go plan matrix, not any of the copy files. These
 * tests read it directly so the copy cannot drift from what is actually
 * enforced without failing the build.
 */
const REPO_ROOT = path.join(__dirname, "../../../..");

function read(rel: string): string {
  return readFileSync(path.join(REPO_ROOT, rel), "utf8");
}

/**
 * Strips comments from TypeScript sources before scanning. What ships to a
 * customer is the string literals, not the commentary — and the comments in
 * these very files name the removed claims in order to explain why they were
 * removed. Scanning raw source would make documenting the fix trip the guard
 * that enforces it.
 */
function copyOnly(source: string): string {
  return source
    .replace(/\/\*[\s\S]*?\*\//g, "")
    .replace(/^\s*\/\/.*$/gm, "");
}

/**
 * Pulls `FeatureStores: N` out of each plan block in the Go matrix. The
 * blocks appear in declaration order: Trial, Starter, Studio, Pro.
 */
function storeLimitsFromMatrix(): Record<string, number> {
  const src = read("services/marketplace-api/internal/plangate/matrix.go");
  const order = ["trial", "starter", "studio", "pro"];
  const found = [...src.matchAll(/FeatureStores:\s*(\d+)/g)].map((m) =>
    Number(m[1]),
  );
  expect(
    found.length,
    "expected four FeatureStores entries in matrix.go (trial/starter/studio/pro) — " +
      "if the matrix was restructured, this parser needs updating rather than deleting",
  ).toBe(order.length);
  return Object.fromEntries(order.map((k, i) => [k, found[i]!]));
}

test("public pricing copy quotes the storefront counts plangate enforces", () => {
  const limits = storeLimitsFromMatrix();
  const surfaces: ReadonlyArray<[string, string]> = [
    ["onboarding", read("apps/onboarding/components/marketing/Pricing.tsx")],
    ["admin", read("apps/admin/lib/copy/pricing.ts")],
  ];

  for (const [name, src] of surfaces) {
    for (const [plan, n] of Object.entries(limits)) {
      if (plan === "trial") continue; // not sold as a plan
      expect(
        src,
        `${name} pricing copy should quote "Up to ${n} storefronts/stores" for ${plan} ` +
          `(plangate FeatureStores = ${n}). A count that disagrees with the matrix is a ` +
          `promise the product will not keep — see #564.`,
      ).toMatch(new RegExp(`Up to ${n} store`));
    }
  }
});

/**
 * Limits that exist only in marketing copy. Each of these was on the live
 * admin page against a service that enforces no such cap — a merchant
 * hitting the advertised number would find nothing there.
 */
const INVENTED_LIMITS: ReadonlyArray<RegExp> = [
  /Up to \d+ products/i,
  /\d[\d,]* orders \/ month/i,
  /\d+ staff accounts/i,
  /Unlimited storefronts/i,
  /Dedicated infrastructure/i,
  /SLA-backed uptime/i,
  /Advanced fraud tooling/i,
];

test("no pricing surface advertises a limit or capability the service lacks", () => {
  const surfaces: ReadonlyArray<[string, string]> = [
    ["onboarding Pricing.tsx", copyOnly(read("apps/onboarding/components/marketing/Pricing.tsx"))],
    ["admin lib/copy/pricing.ts", copyOnly(read("apps/admin/lib/copy/pricing.ts"))],
    ["llms-full.txt", read("apps/onboarding/public/llms-full.txt")],
  ];

  for (const [name, src] of surfaces) {
    for (const pattern of INVENTED_LIMITS) {
      expect(
        src,
        `${name} advertises "${pattern.source}", which nothing in ` +
          `marketplace-api enforces or provides (#564). There is no product, ` +
          `order or staff cap in the schema, FeatureStores caps Pro at 10 rather ` +
          `than unlimited, and the cluster is a shared Cloud SQL micro instance.`,
      ).not.toMatch(pattern);
    }
  }
});

test("no pricing surface still sells webhooks or custom code injection (#544)", () => {
  const surfaces: ReadonlyArray<[string, string]> = [
    ["onboarding Pricing.tsx", copyOnly(read("apps/onboarding/components/marketing/Pricing.tsx"))],
    ["admin lib/copy/pricing.ts", copyOnly(read("apps/admin/lib/copy/pricing.ts"))],
    ["llms-full.txt", read("apps/onboarding/public/llms-full.txt")],
  ];

  for (const [name, src] of surfaces) {
    for (const claim of [/webhook/i, /code injection/i]) {
      expect(
        src,
        `${name} still advertises ${claim.source}. Neither exists server-side: ` +
          `no column stores a merchant callback URL, and FeatureCustomCodeInjection ` +
          `is a forward-compat hedge no handler reads (#544). Outbound webhooks are ` +
          `tracked in #562 — restore the copy when they ship, not before.`,
      ).not.toMatch(claim);
    }
  }
});
