import { describe, it, expect } from "vitest";
import {
  recentOrderSchema,
  lowStockItemSchema,
} from "../dashboard";

const baseOrder = {
  id: "o1",
  order_number: "1042",
  customer_email: "ana@bondi.co",
  grand_total: 189,
  status: "pending",
  created_at: "2026-07-27T09:00:00Z",
};

const baseLowStock = {
  id: "v1",
  title: "Bondi Linen Shirt",
  variant_title: "M",
  quantity: 2,
  low_stock_threshold: 10,
};

describe("dashboard schema — new optional fields", () => {
  it("accepts a recent order without the new fields (pre-deploy API)", () => {
    const parsed = recentOrderSchema.parse(baseOrder);
    expect(parsed.customer_name).toBeUndefined();
    expect(parsed.image_url).toBeUndefined();
  });

  it("accepts a recent order with the new fields (post-deploy API)", () => {
    const parsed = recentOrderSchema.parse({
      ...baseOrder,
      customer_name: "Ana Ruiz",
      image_url: "https://cdn.example/shirt.jpg",
    });
    expect(parsed.customer_name).toBe("Ana Ruiz");
    expect(parsed.image_url).toBe("https://cdn.example/shirt.jpg");
  });

  it("rejects a null customer_name — omitempty means absent, not null", () => {
    expect(() =>
      recentOrderSchema.parse({ ...baseOrder, customer_name: null }),
    ).toThrow();
  });

  it("accepts a low-stock item without the new fields", () => {
    const parsed = lowStockItemSchema.parse(baseLowStock);
    expect(parsed.product_id).toBeUndefined();
    expect(parsed.image_url).toBeUndefined();
  });

  it("accepts a low-stock item with product_id and image_url", () => {
    const parsed = lowStockItemSchema.parse({
      ...baseLowStock,
      product_id: "p1",
      image_url: "https://cdn.example/shirt.jpg",
    });
    expect(parsed.product_id).toBe("p1");
    expect(parsed.image_url).toBe("https://cdn.example/shirt.jpg");
  });
});
