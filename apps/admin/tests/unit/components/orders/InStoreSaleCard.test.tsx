import { describe, expect, it } from "vitest";
import { render, screen } from "@testing-library/react";

import { InStoreSaleCard, showsShipping } from "@/components/orders/InStoreSaleCard";
import { OrderDetailHeader } from "@/components/orders/OrderDetailHeader";
import { RecentOrders } from "@/components/dashboard/RecentOrders";
import type { AdminOrder, RecentOrder } from "@/lib/api/marketplace-api";

function order(over: Partial<AdminOrder>): AdminOrder {
  return {
    id: "o1", tenant_id: "t", store_id: "s", order_number: "M-1", idempotency_key: "k",
    customer_email: "", status: "fulfilled", payment_status: "paid", fulfillment_status: "fulfilled",
    subtotal: "25", shipping_total: "0", tax_total: "3", discount_total: "5", grand_total: "28",
    refunded_amount: "0", currency_code: "AUD", channel: "in_store", payment_method: "card_external",
    items: [], addresses: [], placed_at: "2026-11-15T00:00:00Z",
    created_at: "2026-11-15T00:00:00Z", updated_at: "2026-11-15T00:00:00Z",
    ...over,
  } as AdminOrder;
}

describe("InStoreSaleCard", () => {
  it.each([
    ["card_external", undefined, "Card (EFTPOS)"],
    ["cash", undefined, "Cash"],
    ["bank_transfer", undefined, "Bank transfer"],
    ["other", "UPI", "UPI"],
  ] as const)("says how a %s sale was paid", (method, label, text) => {
    render(<InStoreSaleCard order={order({ payment_method: method, payment_method_label: label })} />);
    expect(screen.getByText(text)).toBeInTheDocument();
  });

  it("shows cash given and change", () => {
    render(<InStoreSaleCard order={order({ payment_method: "cash", cash_tendered: "50" })} />);
    expect(screen.getByText("A$50.00")).toBeInTheDocument();
    expect(screen.getByText("A$22.00")).toBeInTheDocument();
  });

  it("names who rang it up, or says a team member", () => {
    const { rerender } = render(<InStoreSaleCard order={order({ sold_by_email: "sam@shop.test" })} />);
    expect(screen.getByText("sam@shop.test")).toBeInTheDocument();
    rerender(<InStoreSaleCard order={order({})} />);
    expect(screen.getByText("A team member")).toBeInTheDocument();
  });

  it("shows the note", () => {
    render(<InStoreSaleCard order={order({ notes: "Engrave Sam" })} />);
    expect(screen.getByText("Engrave Sam")).toBeInTheDocument();
  });

  it("renders nothing for an online order", () => {
    const { container } = render(<InStoreSaleCard order={order({ channel: "online" })} />);
    expect(container).toBeEmptyDOMElement();
  });
});

describe("in-store order elsewhere", () => {
  it("hides shipping for an in-store sale", () => {
    expect(showsShipping(order({}))).toBe(false);
    expect(showsShipping(order({ channel: "online" }))).toBe(true);
  });

  it("header says In-store sale in place of an empty customer", () => {
    render(<OrderDetailHeader order={order({})} />);
    expect(screen.getByText("In-store sale")).toBeInTheDocument();
  });

  it("dashboard recent orders say In-store sale in place of an empty email", () => {
    const recent = { id: "o1", order_number: "M-1", customer_email: "", status: "fulfilled", grand_total: "28", created_at: "2026-11-15T00:00:00Z" } as RecentOrder;
    render(<RecentOrders orders={[recent]} currencyCode="AUD" />);
    expect(screen.getByText("In-store sale")).toBeInTheDocument();
  });
});
