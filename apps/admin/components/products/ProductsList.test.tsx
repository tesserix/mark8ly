import { describe, it, expect, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";

// Mock next/image
vi.mock("next/image", () => ({
  default: (props: { alt: string }) => <img alt={props.alt} />,
}));

// Mock next/link
vi.mock("next/link", () => ({
  default: ({ children, href }: { children: ReactNode; href: string }) => (
    <a href={href}>{children}</a>
  ),
}));

// Mock @repo/ui components
vi.mock("@repo/ui/status-dot", () => ({
  StatusDot: ({ status }: { status: string }) => <span data-testid="status-dot">{status}</span>,
}));

vi.mock("@repo/ui/price-display", () => ({
  PriceDisplay: ({ amount }: { amount: string }) => <span data-testid="price">{amount}</span>,
}));

// Mock @tesserix/web
vi.mock("@tesserix/web", () => ({
  Dialog: ({ open, children }: { open?: boolean; children: ReactNode }) =>
    open ? <div data-testid="dialog">{children}</div> : null,
  DialogContent: ({ children }: { children: ReactNode }) => <div>{children}</div>,
  DialogHeader: ({ children }: { children: ReactNode }) => <div>{children}</div>,
  DialogTitle: ({ children }: { children: ReactNode }) => <h2>{children}</h2>,
  DialogDescription: ({ children }: { children: ReactNode }) => <p>{children}</p>,
  DialogFooter: ({ children }: { children: ReactNode }) => <div>{children}</div>,
  DialogClose: ({ children }: { children: ReactNode }) => <>{children}</>,
  AlertDialog: () => null,
}));

import { ProductsList } from "./ProductsList";
import type { AdminProduct, AdminStore, AdminCategory } from "@/lib/api/marketplace-api";

function makeProduct(overrides: Partial<AdminProduct> = {}): AdminProduct {
  return {
    id: "p1",
    store_id: "s1",
    handle: "test-product",
    title: "Test Product",
    description: null,
    status: "draft",
    tags: [],
    seo_title: null,
    seo_description: null,
    primary_category_id: null,
    copy_source_product_id: null,
    categories: [],
    options: [],
    variants: [
      {
        id: "v1",
        sku: "SKU-1",
        barcode: null,
        price: "29.99",
        compare_at_price: null,
        cost_price: null,
        currency_code: "USD",
        weight_grams: null,
        inventory_quantity: 10,
        inventory_policy: "deny",
        low_stock_threshold: null,
        option_values: [],
        position: 0,
      },
    ],
    media: [],
    published_at: null,
    created_at: "2026-01-01T00:00:00Z",
    updated_at: "2026-01-01T00:00:00Z",
    ...overrides,
  };
}

const stores: AdminStore[] = [
  { id: "s1", name: "US Store", slug: "us-store", country_code: "US", currency_code: "USD", status: "active" },
  { id: "s2", name: "UK Store", slug: "uk-store", country_code: "GB", currency_code: "GBP", status: "active" },
];

const categories: AdminCategory[] = [];

describe("ProductsList", () => {
  it("renders the table with product rows", () => {
    render(
      <ProductsList
        products={[makeProduct()]}
        storeId="s1"
        role="admin"
        stores={stores}
        categories={categories}
      />,
    );

    expect(screen.getByRole("table", { name: /products/i })).toBeInTheDocument();
    expect(screen.getByText("Test Product")).toBeInTheDocument();
  });

  it("enables checkboxes for selection", async () => {
    const user = userEvent.setup();

    render(
      <ProductsList
        products={[makeProduct(), makeProduct({ id: "p2", title: "Second", handle: "second" })]}
        storeId="s1"
        role="admin"
        stores={stores}
        categories={categories}
      />,
    );

    const checkboxes = screen.getAllByRole("checkbox");
    // Header checkbox + 2 row checkboxes
    expect(checkboxes.length).toBeGreaterThanOrEqual(2);

    // Click a row checkbox
    await user.click(checkboxes[1]!);

    // Bulk bar should appear
    expect(screen.getByText(/1 selected/i)).toBeInTheDocument();
  });

  it("shows copy to store in overflow menu", async () => {
    const user = userEvent.setup();

    render(
      <ProductsList
        products={[makeProduct()]}
        storeId="s1"
        role="admin"
        stores={stores}
        categories={categories}
      />,
    );

    const moreBtn = screen.getByLabelText(/more actions for test product/i);
    await user.click(moreBtn);

    // Overflow menu item is visible (may also appear in bulk bar if selection persists)
    const copyItems = screen.getAllByText(/copy to store/i);
    expect(copyItems.length).toBeGreaterThanOrEqual(1);
  });
});
