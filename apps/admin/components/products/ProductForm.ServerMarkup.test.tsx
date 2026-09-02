import { describe, it, expect, vi } from "vitest";
import { renderToString } from "react-dom/server";

import type { AdminCategory, AdminProduct } from "@/lib/api/marketplace-api";

vi.mock("@/app/(admin)/products/actions", () => ({
  createProductAction: vi.fn(),
  updateProductAction: vi.fn(),
  deleteProductAction: vi.fn(),
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
    description: "Turkish cotton, woven flat-weave.",
    status: "active",
    categories: [],
    options: [],
    media: [],
    variants: [
      {
        id: "v1",
        sku: "TBS-BBCT-100X180CM",
        price: "75",
        inventory_quantity: 28,
        option_values: [],
        weight_grams: 620,
      },
    ],
  } as unknown as AdminProduct;
}

// The page is a server component that already holds the product, but the
// form is a client component driven by react-hook-form. register() returns
// only { name, onChange, onBlur, ref } — no value, no defaultValue — so the
// server HTML shipped EMPTY inputs and the values appeared only once the
// client hydrated and RHF wrote them through its refs.
//
// The visible result was a form that looked like a product with no data, or
// a failed load: heading and breadcrumb correct immediately, Title blank and
// Handle showing its placeholder for a beat on every load.
//
// renderToString is the honest test for this: it is the server pass, with no
// hydration to paper over the gap.
describe("ProductForm — server-rendered markup", () => {
  const html = () =>
    renderToString(
      <ProductForm {...baseProps} mode="edit" initialProduct={product()} />,
    );

  it("carries the product's values before any hydration", () => {
    const markup = html();
    expect(markup).toContain('value="Bondi Beach Cotton Towel"');
    expect(markup).toContain('value="bondi-beach-cotton-towel"');
    expect(markup).toContain('value="75"');
    expect(markup).toContain('value="TBS-BBCT-100X180CM"');
    expect(markup).toContain('value="28"');
  });

  it("renders the description into the textarea, not as an empty box", () => {
    expect(html()).toContain("Turkish cotton, woven flat-weave.");
  });

  it("carries shipping values too", () => {
    // 620g -> 0.62kg, set on the form's defaults.
    expect(html()).toContain('value="0.62"');
  });
});
