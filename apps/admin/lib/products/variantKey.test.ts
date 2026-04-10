import { describe, it, expect } from "vitest";
import { buildVariantKey, parseVariantKey } from "./variantKey";

describe("buildVariantKey", () => {
  it("joins sorted option name=value pairs with |", () => {
    expect(
      buildVariantKey([
        { name: "Color", value: "Red" },
        { name: "Size", value: "M" },
      ]),
    ).toBe("Color=Red|Size=M");
  });

  it("is order-insensitive (sorts by option name)", () => {
    const a = buildVariantKey([
      { name: "Size", value: "M" },
      { name: "Color", value: "Red" },
    ]);
    const b = buildVariantKey([
      { name: "Color", value: "Red" },
      { name: "Size", value: "M" },
    ]);
    expect(a).toBe(b);
    expect(a).toBe("Color=Red|Size=M");
  });

  it("handles single option", () => {
    expect(buildVariantKey([{ name: "Size", value: "M" }])).toBe("Size=M");
  });

  it("handles empty input as empty string", () => {
    expect(buildVariantKey([])).toBe("");
  });

  it("parseVariantKey is the inverse of buildVariantKey", () => {
    const pairs = [
      { name: "Color", value: "Red" },
      { name: "Size", value: "M" },
    ];
    const key = buildVariantKey(pairs);
    expect(parseVariantKey(key)).toEqual([
      { name: "Color", value: "Red" },
      { name: "Size", value: "M" },
    ]);
  });
});
