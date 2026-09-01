import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";

import type { Warehouse } from "@/lib/api/warehouses-api";
import type { VariantDraft } from "./VariantMatrixTable";

vi.mock("@/app/(admin)/products/actions", () => ({
  saveVariantStockByLocation: vi.fn().mockResolvedValue({ ok: true }),
}));
vi.mock("next/navigation", () => ({ useRouter: () => ({ refresh: vi.fn() }) }));

import { VariantRow } from "./VariantRow";

function warehouse(id: string, name: string, city: string): Warehouse {
  return {
    id, name, city, line1: "1 Dock Rd", region: "NSW", postal_code: "2026",
    country_code: "AU", phone: "+61200000000", is_default: false, priority: 0,
  };
}

const sydney = warehouse("wh-1", "Main", "Sydney");
const melbourne = warehouse("wh-2", "Overflow", "Melbourne");

function variant(overrides: Partial<VariantDraft> = {}): VariantDraft {
  return {
    id: "var-1",
    key: "Size=M",
    price: "10.00",
    sku: "SKU-1",
    stock: 12,
    weight: 1,
    optionValues: [{ optionName: "Size", value: "M" }],
    ...overrides,
  };
}

function renderRow(props: Partial<React.ComponentProps<typeof VariantRow>> = {}) {
  render(
    <table>
      <tbody>
        <VariantRow
          variant={variant()}
          optionNames={["Size"]}
          currencyCode="AUD"
          media={[]}
          onPatch={vi.fn()}
          warehouses={[sydney, melbourne]}
          storeId="store-1"
          productId="prod-1"
          stockByLocation={{ "wh-1": 7, "wh-2": 5 }}
          {...props}
        />
      </tbody>
    </table>,
  );
}

beforeEach(() => vi.clearAllMocks());

// A store with one warehouse has no split to make, and this table must
// look exactly as it did before #177.
describe("VariantRow — one warehouse", () => {
  it("keeps the plain Stock input", () => {
    renderRow({ warehouses: [sydney], stockByLocation: { "wh-1": 12 } });

    expect(screen.getByLabelText("Stock")).toHaveValue(12);
    expect(screen.queryByRole("button", { name: /Expand to edit/ })).toBeNull();
  });

  it("keeps the plain input for an unsaved variant even with two warehouses", () => {
    // No id yet: per-warehouse stock saves through the variant PATCH, which
    // needs one. The split waits until the product is saved.
    renderRow({ variant: variant({ id: undefined }) });

    expect(screen.getByLabelText("Stock")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /Expand to edit/ })).toBeNull();
  });
});

describe("VariantRow — the collapsed summary", () => {
  it("names where the units are, not how many warehouses exist", () => {
    renderRow();

    // "in 2" told the merchant nothing they could not already see.
    expect(screen.getByText("Sydney, Melbourne")).toBeInTheDocument();
    expect(screen.queryByText(/^in 2$/)).toBeNull();
  });

  it("says 'only' when a variant sits in one location", () => {
    renderRow({ stockByLocation: { "wh-1": 12 } });
    expect(screen.getByText("Sydney only")).toBeInTheDocument();
  });

  it("flags unassigned units, which is the state worth colour", () => {
    // 12 on the variant, 9 accounted for across warehouses.
    renderRow({ stockByLocation: { "wh-1": 5, "wh-2": 4 } });
    expect(screen.getByText("3 unassigned")).toBeInTheDocument();
  });

  it("truncates beyond two locations rather than overflowing the cell", () => {
    const brisbane = warehouse("wh-3", "North", "Brisbane");
    renderRow({
      warehouses: [sydney, melbourne, brisbane],
      stockByLocation: { "wh-1": 4, "wh-2": 4, "wh-3": 4 },
    });
    expect(screen.getByText("Sydney +2")).toBeInTheDocument();
  });

  // WCAG 1.4.1 — the amber on the number is never the only carrier of the
  // unassigned state.
  it("states the split in the accessible name, not only in colour", () => {
    renderRow({ stockByLocation: { "wh-1": 5, "wh-2": 4 } });

    expect(
      screen.getByRole("button", { name: /Stock, 12 units — 3 unassigned/ }),
    ).toBeInTheDocument();
  });
});

describe("VariantRow — the disclosure", () => {
  it("is a real button reporting its state, wired to the panel it controls", async () => {
    const user = userEvent.setup();
    renderRow();

    const trigger = screen.getByRole("button", { name: /Expand to edit/ });
    expect(trigger).toHaveAttribute("aria-expanded", "false");
    expect(trigger).toHaveAttribute("aria-controls", "stock-panel-var-1");

    await user.click(trigger);
    expect(trigger).toHaveAttribute("aria-expanded", "true");
    expect(screen.getByText(/Stock by warehouse/i)).toBeInTheDocument();
  });

  it("moves focus into the panel on expand", async () => {
    const user = userEvent.setup();
    renderRow();

    await user.click(screen.getByRole("button", { name: /Expand to edit/ }));
    await waitFor(() =>
      expect(screen.getByLabelText(/Main/)).toHaveFocus(),
    );
  });

  it("opens on Enter from the keyboard", async () => {
    const user = userEvent.setup();
    renderRow();

    const trigger = screen.getByRole("button", { name: /Expand to edit/ });
    trigger.focus();
    await user.keyboard("{Enter}");

    expect(trigger).toHaveAttribute("aria-expanded", "true");
  });
});
