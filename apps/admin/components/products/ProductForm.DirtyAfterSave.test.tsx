import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";

import type { AdminCategory, AdminProduct } from "@/lib/api/marketplace-api";

vi.mock("@/app/(admin)/products/actions", () => ({
  createProductAction: vi.fn(async () => ({ ok: true, data: { id: "p1" } })),
  updateProductAction: vi.fn(async () => ({ ok: true, data: { id: "p1" } })),
  deleteProductAction: vi.fn(async () => ({ ok: true })),
}));
vi.mock("next/navigation", () => ({
  useRouter: () => ({ push: vi.fn(), refresh: vi.fn() }),
}));
vi.mock("./form/MediaTab", () => ({
  MediaTab: () => <div data-testid="media-tab-stub" />,
}));

// Capture what the unsaved-changes guard is told, which is the thing
// that actually decides whether "Leave site?" fires.
const guardCalls: boolean[] = [];
vi.mock("@/lib/hooks/useUnsavedGuard", () => ({
  useUnsavedGuard: (dirty: boolean) => {
    guardCalls.push(dirty);
  },
}));

import { ProductForm } from "./ProductForm";

const categories: AdminCategory[] = [];
const baseProps = {
  storeId: "s1",
  categories,
  currencyCode: "AUD",
  storeCountryCode: "AU",
  canDelete: false,
  canArchive: false,
  session: { userId: "u1", tenantId: "t1" },
};

function product(): AdminProduct {
  return {
    id: "p1",
    title: "Bondi Beach Cotton Towel",
    handle: "bondi-beach-cotton-towel",
    description: "",
    status: "active",
    categories: [],
    options: [],
    media: [],
    variants: [
      { id: "v1", sku: "A", price: "49", inventory_quantity: 28, option_values: [] },
    ],
  } as unknown as AdminProduct;
}

beforeEach(() => {
  vi.clearAllMocks();
  guardCalls.length = 0;
});

// #1001. react-hook-form's isDirty compares against the values the form
// was last reset with, not against the server — so without a reset after
// a successful save it stays true for the life of the page. That fed the
// unsaved-changes guard, which raised the browser's "Leave site?" dialog
// on every navigation, immediately after saying "Changes saved".
//
// Worse than an annoyance: it trains merchants to click through a
// data-loss warning, so the one time it is telling the truth they
// dismiss that too.
describe("ProductForm — the dirty flag after a successful save", () => {
  it("stops reporting unsaved changes once the save succeeds", async () => {
    render(<ProductForm {...baseProps} mode="edit" initialProduct={product()} />);

    const title = screen.getByRole("textbox", { name: /^title$/i });
    fireEvent.change(title, { target: { value: "Bondi Beach Cotton Towel v2" } });

    await waitFor(() => expect(guardCalls.at(-1)).toBe(true));

    fireEvent.click(screen.getByRole("button", { name: /save changes/i }));

    // The assertion that matters: after the save resolves, the guard is
    // told the form is clean. Before the fix this stayed true forever.
    await waitFor(() => expect(guardCalls.at(-1)).toBe(false), { timeout: 3000 });
  });

  it("still reports unsaved changes for an edit that has not been saved", async () => {
    // The guard must keep working — a fix that simply never reports
    // dirty would pass the test above and lose real work.
    render(<ProductForm {...baseProps} mode="edit" initialProduct={product()} />);

    fireEvent.change(screen.getByRole("textbox", { name: /^title$/i }), {
      target: { value: "Edited but not saved" },
    });

    await waitFor(() => expect(guardCalls.at(-1)).toBe(true));
  });
});
