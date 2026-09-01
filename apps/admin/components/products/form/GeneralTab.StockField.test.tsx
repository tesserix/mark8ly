import { describe, it, expect, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import { FormProvider, useForm } from "react-hook-form";

import type { Warehouse } from "@/lib/api/warehouses-api";
import type { ProductFormValues } from "@/lib/validation/product-form";

vi.mock("@/app/(admin)/products/actions", () => ({
  saveVariantStockByLocation: vi.fn(),
}));
vi.mock("next/navigation", () => ({ useRouter: () => ({ refresh: vi.fn() }) }));
vi.mock("../ProductCategoriesPicker", () => ({
  ProductCategoriesPicker: () => null,
}));

import { GeneralTab, type GeneralTabProps } from "./GeneralTab";

function warehouse(id: string, name: string): Warehouse {
  return {
    id, name, line1: "1 Dock Rd", city: "Bondi Beach", region: "NSW",
    postal_code: "2026", country_code: "AU", phone: "+61200000000",
    is_default: false, priority: 0,
  };
}

function Harness(props: Partial<GeneralTabProps>) {
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
      <GeneralTab
        mode="edit"
        categories={[]}
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

// The spec's rule for this slice, stated as a test: a store with one
// warehouse must see EXACTLY what it saw before. Anything else is a
// regression for every merchant who has not added a second location.
describe("GeneralTab — stock field", () => {
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
