import { describe, it, expect, vi } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";

import type { Warehouse } from "@/lib/api/warehouses-api";

// The real fieldset fetches countries and states through the admin's
// proxy routes; the rule under test lives in the form, not there.
vi.mock("@/components/forms/AddressFieldset", () => ({
  EMPTY_ADDRESS: {
    name: "", line1: "", line2: "", city: "", region: "", postal: "", country: "", phone: "",
  },
  AddressFieldset: () => null,
}));

import { WarehouseForm } from "./WarehouseForm";

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

describe("WarehouseForm", () => {
  it("refuses an Australian warehouse with no state, which carriers cannot quote from", async () => {
    const onSubmit = vi.fn();
    render(
      <WarehouseForm
        existing={warehouse({ region: "" })}
        storeCountry="AU"
        onSubmit={onSubmit}
        onCancel={vi.fn()}
      />,
    );

    await userEvent.click(screen.getByRole("button", { name: "Save changes" }));

    expect(screen.getByRole("alert")).toHaveTextContent(/Select a state/);
    expect(onSubmit).not.toHaveBeenCalled();
  });

  it("saves a New Zealand warehouse with no region, which carriers accept", async () => {
    const onSubmit = vi.fn().mockResolvedValue({ ok: true });
    render(
      <WarehouseForm
        existing={warehouse({ region: "", country_code: "NZ", postal_code: "1010" })}
        storeCountry="NZ"
        onSubmit={onSubmit}
        onCancel={vi.fn()}
      />,
    );

    await userEvent.click(screen.getByRole("button", { name: "Save changes" }));

    await waitFor(() => expect(onSubmit).toHaveBeenCalledOnce());
    expect(onSubmit.mock.calls[0][0]).toMatchObject({ country_code: "NZ", region: undefined });
  });
});
