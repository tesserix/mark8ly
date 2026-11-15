import { describe, expect, it, vi } from "vitest";
import { render, screen } from "@testing-library/react";

vi.mock("@/app/(admin)/orders/[id]/actions", () => ({}));
vi.mock("@/app/(admin)/orders/[id]/shipping-actions", () => ({ getShipmentAction: vi.fn() }));
vi.mock("@/components/feedback/Toaster", () => ({ useToast: () => ({ success: vi.fn(), error: vi.fn() }) }));
vi.mock("next/navigation", () => ({ useRouter: () => ({ refresh: vi.fn() }) }));

import { OrderActionsBar } from "@/components/orders/OrderActionsBar";
import type { AdminOrder } from "@/lib/api/marketplace-api";

function order(channel: AdminOrder["channel"]): AdminOrder {
  return {
    id: "o1", tenant_id: "t", store_id: "s", order_number: "M-1", idempotency_key: "k",
    customer_email: "", status: "fulfilled", payment_status: "paid", fulfillment_status: "fulfilled",
    subtotal: "25", shipping_total: "0", tax_total: "3", discount_total: "0", grand_total: "28",
    refunded_amount: "0", currency_code: "AUD", channel, items: [], addresses: [],
    placed_at: "2026-11-15T00:00:00Z", created_at: "2026-11-15T00:00:00Z", updated_at: "2026-11-15T00:00:00Z",
  } as AdminOrder;
}

describe("OrderActionsBar refunds", () => {
  it("offers a refund on a paid online order", () => {
    render(<OrderActionsBar order={order("online")} shipmentStatus={null} />);
    expect(screen.getByRole("button", { name: /issue refund/i })).toBeInTheDocument();
  });

  // Paid on the shop's own terminal or in cash: there is no provider payment to refund.
  it("does not offer a provider refund on an in-store sale", () => {
    render(<OrderActionsBar order={order("in_store")} shipmentStatus={null} />);
    expect(screen.queryByRole("button", { name: /issue refund/i })).not.toBeInTheDocument();
  });
});
