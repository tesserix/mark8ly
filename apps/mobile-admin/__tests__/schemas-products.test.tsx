import { productSchema, productListSchema } from "@repo/mobile-shared/api/schemas/products";

// A verbatim product from prod (2026-07-16), trimmed to the fields that matter.
const REAL_PRODUCT = {
  id: "a28defe3-9bf0-4273-9247-6f57a5e5a5ab",
  store_id: "8b69eea9-2537-4d36-9d99-bafcbad02dbc",
  handle: "palm-beach-linen-robe",
  title: "Palm Beach Linen Robe",
  description: "An open-front linen robe to throw over swimwear.",
  status: "active",
  tags: ["linen", "robe"],
  seo_title: "Palm Beach Linen Robe — The Bondi Store",
  seo_description: "Open-front linen beach robe with tie waist.",
  primary_category_id: "bdd640fb-0667-4ad1-9c80-317fa3b1799d",
  categories: [],
  options: [],
  variants: [
    { id: "3eabedcb", sku: "TBS-PBLR-XS-S", price: "199", currency_code: "AUD", inventory_quantity: 0, inventory_policy: "deny", option_values: [], position: 0 },
    { id: "451b4cf3", sku: "TBS-PBLR-M-L", price: "199", currency_code: "AUD", inventory_quantity: 4, inventory_policy: "deny", option_values: [], position: 1 },
  ],
  media: [
    { id: "4870d33f", url: "https://cdn.mark8ly.com/a.png", storage_key: "tenants/x/a.png", alt: "front", position: 0, media_type: "image" },
  ],
  published_at: "2026-05-04T23:48:01.08461Z",
  created_at: "2026-05-04T23:48:01.08461Z",
  updated_at: "2026-05-04T23:48:01.08461Z",
};

describe("productSchema", () => {
  it("parses a real product and coerces the QUOTED price string to a number", () => {
    const parsed = productSchema.parse(REAL_PRODUCT);
    expect(parsed.title).toBe("Palm Beach Linen Robe");
    expect(parsed.variants[0]!.price).toBe(199);
    expect(typeof parsed.variants[0]!.price).toBe("number");
  });

  it("parses a decimal price string like \"19.99\"", () => {
    const p = { ...REAL_PRODUCT, variants: [{ ...REAL_PRODUCT.variants[0], price: "19.99" }] };
    expect(productSchema.parse(p).variants[0]!.price).toBe(19.99);
  });

  it("accepts a product with no media", () => {
    const parsed = productSchema.parse({ ...REAL_PRODUCT, media: [] });
    expect(parsed.media).toEqual([]);
  });

  it("accepts a product with no variants without throwing", () => {
    const parsed = productSchema.parse({ ...REAL_PRODUCT, variants: [] });
    expect(parsed.variants).toEqual([]);
  });

  it("parses the real list envelope, meta.total 161", () => {
    const parsed = productListSchema.parse({
      data: [REAL_PRODUCT],
      meta: { page: 1, page_size: 20, total: 161, total_pages: 9 },
    });
    expect(parsed.meta.total).toBe(161);
    expect(parsed.data[0]!.title).toBe("Palm Beach Linen Robe");
  });

  it("rejects the {items} fiction", () => {
    expect(productListSchema.safeParse({ items: [], total: 0 }).success).toBe(false);
  });

  it("names the field path on a bad price", () => {
    const bad = { ...REAL_PRODUCT, variants: [{ ...REAL_PRODUCT.variants[0], price: "abc" }] };
    const res = productSchema.safeParse(bad);
    expect(res.success).toBe(false);
    if (!res.success) expect(res.error.issues[0]!.path.join(".")).toContain("price");
  });

  // Regression test for the options wire-shape bug: every product in prod
  // currently has options: [] (see REAL_PRODUCT above), so that fixture
  // never exercises AdminProductOption at all. The moment a merchant adds
  // a real option, `options` arrives as objects (`AdminProductOption` /
  // `AdminProductOptionValue`, dto.go:114-125) — NOT `string[]`, which was
  // the previous (wrong) schema. This mirrors that populated shape.
  it("parses a product with a populated options array (mirrors AdminProductOption DTO)", () => {
    const withOptions = {
      ...REAL_PRODUCT,
      options: [
        {
          id: "opt-size",
          name: "Size",
          position: 0,
          values: [
            { id: "optval-s", value: "Small", position: 0 },
            { id: "optval-m", value: "Medium", position: 1 },
          ],
        },
      ],
      variants: [
        {
          id: "3eabedcb",
          sku: "TBS-PBLR-XS-S",
          price: "199",
          currency_code: "AUD",
          inventory_quantity: 0,
          inventory_policy: "deny",
          option_values: [
            { option_name: "Size", option_value_id: "optval-s", value: "Small" },
          ],
          position: 0,
        },
      ],
    };

    const parsed = productSchema.parse(withOptions);
    expect(parsed.options).toHaveLength(1);
    expect(parsed.options[0]).toEqual({
      id: "opt-size",
      name: "Size",
      position: 0,
      values: [
        { id: "optval-s", value: "Small", position: 0 },
        { id: "optval-m", value: "Medium", position: 1 },
      ],
    });
    expect(parsed.variants[0]!.option_values[0]).toEqual({
      option_name: "Size",
      option_value_id: "optval-s",
      value: "Small",
    });
  });
});
