import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";

import type { Warehouse } from "@/lib/api/warehouses-api";

const saveWarehouse = vi.fn();
const deleteWarehouse = vi.fn();
const makeDefaultWarehouse = vi.fn();
const reorderWarehouseList = vi.fn();
const refresh = vi.fn();

vi.mock("@/app/(admin)/settings/warehouses/actions", () => ({
  saveWarehouse: (...args: unknown[]) => saveWarehouse(...args),
  deleteWarehouse: (...args: unknown[]) => deleteWarehouse(...args),
  makeDefaultWarehouse: (...args: unknown[]) => makeDefaultWarehouse(...args),
  reorderWarehouseList: (...args: unknown[]) => reorderWarehouseList(...args),
}));

vi.mock("next/navigation", () => ({
  useRouter: () => ({ refresh }),
}));

import { WarehousesSettingsClient } from "./WarehousesSettingsClient";

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
});

describe("WarehousesSettingsClient — removal", () => {
  it("removing asks first and calls nothing until confirmed", async () => {
    const user = userEvent.setup();
    render(
      <WarehousesSettingsClient
        warehouses={[warehouse()]}
        editable
        storeCountry="AU"
      />,
    );

    await user.click(screen.getByRole("button", { name: "Remove" }));
    expect(screen.getByText("Remove Main Warehouse?")).toBeInTheDocument();
    expect(deleteWarehouse).not.toHaveBeenCalled();
  });

  // An archive reported as a delete would leave the merchant expecting the
  // row to be gone from past orders, where it deliberately still appears —
  // and, worse, unaware that its stock just became unsellable.
  it("an archive says so, and names the stock it stranded", async () => {
    const user = userEvent.setup();
    deleteWarehouse.mockResolvedValue({
      ok: true,
      data: {
        id: "wh-1",
        outcome: "archived",
        reason: "holds_stock",
        units_remaining: 12,
      },
    });
    render(
      <WarehousesSettingsClient
        warehouses={[warehouse()]}
        editable
        storeCountry="AU"
      />,
    );

    await user.click(screen.getByRole("button", { name: "Remove" }));
    const buttons = screen.getAllByRole("button", { name: "Remove" });
    await user.click(buttons[buttons.length - 1]);

    const status = await screen.findByRole("status");
    expect(status).toHaveTextContent("archived");
    expect(status).toHaveTextContent("12 units");
    expect(status).toHaveTextContent("can no longer be sold");
  });

  it("a plain delete does not claim anything was archived", async () => {
    const user = userEvent.setup();
    deleteWarehouse.mockResolvedValue({
      ok: true,
      data: { id: "wh-1", outcome: "deleted", units_remaining: 0 },
    });
    render(
      <WarehousesSettingsClient
        warehouses={[warehouse()]}
        editable
        storeCountry="AU"
      />,
    );

    await user.click(screen.getByRole("button", { name: "Remove" }));
    const buttons = screen.getAllByRole("button", { name: "Remove" });
    await user.click(buttons[buttons.length - 1]);

    const status = await screen.findByRole("status");
    expect(status).toHaveTextContent("was removed");
    expect(status).not.toHaveTextContent("archived");
  });

  it("a failed removal surfaces the reason instead of looking successful", async () => {
    const user = userEvent.setup();
    deleteWarehouse.mockResolvedValue({
      ok: false,
      code: "forbidden",
      message: "You do not have permission to edit warehouses.",
    });
    render(
      <WarehousesSettingsClient
        warehouses={[warehouse()]}
        editable
        storeCountry="AU"
      />,
    );

    await user.click(screen.getByRole("button", { name: "Remove" }));
    const buttons = screen.getAllByRole("button", { name: "Remove" });
    await user.click(buttons[buttons.length - 1]);

    expect(await screen.findByRole("alert")).toHaveTextContent(
      "You do not have permission",
    );
  });
});

describe("WarehousesSettingsClient — fill order", () => {
  const two = [
    warehouse({ id: "wh-1", name: "Alpha", priority: 0 }),
    warehouse({ id: "wh-2", name: "Bravo", priority: 1, is_default: false }),
  ];

  // The API rejects a delta. Sending one would 400 every reorder.
  it("moving one sends the COMPLETE reordered set", async () => {
    const user = userEvent.setup();
    reorderWarehouseList.mockResolvedValue({ ok: true });
    render(
      <WarehousesSettingsClient warehouses={two} editable storeCountry="AU" />,
    );

    await user.click(
      screen.getByRole("button", { name: "Move Bravo earlier in the fill order" }),
    );

    await waitFor(() => expect(reorderWarehouseList).toHaveBeenCalledTimes(1));
    expect(reorderWarehouseList).toHaveBeenCalledWith(["wh-2", "wh-1"]);
  });

  it("the first warehouse cannot move up and the last cannot move down", () => {
    render(
      <WarehousesSettingsClient warehouses={two} editable storeCountry="AU" />,
    );

    expect(
      screen.getByRole("button", { name: "Move Alpha earlier in the fill order" }),
    ).toBeDisabled();
    expect(
      screen.getByRole("button", { name: "Move Bravo later in the fill order" }),
    ).toBeDisabled();
  });

  // Reordering decides where an order ships from. Behind a drag handle it
  // would be unreachable without a mouse.
  it("reordering is done with real buttons, not drag-only affordances", () => {
    render(
      <WarehousesSettingsClient warehouses={two} editable storeCountry="AU" />,
    );
    expect(
      screen.getAllByRole("button", { name: /fill order/ }),
    ).toHaveLength(4);
  });
});

describe("WarehousesSettingsClient — read-only", () => {
  it("a viewer gets no write affordances at all", () => {
    render(
      <WarehousesSettingsClient
        warehouses={[warehouse()]}
        editable={false}
        storeCountry="AU"
      />,
    );

    expect(screen.queryByRole("button", { name: "Remove" })).toBeNull();
    expect(screen.queryByRole("button", { name: "Edit" })).toBeNull();
    expect(screen.queryByRole("button", { name: "Add warehouse" })).toBeNull();
  });

  it("the empty state explains why a warehouse is needed", () => {
    render(
      <WarehousesSettingsClient warehouses={[]} editable storeCountry="AU" />,
    );
    expect(
      screen.getByText(/Carriers cannot quote a rate without an origin address/),
    ).toBeInTheDocument();
  });
});
