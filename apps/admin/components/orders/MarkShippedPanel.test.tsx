import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";

const markShippedAction = vi.fn();
const updateTrackingAction = vi.fn();
vi.mock("@/app/(admin)/orders/[id]/shipping-actions", () => ({
  markShippedAction: (...a: unknown[]) => markShippedAction(...a),
  updateTrackingAction: (...a: unknown[]) => updateTrackingAction(...a),
}));

import { MarkShippedPanel } from "./MarkShippedPanel";

const couriers = [
  { id: "c1", name: "DTDC", tracking_url_template: "https://t.example/{number}" },
  { id: "c2", name: "Local rider", tracking_url_template: null },
];
const shipped = {
  id: "s1",
  order_id: "o1",
  provider: "own",
  provider_shipment_id: "",
  tracking_number: "AB123",
  label_url: "",
  service: "",
  status: "in_transit",
  currency_code: "INR",
  created_at: "",
  courier_id: "c1",
  courier_name: "DTDC",
  tracking_url: "https://t.example/AB123",
};
const base = { storeId: "st1", orderId: "o1", orderStatus: "confirmed", couriers, initialShipment: null };

beforeEach(() => {
  vi.clearAllMocks();
  localStorage.clear();
});

describe("MarkShippedPanel", () => {
  it("shows the buyer's delivery choice, estimate only when set", () => {
    const { rerender } = render(
      <MarkShippedPanel {...base} optionName="Express" optionEstimate="2-3 days" />,
    );
    expect(screen.getByText("Buyer chose: Express (2-3 days)")).toBeInTheDocument();
    rerender(<MarkShippedPanel {...base} optionName="Express" />);
    expect(screen.getByText("Buyer chose: Express")).toBeInTheDocument();
  });

  it("shows courier select, tracking input and button; defaults to last used courier", async () => {
    localStorage.setItem("m8.lastCourier.st1", "c2");
    render(<MarkShippedPanel {...base} />);
    expect(screen.getByLabelText(/tracking number/i)).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Mark as shipped" })).toBeInTheDocument();
    await waitFor(() => expect(screen.getByLabelText(/courier/i)).toHaveValue("c2"));
  });

  it("submits once, disables while pending, then shows the shipment", async () => {
    let resolve!: (v: unknown) => void;
    markShippedAction.mockReturnValue(new Promise((r) => (resolve = r)));
    render(<MarkShippedPanel {...base} />);
    await userEvent.type(screen.getByLabelText(/tracking number/i), "AB123");
    const btn = screen.getByRole("button", { name: "Mark as shipped" });
    await userEvent.click(btn);
    await waitFor(() => expect(btn).toBeDisabled());
    await userEvent.click(btn);
    expect(markShippedAction).toHaveBeenCalledTimes(1);
    expect(markShippedAction).toHaveBeenCalledWith("st1", "o1", { courierId: "c1", trackingNumber: "AB123" });
    resolve({ ok: true, data: shipped });
    await screen.findByRole("button", { name: "Edit tracking" });
    expect(await screen.findByText("DTDC")).toBeInTheDocument();
    expect(localStorage.getItem("m8.lastCourier.st1")).toBe("c1");
    const link = screen.getByRole("link", { name: /AB123/ });
    expect(link).toHaveAttribute("href", "https://t.example/AB123");
    expect(link).toHaveAttribute("target", "_blank");
    expect(link).toHaveAttribute("rel", "noopener noreferrer");
    expect(screen.getByRole("button", { name: "Edit tracking" })).toBeInTheDocument();
  });

  it("shows the API message on error", async () => {
    markShippedAction.mockResolvedValue({ ok: false, error: { code: "already_shipped", message: "Already shipped." } });
    render(<MarkShippedPanel {...base} />);
    await userEvent.click(screen.getByRole("button", { name: "Mark as shipped" }));
    expect(await screen.findByText("Already shipped.")).toBeInTheDocument();
  });

  it("does not link a non-https tracking url", () => {
    render(<MarkShippedPanel {...base} initialShipment={{ ...shipped, tracking_url: "javascript:alert(1)" }} />);
    expect(screen.getByText("AB123")).toBeInTheDocument();
    expect(screen.queryByRole("link")).toBeNull();
  });

  it("edit tracking rejects blank and saves a new number", async () => {
    updateTrackingAction.mockResolvedValue({ ok: true, data: { ...shipped, tracking_number: "ZZ9" } });
    render(<MarkShippedPanel {...base} initialShipment={shipped} />);
    await userEvent.click(screen.getByRole("button", { name: "Edit tracking" }));
    const input = screen.getByLabelText(/tracking number/i);
    await userEvent.clear(input);
    expect(screen.getByRole("button", { name: "Save" })).toBeDisabled();
    await userEvent.type(input, "ZZ9");
    await userEvent.click(screen.getByRole("button", { name: "Save" }));
    expect(updateTrackingAction).toHaveBeenCalledWith("st1", "o1", "s1", { trackingNumber: "ZZ9", courierId: "c1" });
    expect(await screen.findByText("ZZ9")).toBeInTheDocument();
  });

  it("treats a cancelled shipment as none and shows the form", () => {
    render(<MarkShippedPanel {...base} initialShipment={{ ...shipped, status: "cancelled" }} />);
    expect(screen.getByRole("button", { name: "Mark as shipped" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Edit tracking" })).toBeNull();
  });

  it("links to Settings -> Shipping when no couriers are saved", () => {
    render(<MarkShippedPanel {...base} couriers={[]} />);
    expect(screen.getByRole("link", { name: /settings/i })).toHaveAttribute("href", "/settings/shipping");
    expect(screen.queryByRole("button", { name: "Mark as shipped" })).toBeNull();
  });
});
