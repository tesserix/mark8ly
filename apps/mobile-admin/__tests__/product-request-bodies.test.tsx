import { productOptionSchema } from "@repo/mobile-shared/api/schemas/products";
import type { UpdateProductBody, UpdateVariantBody } from "@repo/mobile-shared/api/products";

// This file pins the request/response option asymmetry. It is a type-level and
// shape-level guard against the single most expensive recurring bug on this
// project: modelling the RESPONSE with the REQUEST's shape.
describe("product option request vs response shapes", () => {
  it("REQUEST options.values is string[]", () => {
    const body: UpdateProductBody = {
      options: [{ name: "Size", values: ["S", "M", "L"] }],
    };
    expect(body.options![0]!.values).toEqual(["S", "M", "L"]);
  });

  it("RESPONSE options.values is [{id, value, position}] — NOT string[]", () => {
    const parsed = productOptionSchema.parse({
      id: "opt-1",
      name: "Size",
      position: 0,
      values: [{ id: "v1", value: "S", position: 0 }],
    });
    expect(parsed.values[0]!.value).toBe("S");
    expect(parsed.values[0]!.id).toBe("v1");
  });

  it("RESPONSE rejects the REQUEST shape — the two must never be swapped", () => {
    expect(() =>
      productOptionSchema.parse({ id: "opt-1", name: "Size", position: 0, values: ["S", "M"] }),
    ).toThrow();
  });
});

describe("UpdateVariantBody", () => {
  it("carries the shipping fields the variant quick-PATCH accepts", () => {
    const body: UpdateVariantBody = {
      sku: "ABC-1",
      weight_grams: 450,
      length_cm: 30.5,
      width_cm: 20,
      height_cm: 10,
    };
    expect(body.weight_grams).toBe(450);
    expect(body.length_cm).toBe(30.5);
  });
});

describe("UpdateProductBody", () => {
  it("carries category_ids", () => {
    const body: UpdateProductBody = { category_ids: ["cat-1", "cat-2"] };
    expect(body.category_ids).toHaveLength(2);
  });
});
