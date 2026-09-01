import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";

import type { ShippingConfig } from "@/lib/api/settings-api";
import type { Warehouse } from "@/lib/api/warehouses-api";

const saveShippingConfig = vi.fn();
const refresh = vi.fn();

vi.mock("@/app/(admin)/settings/shipping/actions", () => ({
  saveShippingConfig: (...args: unknown[]) => saveShippingConfig(...args),
}));

vi.mock("next/navigation", () => ({ useRouter: () => ({ refresh }) }));

import { ShippingConfigForm } from "./ShippingConfigForm";

function warehouse(overrides: Partial<Warehouse> = {}): Warehouse {
  return {
    id: "wh-1",
    name: "Main Warehouse",
    line1: "1 Campbell Parade",
    city: "Bondi Beach",
    region: "NSW",
    postal_code: "2026",
    country_code: "AU",
    phone: "+61200000000",
    is_default: true,
    priority: 0,
    ...overrides,
  };
}

beforeEach(() => {
  vi.clearAllMocks();
  saveShippingConfig.mockResolvedValue({ ok: true });
});

// The submit button stays disabled until a NEW config has credentials, so
// every save path here types one first.
async function saveForm(user: ReturnType<typeof userEvent.setup>) {
  const apiKey = screen.queryByLabelText(/api key/i);
  if (apiKey && (apiKey as HTMLInputElement).value === "") {
    await user.type(apiKey, "test-token-1234");
  }
  await user.click(screen.getByRole("button", { name: /save configuration/i }));
}

// The form has several selects (mode, pickup window). Only ever assert on
// the one labelled Warehouse.
const warehousePicker = () => screen.queryByLabelText(/^warehouse$/i);

// #177 PR 5d: the carrier form no longer collects an address. A free-text
// name that had to exactly match an existing record is what created a
// second, stockless warehouse and left its orders unshippable.
describe("ShippingConfigForm — ships from", () => {
  it("collects no address fields at all", () => {
    render(
      <ShippingConfigForm provider="shipengine" warehouses={[warehouse()]} />,
    );

    expect(screen.queryByLabelText(/street/i)).toBeNull();
    expect(screen.queryByLabelText(/postcode/i)).toBeNull();
    expect(screen.queryByLabelText(/postal/i)).toBeNull();
    expect(screen.queryByText(/Warehouse address/i)).toBeNull();
  });

  // One warehouse is not a choice. A dropdown with one option is a
  // question with one answer.
  it("with exactly one warehouse it binds silently and shows it read-only", async () => {
    const user = userEvent.setup();
    render(
      <ShippingConfigForm provider="shipengine" warehouses={[warehouse()]} />,
    );

    expect(screen.getByText("Main Warehouse")).toBeInTheDocument();
    expect(warehousePicker()).toBeNull();

    await saveForm(user);

    await waitFor(() => expect(saveShippingConfig).toHaveBeenCalled());
    const payload = saveShippingConfig.mock.calls[0][1];
    expect(payload.warehouse_id).toBe("wh-1");
  });

  it("with several warehouses it offers a picker", () => {
    render(
      <ShippingConfigForm
        provider="shipengine"
        warehouses={[
          warehouse({ id: "wh-1", name: "Bondi" }),
          warehouse({ id: "wh-2", name: "Marrickville", is_default: false }),
        ]}
      />,
    );

    expect(warehousePicker()).toBeInTheDocument();
  });

  it("an existing config opens on the warehouse it is bound to", async () => {
    const user = userEvent.setup();
    const existing = {
      provider: "shipengine",
      enabled: true,
      mode: "test",
      warehouse_id: "wh-2",
    } as unknown as ShippingConfig;

    render(
      <ShippingConfigForm
        provider="shipengine"
        existing={existing}
        warehouses={[
          warehouse({ id: "wh-1", name: "Bondi" }),
          warehouse({ id: "wh-2", name: "Marrickville", is_default: false }),
        ]}
      />,
    );

    await saveForm(user);
    await waitFor(() => expect(saveShippingConfig).toHaveBeenCalled());
    expect(saveShippingConfig.mock.calls[0][1].warehouse_id).toBe("wh-2");
  });

  // Offering a picker with nothing in it, or letting the save through,
  // would reproduce the original failure: a carrier with no origin, which
  // the storefront can only report as "no delivery options".
  it("with no warehouses it points at the warehouses page instead of a picker", () => {
    render(<ShippingConfigForm provider="shipengine" warehouses={[]} />);

    expect(warehousePicker()).toBeNull();
    const link = screen.getByRole("link", { name: /add a warehouse/i });
    expect(link).toHaveAttribute("href", "/settings/warehouses");
  });

  it("refuses to save when several warehouses exist and none is chosen", async () => {
    const user = userEvent.setup();
    render(
      <ShippingConfigForm
        provider="shipengine"
        warehouses={[
          warehouse({ id: "wh-1", name: "Bondi" }),
          warehouse({ id: "wh-2", name: "Marrickville", is_default: false }),
        ]}
      />,
    );

    await saveForm(user);

    expect(
      await screen.findByText(/Choose which warehouse/i),
    ).toBeInTheDocument();
    expect(saveShippingConfig).not.toHaveBeenCalled();
  });

  // The address is the warehouse's. Sending it from here too would put one
  // address behind two forms, which is how the copies drifted apart.
  it("sends only the id, never an address", async () => {
    const user = userEvent.setup();
    render(
      <ShippingConfigForm provider="shipengine" warehouses={[warehouse()]} />,
    );

    await saveForm(user);
    await waitFor(() => expect(saveShippingConfig).toHaveBeenCalled());

    const payload = saveShippingConfig.mock.calls[0][1];
    for (const key of [
      "warehouse_name",
      "warehouse_line1",
      "warehouse_city",
      "warehouse_postal",
      "warehouse_phone",
    ]) {
      expect(payload[key]).toBeUndefined();
    }
  });
});
