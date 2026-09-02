import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen } from "@testing-library/react";

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

beforeEach(() => vi.clearAllMocks());

// Save moved out of the rail and into a bar docked under the page header.
// The rail is `self-start`, so it is a short box floating beside a long
// scrolling column — which is why Save read as "dangling in the middle of
// the page". The bar is the anchor; these tests keep it from drifting back.
describe("ProductForm — the docked action bar", () => {
  it("offers exactly one save control, not one per column", () => {
    render(<ProductForm {...baseProps} mode="edit" initialProduct={product()} />);
    expect(screen.getAllByRole("button", { name: /save changes/i })).toHaveLength(1);
  });

  it("docks to the shell's published topbar height rather than to top-0", () => {
    const { container } = render(
      <ProductForm {...baseProps} mode="edit" initialProduct={product()} />,
    );
    const bar = container.querySelector(".sticky");
    expect(bar).not.toBeNull();
    // AdminShell's own topbar is `sticky top-0 z-30`. A bar at top-0 would
    // slide underneath it, so this must dock to --admin-topbar-h, which
    // AdminShell publishes on the ancestor of both the topbar and <main>.
    expect(bar!.className).toContain("top-[var(--admin-topbar-h)]");
    expect(bar!.className).not.toContain("top-0");
  });

  it("keeps the bar flat — no shadow, no backdrop blur", () => {
    const { container } = render(
      <ProductForm {...baseProps} mode="edit" initialProduct={product()} />,
    );
    const bar = container.querySelector(".sticky")!;
    // Elevation and blur are what turn a docked bar into dashboard chrome.
    // The system's cue for "docked" is a single hairline.
    expect(bar.className).not.toMatch(/shadow-|backdrop-blur/);
    expect(bar.className).toContain("border-b");
  });

  it("carries Discard, and Delete only when the user may delete", () => {
    const { unmount } = render(
      <ProductForm {...baseProps} mode="edit" initialProduct={product()} />,
    );
    expect(screen.getByRole("link", { name: /discard/i })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /delete product/i })).not.toBeInTheDocument();
    unmount();

    render(
      <ProductForm
        {...baseProps}
        canDelete
        mode="edit"
        initialProduct={product()}
      />,
    );
    expect(screen.getByRole("button", { name: /delete product/i })).toBeInTheDocument();
  });

  it("does not appear on the create form, which has no scroll to get lost in", () => {
    const { container } = render(<ProductForm {...baseProps} mode="create" />);
    // Create is a short form. A bar that exists to answer "did I save"
    // across a long scroll has nothing to do here, and its two actions are
    // "Create as draft" / "Create and publish" instead.
    expect(container.querySelector(".sticky")).toBeNull();
    expect(screen.queryByRole("button", { name: /save changes/i })).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: /create as draft/i })).toBeInTheDocument();
  });
});

// The rail's remaining job is metadata a merchant glances at, not actions.
describe("ProductForm — the rail", () => {
  it("holds status and categories and no longer holds the actions", () => {
    render(<ProductForm {...baseProps} canDelete mode="edit" initialProduct={product()} />);
    const rail = screen.getByRole("complementary");
    expect(rail).toBeInTheDocument();
    expect(rail.querySelector("button[type=submit]")).toBeNull();
    expect(rail.textContent).not.toMatch(/discard/i);
    expect(rail.textContent).not.toMatch(/delete/i);
  });

  it("is not sticky and draws no vertical rule", () => {
    render(<ProductForm {...baseProps} mode="edit" initialProduct={product()} />);
    const rail = screen.getByRole("complementary");
    // --border-subtle is --paper-300 on a --paper-200 page: ~2% luminance at
    // 1px, so this rule rendered as nothing at all in production while still
    // reading as sidebar-panel chrome in the markup.
    expect(rail.className).not.toMatch(/border-l/);
    expect(rail.className).not.toMatch(/sticky|self-start/);
  });
});
