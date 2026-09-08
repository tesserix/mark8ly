import { readFileSync } from "node:fs";
import path from "node:path";

import { describe, expect, it } from "vitest";

import { pricingCopy } from "@/lib/copy/pricing";

/**
 * The guard `lib/copy/pricing.ts` has claimed since it was written.
 *
 * Its header said "pricing-copy.test.ts pins the counts against the matrix so
 * a divergence fails the build" — and no such file existed. Three separate
 * incidents were found by people reading the file while that sentence told
 * reviewers CI was watching:
 *
 *   #413 — prices quoted that the billing catalog would not charge
 *   #564 — invented product/order/staff caps, wrong storefront counts, and
 *          "SLA-backed uptime" on a shared Cloud SQL micro instance
 *   #838 — "SSO (SAML / OIDC)" advertised against a route answering 501
 *
 * This is that file. It reads `plangate/matrix.go` — the actual source of
 * truth, per its own package comment — rather than a copy of it, for the
 * reason the marketing site's equivalent guard
 * (`apps/onboarding/tests/unit/pricing-copy-truth.spec.ts`) gives: a second
 * transcription of the matrix is just a second thing to drift.
 */

const MATRIX_PATH = path.join(
  __dirname,
  "../../../../../..",
  "services/marketplace-api/internal/plangate/matrix.go",
);

/** Sentinels from matrix.go. Disabled is the zero value, so an unset cell
 *  reads as off — which is what makes an absent row safe to treat as absent. */
const DISABLED = 0;
const UNLIMITED = -1;
const NEGOTIATED = -2;

type PlanId = "starter" | "studio" | "pro";

/**
 * Parses matrix.go into `{ plan: { feature: number } }`.
 *
 * Keyed on the PLAN BLOCK, not on declaration order. The onboarding guard
 * matches every `FeatureWebhookSubscriptions:` in the file and indexes the
 * results positionally, which silently reads the wrong plan the moment a
 * block is reordered or a fifth plan appears. Splitting on
 * `subscription.PlanX: {` costs a few lines and cannot mis-attribute a value.
 *
 * Sentinel names are resolved to their numeric values so a caller compares
 * numbers throughout.
 */
function parseMatrix(): Record<PlanId, Record<string, number>> {
  const src = readFileSync(MATRIX_PATH, "utf8");
  const out = {} as Record<PlanId, Record<string, number>>;

  for (const [plan, goName] of [
    ["starter", "PlanStarter"],
    ["studio", "PlanStudio"],
    ["pro", "PlanPro"],
  ] as const) {
    const start = src.indexOf(`subscription.${goName}: {`);
    expect(
      start,
      `matrix.go no longer declares subscription.${goName} — update this parser rather than deleting the assertions`,
    ).toBeGreaterThan(-1);

    // The block ends at the next plan declaration, or at the end of the map.
    const rest = src.slice(start + 1);
    const nextPlan = rest.search(/\n\tsubscription\.Plan\w+: \{/);
    const block = nextPlan === -1 ? rest : rest.slice(0, nextPlan);

    const cells: Record<string, number> = {};
    for (const m of block.matchAll(/Feature(\w+):\s*([A-Za-z_0-9]+)/g)) {
      const raw = m[2]!;
      const value =
        raw === "Disabled"
          ? DISABLED
          : raw === "Unlimited"
            ? UNLIMITED
            : raw === "Negotiated"
              ? NEGOTIATED
              : Number(raw.replace(/_/g, ""));
      if (!Number.isNaN(value)) cells[m[1]!] = value;
    }
    out[plan] = cells;
  }
  return out;
}

const MATRIX = parseMatrix();

/** The bullets a plan advertises, lower-cased for matching. */
function bullets(plan: PlanId): string[] {
  const entry = pricingCopy.plans.find((p) => p.id === plan);
  expect(entry, `pricing copy has no plan "${plan}"`).toBeDefined();
  return entry!.features.map((f) => f.toLowerCase());
}

/** The first integer in a bullet matching `pattern`, or null if no such
 *  bullet is advertised. A plan need not mention a limit — but if it does,
 *  the number has to be the enforced one. */
function claimedNumber(plan: PlanId, pattern: RegExp): number | null {
  const bullet = bullets(plan).find((b) => pattern.test(b));
  if (bullet === undefined) return null;
  const digits = bullet.replace(/,/g, "").match(/\d+/);
  return digits ? Number(digits[0]) : null;
}

const PLANS: PlanId[] = ["starter", "studio", "pro"];

describe("the parser reads matrix.go, not a copy of it", () => {
  // If the parser silently matched nothing, every assertion below would pass
  // vacuously — the failure mode this whole file exists to prevent.
  it.each(PLANS)("%s resolves a non-trivial number of matrix cells", (plan) => {
    expect(Object.keys(MATRIX[plan]).length).toBeGreaterThan(15);
  });

  it("reads values that differ per plan, so it is not returning one block for all three", () => {
    expect(MATRIX.starter.Stores).not.toBe(MATRIX.pro.Stores);
  });

  it("resolves sentinels rather than leaving them as NaN", () => {
    expect(MATRIX.pro.ImagesPerProduct).toBe(UNLIMITED);
    expect(MATRIX.studio.SSO).toBe(DISABLED);
  });
});

describe("numeric bullets quote the limit plangate enforces", () => {
  it.each(PLANS)("%s storefront count", (plan) => {
    const claimed = claimedNumber(plan, /storefront/);
    if (claimed === null) return;
    expect(claimed, `${plan} advertises storefronts`).toBe(MATRIX[plan].Stores);
  });

  it.each(PLANS)("%s webhook endpoint count", (plan) => {
    const claimed = claimedNumber(plan, /webhook/);
    if (claimed === null) return;
    expect(claimed).toBe(MATRIX[plan].WebhookSubscriptions);
  });

  it.each(PLANS)("%s campaign email allowance", (plan) => {
    const claimed = claimedNumber(plan, /campaign email/);
    if (claimed === null) return;
    expect(claimed).toBe(MATRIX[plan].CampaignEmailsPerMonth);
  });

  it.each(PLANS)("%s images-per-product cap", (plan) => {
    const bullet = bullets(plan).find((b) => /image/.test(b));
    if (bullet === undefined) return;
    const cap = MATRIX[plan].ImagesPerProduct!;
    if (cap === UNLIMITED) {
      // "Unlimited images" is the only honest phrasing of -1; a number here
      // would invent a cap the server does not apply.
      expect(bullet).toMatch(/unlimited/);
    } else {
      expect(claimedNumber(plan, /image/)).toBe(cap);
    }
  });
});

describe("capability bullets appear only where the matrix enables them", () => {
  // The #838 defect, generalised: a bullet naming a gated capability on a
  // plan where plangate has it Disabled is a promise the server refuses.
  const CAPABILITIES: ReadonlyArray<{ what: string; feature: string; match: RegExp }> = [
    { what: "SSO", feature: "SSO", match: /\bsso\b/ },
    { what: "full read/write API", feature: "FullAPI", match: /read\/write api/ },
    { what: "custom CSS", feature: "CustomCSS", match: /custom css/ },
    { what: "custom domain", feature: "CustomDomain", match: /own domain|custom domain/ },
    { what: "announcement bar", feature: "AnnouncementBar", match: /announcement bar/ },
    { what: "uptime SLA", feature: "UptimeSLA", match: /sla|uptime/ },
    { what: "white-label app", feature: "WhiteLabelApp", match: /white[- ]label app/ },
  ];

  it.each(
    PLANS.flatMap((plan) => CAPABILITIES.map((c) => ({ plan, ...c }))),
  )("$plan does not advertise $what unless plangate grants it", ({ plan, feature, match }) => {
    const advertised = bullets(plan).some((b) => match.test(b));
    if (!advertised) return;
    expect(
      MATRIX[plan][feature],
      `${plan} advertises ${feature} but matrix.go has it Disabled`,
    ).not.toBe(DISABLED);
  });

  // The specific claim #838 was opened for. SAML is not implemented — both
  // SSO routes answer 501 — so the bullet must not promise a protocol the
  // server refuses, whatever the matrix says about SSO in general.
  it("never advertises SAML while the SSO routes answer 501 for it", () => {
    for (const plan of PLANS) {
      for (const bullet of bullets(plan)) {
        expect(bullet, `${plan} bullet must not claim SAML`).not.toMatch(/saml/);
      }
    }
  });

  // #564: "SLA-backed uptime" on a shared Cloud SQL micro instance.
  // FeatureUptimeSLA is Disabled on every sold plan (it is a Pro+App add-on),
  // so no plan bullet may claim one.
  it("claims no uptime SLA on any plan, matching the matrix", () => {
    for (const plan of PLANS) {
      expect(MATRIX[plan].UptimeSLA, `matrix changed for ${plan}`).toBe(DISABLED);
      expect(bullets(plan).some((b) => /\bsla\b/.test(b))).toBe(false);
    }
  });
});

describe("audit retention", () => {
  // Days in the matrix, human phrasing in the copy — so this asserts the
  // MEANING rather than the digits, which is where "12-month" vs 365 lives.
  it.each(PLANS)("%s audit-log claim matches the retention plangate applies", (plan) => {
    const bullet = bullets(plan).find((b) => /audit/.test(b));
    if (bullet === undefined) return;
    const days = MATRIX[plan].AuditRetentionDays!;
    if (days === UNLIMITED) {
      expect(bullet).toMatch(/forever|unlimited/);
    } else {
      const months = claimedNumber(plan, /audit/);
      if (months !== null && /month/.test(bullet)) {
        expect(months * 30, `${plan} audit months vs ${days} days`).toBeGreaterThanOrEqual(
          days - 31,
        );
        expect(months * 30).toBeLessThanOrEqual(days + 31);
      }
    }
  });
});
