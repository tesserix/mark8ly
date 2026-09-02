import { test, expect } from "@playwright/test";

// The live pricing table (components/marketing/Pricing.tsx) promised two
// features that plangate never enabled server-side — see #544:
//
//   - "Read-only API + webhooks" (Studio): the read-only API is real
//     (plangate.FeatureReadAPI -> internal/apikeys/*), and outbound webhooks
//     BECAME real in #562 — webhook_subscriptions.url stores exactly the
//     merchant-supplied callback URL this guard was originally written to
//     say did not exist. The blanket "no bullet may say webhook" assertion
//     was therefore stale and has been replaced (#586) by one that lets the
//     claim exist but pins the NUMBER to plangate's
//     FeatureWebhookSubscriptions, so the copy cannot drift from the cap the
//     server actually enforces. Its sibling guard in
//     pricing-surfaces-truth.spec.ts already retired the same assertion.
//   - "Custom code injection" (Pro): plangate.FeatureCustomCodeInjection's
//     own doc comment in matrix.go says no handler accepts arbitrary
//     <script> payloads today - it's a forward-compat hedge, referenced
//     nowhere outside the matrix and its tests.
//
// This spec pins the fix so neither claim can silently return. It does not
// re-verify every other bullet against the server (that audit lives in the
// #544 investigation) - it only guards the two bullets already proven false
// plus the basic shape of PLAN_META.
import { readFileSync } from "node:fs";
import path from "node:path";
import { PLAN_META } from "../../components/marketing/Pricing";

function allFeatures(): string[] {
  return PLAN_META.flatMap((plan) => plan.features);
}

test.describe("pricing copy must not promise unenforced features (#544)", () => {
  test("any webhook bullet quotes the cap plangate enforces (#586)", () => {
    const src = readFileSync(
      path.join(__dirname, "../../../..", "services/marketplace-api/internal/plangate/matrix.go"),
      "utf8",
    );
    // Blocks appear in declaration order: trial, starter, studio, pro.
    // Trial is not sold as a plan, so only the last three are asserted.
    const caps = [...src.matchAll(/FeatureWebhookSubscriptions:\s*(\d+)/g)].map((m) =>
      Number(m[1]),
    );
    expect(
      caps.length,
      "expected four FeatureWebhookSubscriptions entries in matrix.go " +
        "(trial/starter/studio/pro) — if the matrix was restructured, update " +
        "this parser rather than deleting the assertion",
    ).toBe(4);
    const byPlan: Record<string, number> = {
      starter: caps[1]!,
      studio: caps[2]!,
      pro: caps[3]!,
    };

    for (const plan of PLAN_META) {
      const bullets = plan.features.filter((f) => /webhook/i.test(f));
      // A plan need not advertise the cap — but if it does, the number must
      // be the one the server enforces at creation.
      for (const bullet of bullets) {
        const n = Number(bullet.match(/(\d+)/)?.[1]);
        expect(
          n,
          `${plan.id} advertises "${bullet}" but plangate enforces ` +
            `${byPlan[plan.id]} webhook subscriptions per store. A count that ` +
            `disagrees with the matrix is a promise the product will not keep.`,
        ).toBe(byPlan[plan.id]);
      }
    }
  });

  test("no bullet claims code injection", () => {
    const offenders = allFeatures().filter((f) => /code injection/i.test(f));
    expect(
      offenders,
      "Found a pricing bullet mentioning 'code injection'. " +
        "plangate.FeatureCustomCodeInjection's own doc comment " +
        "(internal/plangate/matrix.go) says no handler accepts arbitrary " +
        "<script> payloads today - it exists only as a forward-compat hedge " +
        "and is referenced nowhere outside the matrix and its tests. See " +
        "issue #544 - do not restore this bullet until a real write handler " +
        "enforces the feature.",
    ).toEqual([]);
  });

  test("every plan has a non-empty, unique feature list", () => {
    expect(PLAN_META.length).toBeGreaterThan(0);
    for (const plan of PLAN_META) {
      expect(plan.features.length, `${plan.id} has no features`).toBeGreaterThan(0);
      expect(
        new Set(plan.features).size,
        `${plan.id} has duplicate feature bullets`,
      ).toBe(plan.features.length);
    }
  });
});
