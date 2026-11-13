import { describe, it, expect } from "vitest";
import { productFormSchema } from "./product-form";

const base = {
  title: "Skin",
  status: "active" as const,
  price: "599",
  inventoryQuantity: "10",
  alwaysInStock: false,
  categoryIds: [],
};

function issues(options: unknown) {
  const r = productFormSchema.safeParse({ ...base, options });
  return r.success ? [] : r.error.issues.map((i) => ({ path: i.path.join("."), message: i.message }));
}

describe("product form options", () => {
  it("rejects a named option with no values instead of dropping it", () => {
    expect(issues([{ name: "Model", values: [] }])).toEqual([
      {
        path: "options.0.values",
        message: 'Add at least one value to "Model" (press Enter after each), or remove the option.',
      },
    ]);
  });

  it("rejects values with no option name", () => {
    expect(issues([{ name: " ", values: [{ value: "Red" }] }])).toEqual([
      { path: "options.0.name", message: "Give this option a name, e.g. Size or Colour, or remove it." },
    ]);
  });

  it("ignores an untouched blank row and accepts complete options", () => {
    expect(issues([{ name: "", values: [] }])).toEqual([]);
    expect(issues([{ name: "Size", values: [{ value: "S" }, { value: "M" }] }])).toEqual([]);
  });
});
