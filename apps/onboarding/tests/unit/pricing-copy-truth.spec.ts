import { test, expect } from "@playwright/test";

// The live pricing table (components/marketing/Pricing.tsx) promised two
// features that plangate never enabled server-side — see #544:
//
//   - "Read-only API + webhooks" (Studio): the read-only API is real
//     (plangate.FeatureReadAPI -> internal/apikeys/*), but nothing in the
//     schema stores a merchant-supplied webhook/callback URL, so outbound
//     webhook delivery cannot exist. webhook_events / stripe_webhook_events
//     are inbound idempotency tables, not evidence of the feature.
//   - "Custom code injection" (Pro): plangate.FeatureCustomCodeInjection's
//     own doc comment in matrix.go says no handler accepts arbitrary
//     <script> payloads today - it's a forward-compat hedge, referenced
//     nowhere outside the matrix and its tests.
//
// This spec pins the fix so neither claim can silently return. It does not
// re-verify every other bullet against the server (that audit lives in the
// #544 investigation) - it only guards the two bullets already proven false
// plus the basic shape of PLAN_META.
import { PLAN_META } from "../../components/marketing/Pricing";

function allFeatures(): string[] {
  return PLAN_META.flatMap((plan) => plan.features);
}

test.describe("pricing copy must not promise unenforced features (#544)", () => {
  test("no bullet claims webhooks", () => {
    const offenders = allFeatures().filter((f) => /webhook/i.test(f));
    expect(
      offenders,
      "Found a pricing bullet mentioning 'webhook'. Outbound webhook " +
        "delivery does not exist server-side: webhook_events and " +
        "stripe_webhook_events are inbound idempotency tables, and no " +
        "column anywhere stores a merchant-supplied callback URL. See " +
        "issue #544 - narrow this bullet back to what plangate.FeatureReadAPI " +
        "actually grants ('Read-only API'), don't re-add webhooks.",
    ).toEqual([]);
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
