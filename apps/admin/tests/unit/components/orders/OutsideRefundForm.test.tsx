import { beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";

const recordOutsideRefundAction = vi.fn();
const refresh = vi.fn();
vi.mock("@/app/(admin)/orders/[id]/actions", () => ({
  recordOutsideRefundAction: (...a: unknown[]) => recordOutsideRefundAction(...a),
}));
vi.mock("next/navigation", () => ({ useRouter: () => ({ refresh }) }));

import { OutsideRefundForm } from "@/components/orders/OutsideRefundForm";

const order = {
  id: "o1",
  store_id: "s1",
  grand_total: "77.50",
  refunded_amount: "10.00",
  currency_code: "AUD",
  items: [
    { id: "i1", variant_id: "v1", title_snapshot: "Mug", quantity: 2 },
    { id: "i2", title_snapshot: "Hand-painted tile", quantity: 1 },
  ],
};

beforeEach(() => {
  recordOutsideRefundAction.mockReset().mockResolvedValue({ ok: true });
  refresh.mockReset();
});

describe("OutsideRefundForm", () => {
  it("defaults the amount to what is left to refund", () => {
    render(<OutsideRefundForm order={order} />);
    expect(screen.getByRole("textbox", { name: /amount refunded/i })).toHaveValue("67.50");
  });

  it("offers stock steppers only for catalogue lines, up to the quantity sold", async () => {
    const user = userEvent.setup();
    render(<OutsideRefundForm order={order} />);
    expect(screen.queryByRole("button", { name: /one more hand-painted tile/i })).not.toBeInTheDocument();
    const more = screen.getByRole("button", { name: "One more Mug back in stock" });
    await user.click(more);
    await user.click(more);
    expect(more).toBeDisabled();
  });

  it("records the refund with one request id, reused on retry", async () => {
    recordOutsideRefundAction.mockResolvedValueOnce({ ok: false, error: { message: "No connection." } });
    const user = userEvent.setup();
    render(<OutsideRefundForm order={order} />);
    await user.click(screen.getByRole("radio", { name: "Cash" }));
    await user.click(screen.getByRole("button", { name: "One more Mug back in stock" }));
    await user.click(screen.getByRole("button", { name: "Record refund" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("No connection.");
    await user.click(screen.getByRole("button", { name: "Record refund" }));

    const [first, second] = recordOutsideRefundAction.mock.calls;
    expect(first![0]).toBe("s1");
    expect(first![1]).toBe("o1");
    expect(first![2]).toMatchObject({ amount: "67.50", method: "cash", items: [{ order_item_id: "i1", quantity: 1 }] });
    expect(second![2].refund_request_id).toBe(first![2].refund_request_id);
    expect(refresh).toHaveBeenCalled();
  });

  it("says nothing is left when it has all been refunded", () => {
    render(<OutsideRefundForm order={{ ...order, refunded_amount: "77.50" }} />);
    expect(screen.getByText("This sale has been fully refunded.")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Record refund" })).not.toBeInTheDocument();
  });
});
