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

/**
 * #544 originally caught both "webhooks" and "code injection" as sold
 * claims with nothing behind them. Outbound webhooks shipped in #562, so
 * that half of the guard is gone — see the webhook assertion's history in
 * git blame if you're looking for it.
 *
 * Custom code injection stays covered here on its own, and deliberately:
 * it isn't merely unbuilt (the way webhooks briefly were), it's excluded by
 * design. Arbitrary merchant-authored `<script>` in a storefront is XSS
 * against that storefront's customers, and shipping it would permanently
 * weaken the storefront CSP. Nothing schedules it, so this assertion isn't
 * stale — it is guarding against reintroducing a claim the product should
 * never make. If webhooks copy is ever touched again, remember it is not a
 * tier differentiator (available on every plan), so it does not belong as
 * a Studio-only bullet even though this guard no longer blocks it.
 */
test("no pricing surface still sells custom code injection (#544)", () => {
  const surfaces: ReadonlyArray<[string, string]> = [
    ["onboarding Pricing.tsx", copyOnly(read("apps/onboarding/components/marketing/Pricing.tsx"))],
    ["admin lib/copy/pricing.ts", copyOnly(read("apps/admin/lib/copy/pricing.ts"))],
    ["llms-full.txt", read("apps/onboarding/public/llms-full.txt")],
  ];

  for (const [name, src] of surfaces) {
    for (const claim of [/code injection/i]) {
      expect(
        src,
        `${name} still advertises ${claim.source}. FeatureCustomCodeInjection is a ` +
          `forward-compat hedge no handler reads (#544), and it is excluded by design, ` +
          `not merely unbuilt: arbitrary merchant <script> in a storefront is XSS ` +
          `against that storefront's customers and would permanently weaken the ` +
          `storefront CSP. Unlike webhooks (#562, shipped), there is no ticket to ` +
          `restore this copy — it should not come back.`,
      ).not.toMatch(claim);
    }
  }
});

/**
 * #838. Pro was sold as `SSO (SAML / OIDC)` on all three surfaces. OIDC is
 * real and self-serve since #839; SAML is not, and does not merely lack an
 * implementation — `loginSAML` and its callback both answer **501**
 * deliberately (`internal/handlers/public/sso_login.go:265,303`).
 *
 * #839 narrowed the two TypeScript surfaces to OpenID Connect and left
 * `llms-full.txt` still advertising SAML. That is the worst surface to miss
 * it on: it is the machine-readable pricing document, so the corrected claim
 * reached humans while the stale one kept reaching every agent and crawler,
 * the audience least placed to notice the contradiction.
 *
 * The positive half of this test matters as much as the negative one. Deleting
 * the bullet outright would also pass a SAML check while quietly dropping a
 * capability Pro genuinely has — understating is drift too, just in the
 * direction nobody complains about.
 */
test("pricing surfaces sell the SSO protocol that works, and only that one (#838)", () => {
  const surfaces: ReadonlyArray<[string, string]> = [
    ["onboarding Pricing.tsx", copyOnly(read("apps/onboarding/components/marketing/Pricing.tsx"))],
    ["admin lib/copy/pricing.ts", copyOnly(read("apps/admin/lib/copy/pricing.ts"))],
    ["llms-full.txt", read("apps/onboarding/public/llms-full.txt")],
  ];

  for (const [name, src] of surfaces) {
    expect(
      src,
      `${name} advertises SAML. Both SSO routes answer 501 for it by design ` +
        `(#820, #838) and there is no SP, so this sells a protocol the product ` +
        `refuses. Narrow it to OpenID Connect, which is real and configurable ` +
        `at Settings -> Single sign-on.`,
    ).not.toMatch(/SAML/i);

    expect(
      src,
      `${name} mentions SSO without naming OpenID Connect. Pro really does ` +
        `include self-serve OIDC (#839) — a bullet that says only "SSO", or no ` +
        `bullet at all, understates the plan and leaves the next editor guessing ` +
        `which protocols are meant. Name the one that works.`,
    ).toMatch(/OpenID Connect/i);
  }
});
