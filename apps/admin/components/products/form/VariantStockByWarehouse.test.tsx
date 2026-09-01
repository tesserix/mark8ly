import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";

import type { Warehouse } from "@/lib/api/warehouses-api";
import { SENTINEL_LOCATION_ID } from "@/lib/api/marketplace-api";

const saveVariantStockByLocation = vi.fn();
const refresh = vi.fn();

vi.mock("@/app/(admin)/products/actions", () => ({
  saveVariantStockByLocation: (...args: unknown[]) =>
    saveVariantStockByLocation(...args),
}));
vi.mock("next/navigation", () => ({ useRouter: () => ({ refresh }) }));

import { VariantStockByWarehouse } from "./VariantStockByWarehouse";

function warehouse(id: string, name: string): Warehouse {
  return {
    id,
    name,
    line1: "1 Dock Rd",
    city: "Bondi Beach",
    region: "NSW",
    postal_code: "2026",
    country_code: "AU",
    phone: "+61200000000",
    is_default: id === "wh-1",
    priority: 0,
  };
}

const two = [warehouse("wh-1", "Main"), warehouse("wh-2", "Overflow")];

function renderEditor(byLocation: Record<string, number>) {
  render(
    <VariantStockByWarehouse
      storeId="store-1"
      productId="prod-1"
      variantId="var-1"
      warehouses={two}
      byLocation={byLocation}
    />,
  );
}

beforeEach(() => {
  vi.clearAllMocks();
  saveVariantStockByLocation.mockResolvedValue({ ok: true });
});

describe("VariantStockByWarehouse", () => {
  it("shows one input per warehouse, seeded from the current breakdown", () => {
    renderEditor({ "wh-1": 10, "wh-2": 5 });

    expect(screen.getByLabelText(/Main/)).toHaveValue("10");
    expect(screen.getByLabelText(/Overflow/)).toHaveValue("5");
  });

  it("the total is the sum, and follows edits", async () => {
    const user = userEvent.setup();
    renderEditor({ "wh-1": 10, "wh-2": 5 });
    expect(screen.getByText("15")).toBeInTheDocument();

    const main = screen.getByLabelText(/Main/);
    await user.clear(main);
    await user.type(main, "20");

    expect(screen.getByText("25")).toBeInTheDocument();
  });

  // Units on the sentinel are not at any warehouse yet. Folding them
  // silently into the first one would record stock somewhere the merchant
  // never said it was.
  it("calls out unassigned sentinel units without pre-filling them anywhere", () => {
    renderEditor({ [SENTINEL_LOCATION_ID]: 12 });

    expect(screen.getByRole("status")).toHaveTextContent(
      "12 units not yet assigned",
    );
    expect(screen.getByLabelText(/Main/)).toHaveValue("0");
    expect(screen.getByLabelText(/Overflow/)).toHaveValue("0");
  });

  it("says nothing about unassigned units when there are none", () => {
    renderEditor({ "wh-1": 10 });
    expect(screen.queryByRole("status")).toBeNull();
  });

  it("saves the complete map, including zeroes", async () => {
    const user = userEvent.setup();
    renderEditor({ [SENTINEL_LOCATION_ID]: 12 });

    const main = screen.getByLabelText(/Main/);
    await user.clear(main);
    await user.type(main, "12");
    await user.click(screen.getByRole("button", { name: /save stock/i }));

    await waitFor(() => expect(saveVariantStockByLocation).toHaveBeenCalled());
    expect(saveVariantStockByLocation).toHaveBeenCalledWith(
      "store-1",
      "prod-1",
      "var-1",
      { "wh-1": 12, "wh-2": 0 },
    );
  });

  it("refuses a negative quantity and sends nothing", async () => {
    const user = userEvent.setup();
    renderEditor({ "wh-1": 10, "wh-2": 5 });

    const main = screen.getByLabelText(/Main/);
    await user.clear(main);
    await user.type(main, "-3");
    await user.click(screen.getByRole("button", { name: /save stock/i }));

    expect(await screen.findByRole("alert")).toHaveTextContent(
      "whole number, zero or more",
    );
    expect(saveVariantStockByLocation).not.toHaveBeenCalled();
  });

  it("surfaces a failed save instead of reporting success", async () => {
    const user = userEvent.setup();
    saveVariantStockByLocation.mockResolvedValue({
      ok: false,
      error: { code: "validation_failed", message: "unknown or archived warehouse" },
    });
    renderEditor({ "wh-1": 10, "wh-2": 5 });

    await user.click(screen.getByRole("button", { name: /save stock/i }));

    expect(await screen.findByRole("alert")).toHaveTextContent(
      "unknown or archived warehouse",
    );
    expect(screen.queryByText("Stock saved.")).toBeNull();
  });
});
