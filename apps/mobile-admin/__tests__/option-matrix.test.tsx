import { buildOptionMatrix, OptionMatrixError } from "@/lib/option-matrix";
import { deriveVariantSku } from "@/lib/sku";

// Minimal ProductDetail-shaped fixtures. Only the fields the helper reads.
const singleVariantProduct = {
  id: "p1",
  title: "Linen Shirt",
  options: [],
  variants: [
    {
      id: "v1",
      sku: "TBS-LS",
      price: 149,
      inventory_quantity: 5,
      currency_code: "AUD",
      option_values: [],
      position: 0,
    },
  ],
} as never;

describe("buildOptionMatrix — single variant + one new axis", () => {
  it("maps the existing variant onto the FIRST value and creates the rest empty", () => {
    const { options, variants } = buildOptionMatrix(singleVariantProduct, [
      { name: "Size", values: ["S", "M", "L"] },
    ]);
    expect(options).toEqual([{ name: "Size", values: ["S", "M", "L"] }]);
    expect(variants).toHaveLength(3);

    // First value reuses the existing variant id + its price/stock/sku.
    const s = variants.find((v) => v.option_values[0]!.value === "S")!;
    expect(s.id).toBe("v1");
    expect(s.price).toBe(149);
    expect(s.inventory_quantity).toBe(5);
    expect(s.sku).toBe("TBS-LS");
    expect(s.currency_code).toBe("AUD");
    expect(s.option_values).toEqual([{ option_name: "Size", value: "S" }]);

    // New combinations: no id, inherit price/currency, 0 stock, derived sku.
    const m = variants.find((v) => v.option_values[0]!.value === "M")!;
    expect(m.id).toBeUndefined();
    expect(m.price).toBe(149);
    expect(m.inventory_quantity).toBe(0);
    expect(m.currency_code).toBe("AUD");
    expect(m.sku).toBe(deriveVariantSku("Linen Shirt", ["M"]));
    expect(m.option_values).toEqual([{ option_name: "Size", value: "M" }]);

    const l = variants.find((v) => v.option_values[0]!.value === "L")!;
    expect(l.id).toBeUndefined();
    expect(l.sku).toBe(deriveVariantSku("Linen Shirt", ["L"]));
  });

  it("🔴 never drops the existing variant's id when its tuple survives", () => {
    const { variants } = buildOptionMatrix(singleVariantProduct, [{ name: "Size", values: ["S", "M"] }]);
    const withId = variants.filter((v) => v.id === "v1");
    expect(withId).toHaveLength(1); // exactly one carries the real id → no soft-delete
  });

  it("does not mutate the input product or its arrays", () => {
    const before = JSON.parse(JSON.stringify(singleVariantProduct));
    buildOptionMatrix(singleVariantProduct, [{ name: "Size", values: ["S", "M", "L"] }]);
    expect(singleVariantProduct).toEqual(before);
  });
});

describe("buildOptionMatrix — multi-axis Cartesian", () => {
  // Deliberately out of order on two axes so a naive implementation can't
  // pass by accident: variants[] is in REVERSE position order (vm before
  // vs), and the two existing variants have DIFFERENT prices, so inherited
  // price for new combinations must come from the position-lowest variant
  // (vs, price 20) — not variants[0] (vm, price 25) and not "whichever is
  // first in the array".
  const sizedProduct = {
    id: "p2",
    title: "Tee",
    options: [
      {
        id: "o1",
        name: "Size",
        position: 0,
        values: [
          { id: "m", value: "M", position: 1 },
          { id: "s", value: "S", position: 0 },
        ],
      },
    ],
    variants: [
      {
        id: "vm",
        sku: "T-M",
        price: 25,
        inventory_quantity: 4,
        currency_code: "AUD",
        option_values: [{ option_name: "Size", option_value_id: "m", value: "M" }],
        position: 1,
      },
      {
        id: "vs",
        sku: "T-S",
        price: 20,
        inventory_quantity: 3,
        currency_code: "AUD",
        option_values: [{ option_name: "Size", option_value_id: "s", value: "S" }],
        position: 0,
      },
    ],
  } as never;

  it("expands Size×Colour to 4, preserving both existing variants by tuple", () => {
    const { variants } = buildOptionMatrix(sizedProduct, [
      { name: "Size", values: ["S", "M"] },
      { name: "Colour", values: ["Red", "Blue"] },
    ]);
    expect(variants).toHaveLength(4);

    // The two existing variants keep their ids on their (Size, first-Colour)
    // tuples: vs -> (S, Red), vm -> (M, Red).
    const vs = variants.find((v) => v.id === "vs")!;
    expect(vs.option_values).toEqual([
      { option_name: "Size", value: "S" },
      { option_name: "Colour", value: "Red" },
    ]);
    expect(vs.sku).toBe("T-S");
    expect(vs.inventory_quantity).toBe(3);

    const vm = variants.find((v) => v.id === "vm")!;
    expect(vm.option_values).toEqual([
      { option_name: "Size", value: "M" },
      { option_name: "Colour", value: "Red" },
    ]);
    expect(vm.sku).toBe("T-M");
    expect(vm.inventory_quantity).toBe(4);

    // Every variant carries a full 2-axis tuple.
    for (const v of variants) expect(v.option_values).toHaveLength(2);

    // No id appears twice (no soft-delete via duplicate reuse) and both
    // existing ids are present exactly once each.
    const ids = variants.map((v) => v.id).filter(Boolean);
    expect(new Set(ids).size).toBe(ids.length);
    expect(ids.sort()).toEqual(["vm", "vs"]);

    // Brand-new combinations (S,Blue) and (M,Blue): no id, inherit price
    // from the POSITION-lowest existing variant (vs, price 20) — not
    // variants[0] (vm, price 25).
    const sBlue = variants.find(
      (v) => v.option_values[0]!.value === "S" && v.option_values[1]!.value === "Blue",
    )!;
    expect(sBlue.id).toBeUndefined();
    expect(sBlue.price).toBe(20);
    expect(sBlue.inventory_quantity).toBe(0);
    expect(sBlue.sku).toBe(deriveVariantSku("Tee", ["S", "Blue"]));

    const mBlue = variants.find(
      (v) => v.option_values[0]!.value === "M" && v.option_values[1]!.value === "Blue",
    )!;
    expect(mBlue.id).toBeUndefined();
    expect(mBlue.price).toBe(20);
  });
});

describe("buildOptionMatrix — fail loud", () => {
  it("throws OptionMatrixError when no options are given", () => {
    expect(() => buildOptionMatrix(singleVariantProduct, [])).toThrow(OptionMatrixError);
  });

  it("throws OptionMatrixError rather than guessing when values are empty", () => {
    expect(() => buildOptionMatrix(singleVariantProduct, [{ name: "Size", values: [] }])).toThrow(
      OptionMatrixError,
    );
  });

  it("throws when an axis has duplicate values (would emit a duplicated existing id)", () => {
    // Without the guard, cartesian(["S","S","M"]) yields two (S) tuples, both
    // resolving to v1 → the matrix would carry id:"v1" TWICE, silently
    // corrupting a full-desired-matrix PATCH. Must reject at the source.
    expect(() =>
      buildOptionMatrix(singleVariantProduct, [{ name: "Size", values: ["S", "S", "M"] }]),
    ).toThrow(OptionMatrixError);
  });

  it("throws when two options share a name (duplicate tuple keys)", () => {
    expect(() =>
      buildOptionMatrix(singleVariantProduct, [
        { name: "Size", values: ["S", "M"] },
        { name: "Size", values: ["Red", "Blue"] },
      ]),
    ).toThrow(OptionMatrixError);
  });

  it("throws when two existing variants collide onto the same desired tuple", () => {
    // Two existing variants distinguished only by an axis ("Colour") that is
    // being dropped entirely — both collapse onto Size=S.
    const collidingProduct = {
      id: "p3",
      title: "Mug",
      options: [],
      variants: [
        {
          id: "red",
          sku: "MUG-S-RED",
          price: 10,
          inventory_quantity: 1,
          currency_code: "AUD",
          option_values: [
            { option_name: "Size", option_value_id: "s", value: "S" },
            { option_name: "Colour", option_value_id: "red", value: "Red" },
          ],
          position: 0,
        },
        {
          id: "blue",
          sku: "MUG-S-BLUE",
          price: 10,
          inventory_quantity: 2,
          currency_code: "AUD",
          option_values: [
            { option_name: "Size", option_value_id: "s", value: "S" },
            { option_name: "Colour", option_value_id: "blue", value: "Blue" },
          ],
          position: 1,
        },
      ],
    } as never;

    expect(() => buildOptionMatrix(collidingProduct, [{ name: "Size", values: ["S", "M"] }])).toThrow(
      OptionMatrixError,
    );
  });

  it("throws when >1 existing variant holds a value removed from its axis", () => {
    const stalledSizesProduct = {
      id: "p4",
      title: "Hoodie",
      options: [],
      variants: [
        {
          id: "xl",
          sku: "H-XL",
          price: 60,
          inventory_quantity: 1,
          currency_code: "AUD",
          option_values: [{ option_name: "Size", option_value_id: "xl", value: "XL" }],
          position: 0,
        },
        {
          id: "xxl",
          sku: "H-XXL",
          price: 60,
          inventory_quantity: 2,
          currency_code: "AUD",
          option_values: [{ option_name: "Size", option_value_id: "xxl", value: "XXL" }],
          position: 1,
        },
      ],
    } as never;

    // Neither "XL" nor "XXL" survives in the new value list.
    expect(() =>
      buildOptionMatrix(stalledSizesProduct, [{ name: "Size", values: ["S", "M", "L"] }]),
    ).toThrow(OptionMatrixError);
  });

  it("throws when exactly 1 existing variant holds a value removed from its axis (never silently reassigns)", () => {
    const oneStaleSizeProduct = {
      id: "p5",
      title: "Cap",
      options: [],
      variants: [
        {
          id: "keep",
          sku: "CAP-S",
          price: 15,
          inventory_quantity: 5,
          currency_code: "AUD",
          option_values: [{ option_name: "Size", option_value_id: "s", value: "S" }],
          position: 0,
        },
        {
          id: "stale",
          sku: "CAP-XL",
          price: 15,
          inventory_quantity: 1,
          currency_code: "AUD",
          option_values: [{ option_name: "Size", option_value_id: "xl", value: "XL" }],
          position: 1,
        },
      ],
    } as never;

    expect(() =>
      buildOptionMatrix(oneStaleSizeProduct, [{ name: "Size", values: ["S", "M"] }]),
    ).toThrow(OptionMatrixError);
  });
});
