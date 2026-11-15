import { describe, expect, it, vi } from "vitest";
import { render, screen } from "@testing-library/react";

vi.mock("@/app/(admin)/sell/actions", () => ({ receiptAction: vi.fn() }));

import { InStoreReceiptCard } from "@/components/orders/InStoreReceiptCard";

describe("InStoreReceiptCard", () => {
  it("offers the receipt on an in-store order", () => {
    render(<InStoreReceiptCard storeId="s1" orderId="o1" channel="in_store" />);
    expect(screen.getByRole("heading", { name: "Receipt" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Email receipt" })).toBeInTheDocument();
  });

  it("renders nothing for an online order", () => {
    const { container } = render(<InStoreReceiptCard storeId="s1" orderId="o1" channel="online" />);
    expect(container).toBeEmptyDOMElement();
  });
});
