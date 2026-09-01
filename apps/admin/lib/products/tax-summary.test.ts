import { describe, it, expect } from "vitest";

import { taxSummary } from "./tax-summary";

// The line has to earn the collapse: a merchant must be able to tell,
// without opening the section, whether this product does anything
// unusual. A summary that said "Tax" and nothing else would just be a
// worse tab.
describe("taxSummary", () => {
  it("says plainly when nothing is overridden", () => {
    const s = taxSummary({ taxCode: "", taxCategory: undefined, taxRateOverride: "" }, "flat_rate");
    expect(s.text).toBe("Using the store default");
    expect(s.isOverridden).toBe(false);
  });

  it("treats whitespace as unset rather than as an override", () => {
    const s = taxSummary({ taxCode: "   ", taxCategory: undefined, taxRateOverride: "  " }, "flat_rate");
    expect(s.isOverridden).toBe(false);
  });

  it("names an exemption first, because it means no tax at all", () => {
    const s = taxSummary({ taxCode: "", taxCategory: "exempt", taxRateOverride: "" }, "flat_rate");
    expect(s.text).toBe("Tax exempt");
    expect(s.isOverridden).toBe(true);
  });

  it("calls an HSN code by its name in India", () => {
    const s = taxSummary({ taxCode: "6109", taxCategory: undefined, taxRateOverride: "12" }, "india_gst");
    expect(s.text).toBe("HSN 6109 · 12%");
  });

  it("uses a neutral word for a code elsewhere", () => {
    const s = taxSummary({ taxCode: "20010", taxCategory: undefined, taxRateOverride: "" }, "taxjar");
    expect(s.text).toBe("Code 20010");
  });

  it("does not treat the standard category as an override", () => {
    const s = taxSummary({ taxCode: "", taxCategory: "standard", taxRateOverride: "" }, "flat_rate");
    expect(s.isOverridden).toBe(false);
  });

  it("combines what is set, most significant first", () => {
    const s = taxSummary(
      { taxCode: "6109", taxCategory: "zero_rated", taxRateOverride: "0" },
      "india_gst",
    );
    expect(s.text).toBe("Zero-rated · HSN 6109 · 0%");
  });

  // The admin does not have the store's resolved rate here. Printing a
  // number we have not been given would be worse than saying nothing.
  it("never invents the store's actual rate", () => {
    const s = taxSummary({ taxCode: "", taxCategory: undefined, taxRateOverride: "" }, "india_gst");
    expect(s.text).not.toMatch(/\d/);
  });
});
