import { describe, expect, it } from "vitest";
import { render, screen } from "@testing-library/react";

import { OrderLifecycleStepper } from "@/components/orders/OrderLifecycleStepper";
import type { AdminOrder } from "@/lib/api/marketplace-api";

function order(channel: AdminOrder["channel"]): AdminOrder {
  return {
    id: "o1", store_id: "s", order_number: "M-1", status: "fulfilled", payment_status: "paid",
    fulfillment_status: "fulfilled", channel, placed_at: "2026-11-15T00:00:00Z",
  } as AdminOrder;
}

describe("OrderLifecycleStepper", () => {
  it("shows shipping stages for an online order", () => {
    render(<OrderLifecycleStepper order={order("online")} shipmentStatus={null} />);
    expect(screen.getByText("Shipped")).toBeInTheDocument();
    expect(screen.getByText("Delivered")).toBeInTheDocument();
  });

  // Handed over at the counter: nothing ships.
  it("stops at Fulfilled for an in-store sale", () => {
    render(<OrderLifecycleStepper order={order("in_store")} shipmentStatus={null} />);
    expect(screen.getByText("Fulfilled")).toBeInTheDocument();
    expect(screen.queryByText("Shipped")).not.toBeInTheDocument();
    expect(screen.queryByText("Delivered")).not.toBeInTheDocument();
  });
});
