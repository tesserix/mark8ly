import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";

import type { DeliverySettings } from "@/lib/api/delivery-api";

const setShippingMode = vi.fn();
const saveDeliveryOptions = vi.fn();
const createCourier = vi.fn();
const updateCourier = vi.fn();
const deleteCourier = vi.fn();
const refresh = vi.fn();

vi.mock("@/app/(admin)/settings/shipping/delivery-actions", () => ({
  setShippingMode: (...a: unknown[]) => setShippingMode(...a),
  saveDeliveryOptions: (...a: unknown[]) => saveDeliveryOptions(...a),
  createCourier: (...a: unknown[]) => createCourier(...a),
  updateCourier: (...a: unknown[]) => updateCourier(...a),
  deleteCourier: (...a: unknown[]) => deleteCourier(...a),
}));
vi.mock("next/navigation", () => ({ useRouter: () => ({ refresh }) }));
vi.mock("@/components/feedback/Toaster", () => ({
  useToast: () => ({ toast: { success: vi.fn(), error: vi.fn() } }),
}));

import { ModeSwitch, OwnDeliverySettings } from "./OwnDeliverySettings";

function settings(o: Partial<DeliverySettings> = {}): DeliverySettings {
  return { mode: "own", free_shipping_min: null, options: [], couriers: [], ...o };
}
const opt = (i: number, enabled = true) => ({
  id: `o${i}`, name: `Opt ${i}`, price: "5.00", estimate: null, enabled, position: i,
});

beforeEach(() => {
  vi.clearAllMocks();
  setShippingMode.mockResolvedValue({ ok: true });
  saveDeliveryOptions.mockResolvedValue({ ok: true });
  createCourier.mockResolvedValue({ ok: true });
});

describe("ModeSwitch", () => {
  it("shows the current mode and saves Own immediately", async () => {
    const user = userEvent.setup();
    render(<ModeSwitch mode="carrier" editable />);
    expect(screen.getByLabelText("Mark8ly couriers")).toBeChecked();
    await user.click(screen.getByLabelText("Own delivery"));
    expect(setShippingMode).toHaveBeenCalledWith("own");
  });
});

describe("OwnDeliverySettings", () => {
  it("pre-fills one Standard row with an empty price on first visit", () => {
    render(<OwnDeliverySettings settings={settings()} editable />);
    expect(screen.getByLabelText("Option 1 name")).toHaveValue("Standard");
    expect(screen.getByLabelText("Option 1 price")).toHaveValue("");
  });

  it("disables Add option at 5 options", () => {
    render(
      <OwnDeliverySettings
        settings={settings({ options: [1, 2, 3, 4, 5].map((i) => opt(i)) })}
        editable
      />,
    );
    expect(screen.getByRole("button", { name: "Add option" })).toBeDisabled();
  });

  it("always sends free_shipping_min, null when blank", async () => {
    const user = userEvent.setup();
    render(<OwnDeliverySettings settings={settings({ options: [opt(1)] })} editable />);
    await user.click(screen.getByRole("button", { name: "Save options" }));
    await waitFor(() => expect(saveDeliveryOptions).toHaveBeenCalled());
    expect(saveDeliveryOptions.mock.calls[0][1]).toBeNull();
  });

  it("rejects a courier template without {number} and does not save", async () => {
    const user = userEvent.setup();
    render(<OwnDeliverySettings settings={settings({ options: [opt(1)] })} editable />);
    await user.click(screen.getByRole("button", { name: "Add courier" }));
    await user.type(screen.getByLabelText("Courier name"), "DTDC");
    await user.type(screen.getByLabelText("Tracking link"), "https://x.com/track");
    await user.click(screen.getByRole("button", { name: "Save courier" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("{number}");
    expect(createCourier).not.toHaveBeenCalled();
  });

  it("Test link opens the template with TEST123 in a new tab", async () => {
    const user = userEvent.setup();
    const open = vi.spyOn(window, "open").mockReturnValue(null);
    render(
      <OwnDeliverySettings
        settings={settings({
          options: [opt(1)],
          couriers: [{ id: "c1", name: "DTDC", tracking_url_template: "https://x.com/t/{number}" }],
        })}
        editable
      />,
    );
    await user.click(screen.getByRole("button", { name: "Test link" }));
    expect(open).toHaveBeenCalledWith("https://x.com/t/TEST123", "_blank", "noopener,noreferrer");
  });

  it("warns when no option is enabled", () => {
    render(<OwnDeliverySettings settings={settings({ options: [opt(1, false)] })} editable />);
    expect(
      screen.getByText("Buyers can't check out until at least one option is on."),
    ).toBeInTheDocument();
  });
});
