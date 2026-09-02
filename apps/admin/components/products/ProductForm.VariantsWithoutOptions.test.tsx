import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";

import type { AdminCategory, AdminProduct } from "@/lib/api/marketplace-api";

const updateProductAction = vi.fn(async () => ({ ok: true, data: { id: "p1" } }));

vi.mock("@/app/(admin)/products/actions", () => ({
  createProductAction: vi.fn(async () => ({ ok: true, data: { id: "p1" } })),
  updateProductAction: (...args: unknown[]) => updateProductAction(...(args as [])),
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

// A product with variants but NO options. This is not hypothetical: 8 of
// the-bondi-store's 12 products are in exactly this state, and it is the
// shape seeded catalogues arrive in.
function productWithVariantsButNoOptions(): AdminProduct {
  return {
    id: "p1",
    title: "Palm Beach Linen Robe",
    handle: "palm-beach-linen-robe",
    description: "",
    status: "active",
    categories: [],
    options: [],
    media: [],
    variants: [
      { id: "v1", sku: "ROBE-S", price: "120", inventory_quantity: 20, option_values: [] },
      { id: "v2", sku: "ROBE-M", price: "120", inventory_quantity: 20, option_values: [] },
    ],
  } as unknown as AdminProduct;
}

const baseProps = {
  storeId: "s1",
  categories,
  currencyCode: "AUD",
  storeCountryCode: "AU",
  canDelete: false,
  canArchive: false,
  session: { userId: "u1", tenantId: "t1" },
};

beforeEach(() => vi.clearAllMocks());

// generateVariants([], existing) returns every existing variant id as
// removedIds — correct when a merchant DELETES their last option, and
// catastrophic when it fires on mount for a product that simply never had
// options. The deletion is staged into removed_variant_ids, which
// updateProductAction forwards as-is, so pressing Save destroys every
// variant of the product.
describe("ProductForm — a product whose variants have no options", () => {
  it("does not stage its variants for deletion on mount", async () => {
    const user = userEvent.setup();
    render(
      <ProductForm
        {...baseProps}
        mode="edit"
        initialProduct={productWithVariantsButNoOptions()}
      />,
    );

    await user.click(screen.getByRole("button", { name: /save changes/i }));
    await waitFor(() => expect(updateProductAction).toHaveBeenCalled());

    const values = updateProductAction.mock.calls[0]![3] as {
      removed_variant_ids?: string[];
    };
    expect(values.removed_variant_ids ?? []).toEqual([]);
  });

  it("keeps the variants themselves rather than emptying the form", async () => {
    const user = userEvent.setup();
    render(
      <ProductForm
        {...baseProps}
        mode="edit"
        initialProduct={productWithVariantsButNoOptions()}
      />,
    );

    await user.click(screen.getByRole("button", { name: /save changes/i }));
    await waitFor(() => expect(updateProductAction).toHaveBeenCalled());

    const values = updateProductAction.mock.calls[0]![3] as {
      variants?: unknown[];
    };
    expect(values.variants ?? []).toHaveLength(2);
  });
});

// With no options there are no option-value pairs to compose a key from,
// so buildVariantKey returned "" for EVERY variant of these products. The
// key is both the React list key and the identity VariantsTab.handlePatch
// matches on (`v.key === key`), so a single "" shared by every row means
// editing one row's price wrote that price into all of them — silently,
// and on exactly the products that were already the most broken.
describe("ProductForm — editing one option-less variant", () => {
  it("does not write the edit into every other variant", async () => {
    const user = userEvent.setup();
    render(
      <ProductForm
        {...baseProps}
        mode="edit"
        initialProduct={productWithVariantsButNoOptions()}
      />,
    );

    const prices = await screen.findAllByLabelText("Price");
    expect(prices).toHaveLength(2);

    await user.clear(prices[0]!);
    await user.type(prices[0]!, "999");
    await user.tab();

    await user.click(screen.getByRole("button", { name: /save changes/i }));
    await waitFor(() => expect(updateProductAction).toHaveBeenCalled());

    const values = updateProductAction.mock.calls[0]![3] as {
      variants?: Array<{ price: string }>;
    };
    expect(values.variants?.map((v) => v.price)).toEqual(["999", "120"]);
  });
});

// The page lost its tabs in #539, but two empty states and both variant
// section descriptions still described the old layout — pointing at an
// "Options tab" and an "Otions tab"-era mental model, and calling
// option-less rows "combinations" when nothing is combined.
describe("ProductForm — copy for a product whose variants have no options", () => {
  it("never sends the merchant to a tab", async () => {
    render(
      <ProductForm
        {...baseProps}
        mode="edit"
        initialProduct={productWithVariantsButNoOptions()}
      />,
    );
    await screen.findByRole("button", { name: /save changes/i });
    expect(document.body.textContent).not.toMatch(/\btab\b/i);
  });

  it("calls them variants, not combinations", async () => {
    render(
      <ProductForm
        {...baseProps}
        mode="edit"
        initialProduct={productWithVariantsButNoOptions()}
      />,
    );
    expect(
      await screen.findByText(/each variant has its own price, stock and SKU/i),
    ).toBeInTheDocument();
    expect(screen.queryByText(/each combination/i)).not.toBeInTheDocument();
  });

  it("warns that adding an option replaces the existing variants", async () => {
    render(
      <ProductForm
        {...baseProps}
        mode="edit"
        initialProduct={productWithVariantsButNoOptions()}
      />,
    );
    expect(
      await screen.findByText(/adding one rebuilds the list above from scratch and replaces them/i),
    ).toBeInTheDocument();
  });

  it("keeps the combination wording for a product that really has options", async () => {
    const withOptions = {
      ...productWithVariantsButNoOptions(),
      options: [{ id: "o1", name: "Size", values: [{ id: "ov1", value: "S" }, { id: "ov2", value: "M" }] }],
      variants: [
        { id: "v1", sku: "S", price: "120", inventory_quantity: 2, option_values: [{ option_name: "Size", value: "S" }] },
        { id: "v2", sku: "M", price: "120", inventory_quantity: 2, option_values: [{ option_name: "Size", value: "M" }] },
      ],
    } as unknown as AdminProduct;

    render(<ProductForm {...baseProps} mode="edit" initialProduct={withOptions} />);
    expect(
      await screen.findByText(/each combination has its own price, stock and SKU/i),
    ).toBeInTheDocument();
    expect(screen.queryByText(/rebuilds the list above/i)).not.toBeInTheDocument();
  });
});
