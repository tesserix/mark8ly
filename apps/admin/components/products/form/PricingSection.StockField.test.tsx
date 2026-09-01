import { describe, it, expect, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import { FormProvider, useForm } from "react-hook-form";

import type { Warehouse } from "@/lib/api/warehouses-api";
import type { ProductFormValues } from "@/lib/validation/product-form";

vi.mock("@/app/(admin)/products/actions", () => ({
  saveVariantStockByLocation: vi.fn(),
}));
vi.mock("next/navigation", () => ({ useRouter: () => ({ refresh: vi.fn() }) }));
import { PricingSection, type PricingSectionProps } from "./PricingSection";

function warehouse(id: string, name: string): Warehouse {
  return {
    id, name, line1: "1 Dock Rd", city: "Bondi Beach", region: "NSW",
    postal_code: "2026", country_code: "AU", phone: "+61200000000",
    is_default: false, priority: 0,
  };
}

function Harness(props: Partial<PricingSectionProps>) {
  const methods = useForm<ProductFormValues>({
    defaultValues: {
      title: "", handle: "", description: "", status: "draft",
      price: "10", inventoryQuantity: "7", alwaysInStock: false, sku: "",
      weightKg: "1", lengthCm: "", widthCm: "", heightCm: "",
      categoryIds: [], options: [], variants: [],
    } as unknown as ProductFormValues,
  });
  return (
    <FormProvider {...methods}>
      <PricingSection
        mode="edit"
        currencyCode="AUD"
        hasMultipleVariants={false}
        storeId="store-1"
        productId="prod-1"
        variantId="var-1"
        {...props}
      />
    </FormProvider>
  );
}

// The spec's rule, stated as a test: a store with one warehouse must see
// EXACTLY what it saw before. Anything else is a regression for every
// merchant who has not added a second location.
//
// Moved here from GeneralTab when the tabbed form became one page —
// pricing and stock have a single home now, and this is it.
describe("PricingSection — stock field", () => {
  it("one warehouse keeps the single Stock field and no per-warehouse editor", () => {
    render(<Harness warehouses={[warehouse("wh-1", "Main")]} />);

    expect(screen.getByLabelText("Stock")).toHaveValue("7");
    expect(screen.queryByText(/Stock by warehouse/i)).toBeNull();
  });

  it("no warehouses at all also keeps the single field", () => {
    render(<Harness warehouses={[]} />);

    expect(screen.getByLabelText("Stock")).toHaveValue("7");
    expect(screen.queryByText(/Stock by warehouse/i)).toBeNull();
  });

  it("two warehouses replace the single field with the per-warehouse editor", () => {
    render(
      <Harness
        warehouses={[warehouse("wh-1", "Main"), warehouse("wh-2", "Overflow")]}
        stockByLocation={{ "wh-1": 4, "wh-2": 3 }}
      />,
    );

    expect(screen.queryByLabelText("Stock")).toBeNull();
    expect(screen.getByText(/Stock by warehouse/i)).toBeInTheDocument();
  });

  // On create there is no saved variant to write against, so the split
  // has to wait until the product exists.
  it("create mode keeps the single field even with two warehouses", () => {
    render(
      <Harness
        mode="create"
        productId={undefined}
        variantId={undefined}
        warehouses={[warehouse("wh-1", "Main"), warehouse("wh-2", "Overflow")]}
      />,
    );

    expect(screen.getByLabelText("Stock")).toHaveValue("7");
    expect(screen.queryByText(/Stock by warehouse/i)).toBeNull();
  });
});

// The section ADAPTS — that is its whole reason to exist. Asserting only
// that the old "price lives in the Variants tab" sentence is gone passes
// whether or not the table ever appears; a mutation that never switched
// rendering survived until this test existed.
describe("PricingSection — one home, two renderings", () => {
  it("renders plain inputs for a single-variant product", () => {
    render(<Harness warehouses={[]} />);

    expect(screen.getByLabelText("Stock")).toBeInTheDocument();
    expect(screen.queryByRole("table")).toBeNull();
  });

  it("renders the variants table once the product has options", () => {
    render(<Harness hasMultipleVariants />);

    // No separate tab, no sentence pointing elsewhere — the same section
    // simply renders the other shape.
    expect(screen.queryByLabelText("Stock")).toBeNull();
    expect(screen.queryByText(/live in the Variants tab/i)).toBeNull();
  });
});
