import { describe, expect, it } from "vitest";

import {
  DEFAULT_PARCEL_WEIGHT_GRAMS,
  fallbackParcelWeight,
} from "./parcel-weight";
import type { ShippingOption } from "@/lib/api/checkout-api";

const opt = (w?: number) =>
  ({
    carrier: "shipengine",
    services: ["standard"],
    supported_countries: ["AU"],
    default_parcel_weight_grams: w,
  }) as unknown as ShippingOption;

describe("fallbackParcelWeight", () => {
  it("uses the store's configured weight", () => {
    expect(fallbackParcelWeight([opt(250)])).toBe(250);
  });

  // An older API build omits the field entirely; the previous behaviour
  // must still apply rather than quoting on undefined.
  it("falls back to 500 when the field is absent", () => {
    expect(fallbackParcelWeight([opt(undefined)])).toBe(
      DEFAULT_PARCEL_WEIGHT_GRAMS,
    );
  });

  it("falls back to 500 when there are no options at all", () => {
    expect(fallbackParcelWeight([])).toBe(DEFAULT_PARCEL_WEIGHT_GRAMS);
  });

  // A zero or negative weight would make the carrier request nonsense.
  it("rejects non-positive and non-finite values", () => {
    expect(fallbackParcelWeight([opt(0)])).toBe(DEFAULT_PARCEL_WEIGHT_GRAMS);
    expect(fallbackParcelWeight([opt(-100)])).toBe(DEFAULT_PARCEL_WEIGHT_GRAMS);
    expect(fallbackParcelWeight([opt(Number.NaN)])).toBe(
      DEFAULT_PARCEL_WEIGHT_GRAMS,
    );
  });

  it("skips an unusable value and takes the next usable one", () => {
    expect(fallbackParcelWeight([opt(0), opt(300)])).toBe(300);
  });
});
