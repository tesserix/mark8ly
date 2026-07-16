import {
  categorySchema,
  categoryListSchema,
  categoryRefSchema,
} from "@repo/mobile-shared/api/schemas/categories";
import { productSchema } from "@repo/mobile-shared/api/schemas/products";

// Shape per AdminCategoryResponse (dto.go:14-27).
const REAL_CATEGORY = {
  id: "bdd640fb-0667-4ad1-9c80-317fa3b1799d",
  store_id: "8b69eea9-2537-4d36-9d99-bafcbad02dbc",
  name: "Swimwear",
  slug: "swimwear",
  position: 0,
  is_active: true,
  featured: false,
  created_at: "2026-05-04T23:48:01.08461Z",
  updated_at: "2026-05-04T23:48:01.08461Z",
};

describe("categorySchema", () => {
  it("parses a category with no parent (a root of the tree)", () => {
    const parsed = categorySchema.parse(REAL_CATEGORY);
    expect(parsed.name).toBe("Swimwear");
    expect(parsed.parent_id).toBeUndefined();
  });

  it("parses a child category carrying parent_id", () => {
    const parsed = categorySchema.parse({ ...REAL_CATEGORY, parent_id: "parent-uuid" });
    expect(parsed.parent_id).toBe("parent-uuid");
  });

  it("treats omitted optionals as absent, not null (Go omitempty)", () => {
    const parsed = categorySchema.parse(REAL_CATEGORY);
    expect(parsed.description).toBeUndefined();
    expect(parsed.image_url).toBeUndefined();
  });

  it("parses the {data} envelope — categories send NO meta", () => {
    const parsed = categoryListSchema.parse({ data: [REAL_CATEGORY] });
    expect(parsed.data[0]!.slug).toBe("swimwear");
  });

  it("fails loudly on a malformed category rather than passing it through", () => {
    // `position` is required; a category missing it must throw, not arrive as
    // undefined and sort unpredictably in the picker.
    const { position, ...noPosition } = REAL_CATEGORY;
    expect(() => categoryListSchema.parse({ data: [noPosition] })).toThrow();
  });
});

describe("categoryRefSchema", () => {
  it("parses the lean ref embedded on a product", () => {
    const parsed = categoryRefSchema.parse({ id: "c1", name: "Swimwear", slug: "swimwear" });
    expect(parsed.name).toBe("Swimwear");
  });

  it("product.categories parses refs, not full categories", () => {
    const product = {
      id: "p1",
      store_id: "s1",
      handle: "h",
      title: "T",
      status: "active",
      tags: [],
      categories: [{ id: "c1", name: "Swimwear", slug: "swimwear" }],
      options: [],
      variants: [],
      media: [],
      created_at: "2026-05-04T23:48:01.08461Z",
      updated_at: "2026-05-04T23:48:01.08461Z",
    };
    const parsed = productSchema.parse(product);
    expect(parsed.categories[0]!.name).toBe("Swimwear");
  });
});
