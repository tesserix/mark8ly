import { describe, it, expect } from "vitest";
import {
  generateVariants,
  type OptionDraft,
  type VariantDraft,
} from "./generateVariants";

const baseDefaults = { price: "19.99", sku: "", stock: 0, weight: 0 };

describe("generateVariants", () => {
  it("returns empty array when options are empty", () => {
    expect(generateVariants([], [], baseDefaults)).toEqual({
      variants: [],
      removedIds: [],
    });
  });

  it("generates cartesian product for two options", () => {
    const options: OptionDraft[] = [
      { name: "Color", values: ["Red", "Blue"] },
      { name: "Size", values: ["S", "M"] },
    ];
    const result = generateVariants(options, [], baseDefaults);
    expect(result.variants).toHaveLength(4);
    const keys = result.variants.map((v) => v.key).sort();
    expect(keys).toEqual([
      "Color=Blue|Size=M",
      "Color=Blue|Size=S",
      "Color=Red|Size=M",
      "Color=Red|Size=S",
    ]);
  });

  it("preserves existing variant data by matching key", () => {
    const options: OptionDraft[] = [{ name: "Size", values: ["S", "M"] }];
    const existing: VariantDraft[] = [
      {
        key: "Size=S",
        id: "var-1",
        price: "29.99",
        sku: "SHIRT-S",
        stock: 42,
        weight: 0.2,
      },
    ];
    const result = generateVariants(options, existing, baseDefaults);
    const small = result.variants.find((v) => v.key === "Size=S");
    expect(small?.id).toBe("var-1");
    expect(small?.price).toBe("29.99");
    expect(small?.sku).toBe("SHIRT-S");
    expect(small?.stock).toBe(42);
  });

  it("drops unmatched existing variants into removedIds", () => {
    const options: OptionDraft[] = [{ name: "Size", values: ["M"] }];
    const existing: VariantDraft[] = [
      { key: "Size=S", id: "var-1", ...baseDefaults },
      { key: "Size=M", id: "var-2", ...baseDefaults },
    ];
    const result = generateVariants(options, existing, baseDefaults);
    expect(result.variants.map((v) => v.key)).toEqual(["Size=M"]);
    expect(result.removedIds).toEqual(["var-1"]);
  });

  it("new variants get defaults from baseDefaults", () => {
    const options: OptionDraft[] = [{ name: "Size", values: ["L"] }];
    const result = generateVariants(options, [], baseDefaults);
    expect(result.variants[0]).toMatchObject({
      key: "Size=L",
      price: "19.99",
      stock: 0,
      sku: "",
    });
    expect(result.variants[0]?.id).toBeUndefined();
  });

  it("renaming an option value does NOT preserve data", () => {
    const options: OptionDraft[] = [{ name: "Size", values: ["Medium"] }];
    const existing: VariantDraft[] = [
      {
        key: "Size=M",
        id: "var-1",
        price: "29.99",
        sku: "X",
        stock: 5,
        weight: 0,
      },
    ];
    const result = generateVariants(options, existing, baseDefaults);
    expect(result.variants[0]?.key).toBe("Size=Medium");
    expect(result.variants[0]?.id).toBeUndefined();
    expect(result.removedIds).toEqual(["var-1"]);
  });

  it("reordering options does NOT orphan variants (sorted key is stable)", () => {
    const existing: VariantDraft[] = [
      {
        key: "Color=Red|Size=M",
        id: "var-1",
        price: "19.99",
        sku: "R-M",
        stock: 3,
        weight: 0,
      },
    ];
    const options: OptionDraft[] = [
      { name: "Size", values: ["M"] },
      { name: "Color", values: ["Red"] },
    ];
    const result = generateVariants(options, existing, baseDefaults);
    expect(result.variants[0]?.id).toBe("var-1");
    expect(result.variants[0]?.sku).toBe("R-M");
    expect(result.removedIds).toEqual([]);
  });

  it("enforces 500-variant cap by throwing", () => {
    const options: OptionDraft[] = [
      { name: "A", values: Array.from({ length: 11 }, (_, i) => `a${i}`) },
      { name: "B", values: Array.from({ length: 11 }, (_, i) => `b${i}`) },
      { name: "C", values: Array.from({ length: 5 }, (_, i) => `c${i}`) },
    ];
    expect(() => generateVariants(options, [], baseDefaults)).toThrow(
      /too many variants/i,
    );
  });

  it("500 combinations is allowed (exactly at the cap)", () => {
    const options: OptionDraft[] = [
      { name: "A", values: Array.from({ length: 10 }, (_, i) => `a${i}`) },
      { name: "B", values: Array.from({ length: 50 }, (_, i) => `b${i}`) },
    ];
    const result = generateVariants(options, [], baseDefaults);
    expect(result.variants).toHaveLength(500);
  });
});

// 8 of the-bondi-store's 12 products have variants with no product_options
// rows. Their variants are keyed by id (`id:<uuid>`, see ProductForm) since
// there are no option values to compose a key from. Adding a first option
// therefore matches none of them, and every one is returned for deletion.
// This is the behaviour the Options section's copy warns about; the test
// exists so the warning cannot quietly stop being true.
describe("generateVariants — adding a first option to option-less variants", () => {
  it("returns every existing variant for removal, because no key can match", () => {
    const existing = [
      { id: "v1", key: "id:v1", price: "189", sku: "ROBE-S", stock: 6, weight: 0 },
      { id: "v2", key: "id:v2", price: "189", sku: "ROBE-M", stock: 4, weight: 0 },
    ];

    const { variants, removedIds } = generateVariants(
      [{ name: "Size", values: ["S", "M"] }],
      existing as never,
      { price: "189", sku: "", stock: 0, weight: 0 },
    );

    expect(variants.map((v) => v.key)).toEqual(["Size=S", "Size=M"]);
    expect(removedIds.sort()).toEqual(["v1", "v2"]);
  });
});
