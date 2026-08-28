import { test, expect } from "@playwright/test";

// The marketing Pricing section (components/marketing/Pricing.tsx) and the
// homepage's JSON-LD (app/page.tsx's resolvePricedPlan) both render prices
// via getPlanPrice/getAddOnPrice from the shared pricing package. This spec
// exercises that resolver directly rather than rendering the React
// component (no DOM/browser harness exists in this unit-test config — see
// playwright.unit.config.ts), proving the exact defence Pricing.tsx relies
// on: a visitor currency with no price row must resolve to a USD amount
// labelled USD, never the visitor's raw currency code over a USD number.
//
// Imported by relative path into packages/ui/src rather than via the
// `@repo/ui/subscription` package export map — this unit-test config has no
// bundler step, and esbuild-based TS loaders generally do not transform
// .ts sources reached through node_modules.
import {
  SHARED_PRICING_CATALOGUE,
  getPlanPrice,
  getAddOnPrice,
} from "../../../../packages/ui/src/subscription/pricing-data";
import {
  normalizeCurrency,
  PRICEABLE_CURRENCIES,
} from "../../../../packages/ui/src/subscription/country-map";

test.describe("pricing currency resolution (defence in depth)", () => {
  test("a visitor currency with no price row resolves to USD, labelled USD", () => {
    const starter = SHARED_PRICING_CATALOGUE.plans.find((p) => p.id === "starter")!;
    // THB has no row anywhere in the catalogue (PPP currencies are
    // deliberately excluded — see pricing-data.ts's doc comment).
    const resolved = getPlanPrice(starter, "THB");

    expect(resolved.currency).toBe("USD");
    expect(resolved.price.monthly).toBe(1900);
    expect(resolved.price.annual).toBe(18200);
  });

  test("resolves to the requested currency when a row exists (NZD)", () => {
    const starter = SHARED_PRICING_CATALOGUE.plans.find((p) => p.id === "starter")!;
    const resolved = getPlanPrice(starter, "NZD");

    expect(resolved.currency).toBe("NZD");
    expect(resolved.price.monthly).toBe(2900);
  });

  test("Pro+App add-on resolution follows the same fallback rule", () => {
    const resolved = getAddOnPrice(SHARED_PRICING_CATALOGUE.proApp, "AED");
    expect(resolved.currency).toBe("USD");
    expect(resolved.price.monthly).toBe(19900);
  });

  test("normalizeCurrency rejects a geo-recognized but unpriceable currency", () => {
    // TH maps to THB in COUNTRY_TO_CURRENCY (geo-targeting correctly knows
    // Thailand uses THB) but the pricing table has no THB row, so the
    // cookie value must not survive normalization.
    expect(normalizeCurrency("THB")).toBe("USD");
    expect(PRICEABLE_CURRENCIES.has("THB" as never)).toBe(false);
  });

  test("normalizeCurrency accepts NZD", () => {
    expect(normalizeCurrency("NZD")).toBe("NZD");
    expect(PRICEABLE_CURRENCIES.has("NZD" as never)).toBe(true);
  });

  test("Starter USD headline matches billing: $19/mo, $182/yr", () => {
    const starter = SHARED_PRICING_CATALOGUE.plans.find((p) => p.id === "starter")!;
    expect(starter.prices.USD?.monthly).toBe(1900);
    expect(starter.prices.USD?.annual).toBe(18200);
  });
});
