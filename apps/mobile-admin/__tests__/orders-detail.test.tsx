jest.mock("@/lib/api-client", () => ({ useApiClient: () => ({}) }));

import {
  orderDetailSchema,
  orderItemSchema,
  orderAddressSchema,
} from "@repo/mobile-shared/api/schemas/orders";
import { nextOrderPage } from "@/lib/hooks/use-orders";
import type { OrderListResponse } from "@repo/mobile-shared/api/schemas/orders";

const BASE_ORDER = {
  id: "o1",
  tenant_id: "t1",
  store_id: "s1",
  order_number: "1001",
  idempotency_key: "idem-1",
  customer_email: "buyer@example.com",
  status: "confirmed",
  payment_status: "paid",
  fulfillment_status: "unfulfilled",
  subtotal: "89.00",
  shipping_total: "0",
  tax_total: "8.90",
  discount_total: "0",
  grand_total: "97.90",
  refunded_amount: "0",
  currency_code: "AUD",
  placed_at: "2026-07-17T00:00:00Z",
  created_at: "2026-07-17T00:00:00Z",
  updated_at: "2026-07-17T00:00:00Z",
  items: [
    {
      id: "li1",
      title_snapshot: "Linen Shirt",
      sku_snapshot: "LS-M",
      option_summary: "Size: M",
      unit_price: "89.00",
      quantity: 1,
      line_total: "89.00",
      currency_code: "AUD",
    },
  ],
  addresses: [
    { kind: "shipping", name: "Ada", line1: "1 Main St", city: "Sydney", region: "NSW", postal_code: "2000", country_code: "AU" },
  ],
  tax_lines: [{ description: "GST", rate: "0.10", amount: "8.90" }],
};

describe("orderDetailSchema", () => {
  it("parses items[], addresses[] and tax_lines with quoted-decimal money coerced to numbers", () => {
    const parsed = orderDetailSchema.parse(BASE_ORDER);
    expect(parsed.items[0]!.line_total).toBe(89);
    expect(parsed.addresses[0]!.country_code).toBe("AU");
    expect(parsed.tax_lines?.[0]!.amount).toBe(8.9);
    expect(parsed.grand_total).toBe(97.9);
  });

  it("accepts an order with no tax_lines (Get-only, omitempty)", () => {
    const { tax_lines, ...noTax } = BASE_ORDER;
    expect(() => orderDetailSchema.parse(noTax)).not.toThrow();
  });

  it("drops omitempty optionals on items/addresses (absent, not null)", () => {
    const item = { id: "x", title_snapshot: "T", sku_snapshot: "S", unit_price: "1", quantity: 1, line_total: "1", currency_code: "AUD" };
    expect(() => orderItemSchema.parse(item)).not.toThrow();
    const addr = { kind: "billing", name: "N", line1: "L", city: "C", country_code: "AU" };
    expect(() => orderAddressSchema.parse(addr)).not.toThrow();
  });
});

describe("nextOrderPage", () => {
  function pageOf(page: number, total_pages: number): OrderListResponse {
    return { data: [], meta: { page, page_size: 50, total: total_pages * 50, total_pages } };
  }
  it("advances while pages remain, stops on the last", () => {
    expect(nextOrderPage(pageOf(1, 3))).toBe(2);
    expect(nextOrderPage(pageOf(3, 3))).toBeUndefined();
    expect(nextOrderPage(pageOf(1, 1))).toBeUndefined();
  });
});
