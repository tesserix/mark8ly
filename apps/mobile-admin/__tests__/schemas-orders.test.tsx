import { orderSchema, orderListSchema } from "@repo/mobile-shared/api/schemas/orders";

// Shaped from AdminOrderResponse (orders_dto.go:152-188). The live Bondi
// store has zero orders, so this is built from the Go DTO — the only truth
// available. Money fields are decimal.Decimal, which marshals QUOTED.
const REAL_ORDER = {
  id: "11111111-1111-1111-1111-111111111111",
  tenant_id: "8c302556-b647-4824-8ce4-73f547ca456e",
  store_id: "8b69eea9-2537-4d36-9d99-bafcbad02dbc",
  order_number: "1001",
  idempotency_key: "idem-1",
  customer_email: "maya@example.com",
  status: "pending",
  payment_status: "paid",
  fulfillment_status: "unfulfilled",
  subtotal: "84.00",
  shipping_total: "0",
  tax_total: "0",
  discount_total: "0",
  grand_total: "84.00",
  refunded_amount: "0",
  currency_code: "AUD",
  items: [],
  addresses: [],
  placed_at: "2026-07-14T09:00:00Z",
  created_at: "2026-07-14T09:00:00Z",
  updated_at: "2026-07-14T09:00:00Z",
};

describe("orderSchema", () => {
  it("parses grand_total from the quoted decimal string the wire actually sends", () => {
    const parsed = orderSchema.parse(REAL_ORDER);
    expect(parsed.grand_total).toBe(84);
    expect(typeof parsed.grand_total).toBe("number");
  });

  it("accepts an order with no customer_name (omitempty)", () => {
    const parsed = orderSchema.parse(REAL_ORDER);
    expect(parsed.customer_name).toBeUndefined();
  });

  it("accepts customer_name when present", () => {
    const parsed = orderSchema.parse({ ...REAL_ORDER, customer_name: "Maya Chen" });
    expect(parsed.customer_name).toBe("Maya Chen");
  });

  it("parses the real empty list envelope from prod", () => {
    const parsed = orderListSchema.parse({
      data: [],
      meta: { page: 1, page_size: 50, total: 0, total_pages: 0 },
    });
    expect(parsed.data).toEqual([]);
    expect(parsed.meta.total).toBe(0);
  });

  it("rejects an {items} envelope — the fiction this whole change removes", () => {
    const res = orderListSchema.safeParse({ items: [], total: 0, next_cursor: null, has_more: false });
    expect(res.success).toBe(false);
  });

  it("names the field path when money is unparseable", () => {
    const res = orderSchema.safeParse({ ...REAL_ORDER, grand_total: "" });
    expect(res.success).toBe(false);
    if (!res.success) expect(res.error.issues[0]!.path).toContain("grand_total");
  });
});
