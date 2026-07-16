import {
  primaryVariant,
  productPrice,
  productSku,
  productStock,
  productThumb,
  productCurrency,
  formatMoney,
} from "@/lib/product-display";
import type { Product } from "@repo/mobile-shared/api/types";

function makeProduct(over: Partial<Product> = {}): Product {
  return {
    id: "p-1",
    store_id: "s-1",
    handle: "h",
    title: "T",
    status: "active",
    tags: [],
    categories: [],
    options: [],
    variants: [],
    media: [],
    created_at: "2026-05-04T23:48:01Z",
    updated_at: "2026-05-04T23:48:01Z",
    ...over,
  } as Product;
}

const v = (id: string, position: number, price: number, sku: string, qty: number) => ({
  id, sku, price, currency_code: "AUD",
  inventory_quantity: qty, inventory_policy: "deny",
  option_values: [], position,
});

describe("primaryVariant", () => {
  it("picks by POSITION, not array order — the wire returns them unsorted", () => {
    // Exactly the real "Bondi Linen Beach Shirt" ordering: 2,3,4,0,1.
    const p = makeProduct({
      variants: [
        v("m", 2, 149, "TBS-BLBS-M", 5),
        v("l", 3, 149, "TBS-BLBS-L", 6),
        v("xl", 4, 149, "TBS-BLBS-XL", 7),
        v("xs", 0, 149, "TBS-BLBS-XS", 1),
        v("s", 1, 149, "TBS-BLBS-S", 2),
      ],
    });
    expect(primaryVariant(p)!.id).toBe("xs");
    expect(productSku(p)).toBe("TBS-BLBS-XS");
    expect(productStock(p)).toBe(21);
  });

  it("returns undefined when there are no variants", () => {
    expect(primaryVariant(makeProduct())).toBeUndefined();
    expect(productPrice(makeProduct())).toBeUndefined();
    expect(productSku(makeProduct())).toBeUndefined();
    expect(productCurrency(makeProduct())).toBeUndefined();
  });

  it("does not mutate the input array", () => {
    const variants = [v("b", 1, 1, "B", 0), v("a", 0, 1, "A", 0)];
    const p = makeProduct({ variants });
    primaryVariant(p);
    expect(variants[0]!.id).toBe("b");
  });

  it("handles the single-variant case", () => {
    const p = makeProduct({ variants: [v("only", 0, 21, "BND-49", 100)] });
    expect(productPrice(p)).toBe(21);
    expect(productStock(p)).toBe(100);
    expect(productCurrency(p)).toBe("AUD");
  });
});

describe("productStock", () => {
  it("sums inventory across ALL variants, not just the primary", () => {
    const p = makeProduct({
      variants: [v("a", 0, 1, "A", 3), v("b", 1, 1, "B", 4)],
    });
    expect(productStock(p)).toBe(7);
  });

  it("is 0 when there are no variants", () => {
    expect(productStock(makeProduct())).toBe(0);
  });
});

describe("productThumb", () => {
  it("picks the lowest-position media, not media[0]", () => {
    const p = makeProduct({
      media: [
        { id: "b", url: "b.png", storage_key: "b", position: 1 },
        { id: "a", url: "a.png", storage_key: "a", position: 0 },
      ],
    });
    expect(productThumb(p)).toBe("a.png");
  });

  it("does not mutate the input media array", () => {
    const media = [
      { id: "b", url: "b.png", storage_key: "b", position: 1 },
      { id: "a", url: "a.png", storage_key: "a", position: 0 },
    ];
    productThumb(makeProduct({ media }));
    expect(media[0]!.id).toBe("b");
  });

  it("returns undefined when a product has no media (1 real product does not)", () => {
    expect(productThumb(makeProduct())).toBeUndefined();
  });
});

describe("formatMoney", () => {
  it("uses the product's real currency, not a hardcoded USD", () => {
    expect(formatMoney(199, "AUD")).toContain("199");
    expect(formatMoney(199, "AUD")).not.toBe(formatMoney(199, "USD"));
  });

  it("falls back to a bare number when no currency is known", () => {
    expect(formatMoney(199)).toContain("199");
  });
});
