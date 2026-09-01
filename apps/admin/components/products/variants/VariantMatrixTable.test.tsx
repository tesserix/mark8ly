import { describe, it, expect, vi } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import { VariantMatrixTable, type VariantDraft } from "./VariantMatrixTable";
import type { AdminMediaResponse } from "@/lib/api/marketplace-api";

const media: AdminMediaResponse[] = [];

const variants: VariantDraft[] = [
  {
    id: "v1",
    key: "Red|M",
    price: "19.99",
    sku: "RED-M",
    stock: 10,
    weight: 0.5,
    variantImageId: null,
    optionValues: [
      { optionName: "Color", value: "Red" },
      { optionName: "Size", value: "M" },
    ],
  },
  {
    id: "v2",
    key: "Blue|L",
    price: "21.50",
    sku: "BLUE-L",
    stock: 3,
    weight: 0.6,
    variantImageId: null,
    optionValues: [
      { optionName: "Color", value: "Blue" },
      { optionName: "Size", value: "L" },
    ],
  },
];

describe("VariantMatrixTable", () => {
  it("composes every option into a single Variant cell", () => {
    render(
      <VariantMatrixTable
        variants={variants}
        currencyCode="USD"
        media={media}
        onPatch={() => {}}
      />,
    );
    // Colour and Size used to be two columns. They are now one cell, joined
    // by the interpunct, in the header's column order.
    expect(screen.getByText("Red · M")).toBeInTheDocument();
    expect(screen.getByText("Blue · L")).toBeInTheDocument();
  });

  it("shows four data columns, not one per option", () => {
    render(
      <VariantMatrixTable
        variants={variants}
        currencyCode="USD"
        media={media}
        onPatch={() => {}}
      />,
    );
    const headers = screen
      .getAllByRole("columnheader")
      .map((th) => th.textContent?.trim());
    expect(headers).toEqual(["Variant", "Price (USD)", "SKU", "Stock", "Details"]);
  });

  it("keeps weight, dimensions and image out of the row until expanded", () => {
    render(
      <VariantMatrixTable
        variants={variants}
        currencyCode="USD"
        media={media}
        onPatch={() => {}}
      />,
    );
    expect(screen.queryByLabelText("Weight")).not.toBeInTheDocument();
    expect(screen.queryByLabelText("Length (cm)")).not.toBeInTheDocument();
  });

  it("renders price input populated from variant", () => {
    render(
      <VariantMatrixTable
        variants={variants}
        currencyCode="USD"
        media={media}
        onPatch={() => {}}
      />,
    );
    expect(screen.getByDisplayValue("19.99")).toBeInTheDocument();
    expect(screen.getByDisplayValue("21.50")).toBeInTheDocument();
  });

  it("editing price + blur emits onPatch with string price", () => {
    const onPatch = vi.fn();
    render(
      <VariantMatrixTable
        variants={variants}
        currencyCode="USD"
        media={media}
        onPatch={onPatch}
      />,
    );
    const input = screen.getByDisplayValue("19.99");
    fireEvent.change(input, { target: { value: "29.99" } });
    fireEvent.blur(input);
    expect(onPatch).toHaveBeenCalledWith("Red|M", { price: "29.99" });
  });

  it("editing stock + blur emits onPatch with number", () => {
    const onPatch = vi.fn();
    render(
      <VariantMatrixTable
        variants={variants}
        currencyCode="USD"
        media={media}
        onPatch={onPatch}
      />,
    );
    const input = screen.getByDisplayValue("10");
    fireEvent.change(input, { target: { value: "42" } });
    fireEvent.blur(input);
    expect(onPatch).toHaveBeenCalledWith("Red|M", { stock: 42 });
  });
});
