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
