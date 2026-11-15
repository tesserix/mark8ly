import { describe, expect, it } from "vitest";
import { render, screen } from "@testing-library/react";

import { OrdersList } from "@/components/orders/OrdersList";
import type { AdminOrder } from "@/lib/api/marketplace-api";

function order(over: Partial<AdminOrder>): AdminOrder {
  return {
    id: "o1", tenant_id: "t", store_id: "s", order_number: "M-1", idempotency_key: "k",
    customer_email: "ava@example.com", status: "confirmed", payment_status: "paid",
    fulfillment_status: "unfulfilled", subtotal: "10", shipping_total: "0", tax_total: "0",
    discount_total: "0", grand_total: "10", refunded_amount: "0", currency_code: "AUD",
    channel: "online", items: [], addresses: [], placed_at: "2026-11-15T00:00:00Z",
    created_at: "2026-11-15T00:00:00Z", updated_at: "2026-11-15T00:00:00Z",
    ...over,
  } as AdminOrder;
}

describe("OrdersList", () => {
  it("shows an In-store badge on in-store orders only", () => {
    render(<OrdersList orders={[order({ id: "a", order_number: "M-1" }), order({ id: "b", order_number: "M-2", channel: "in_store" })]} />);
    expect(screen.getAllByText("In-store")).toHaveLength(1);
    expect(screen.getByRole("link", { name: /M-2/ })).toHaveTextContent("In-store");
  });

  it("shows In-store sale when there is no customer", () => {
    render(<OrdersList orders={[order({ channel: "in_store", customer_email: "" })]} />);
    const row = screen.getByRole("link", { name: "Order M-1, in-store sale" });
    expect(row).toHaveTextContent("In-store sale");
  });
});
