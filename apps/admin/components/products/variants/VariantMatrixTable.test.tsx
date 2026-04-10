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
  it("renders one row per variant with option cells", () => {
    render(
      <VariantMatrixTable
        variants={variants}
        currencyCode="USD"
        media={media}
        onPatch={() => {}}
      />,
    );
    expect(screen.getByText("Red")).toBeInTheDocument();
    expect(screen.getByText("Blue")).toBeInTheDocument();
    expect(screen.getByText("M")).toBeInTheDocument();
    expect(screen.getByText("L")).toBeInTheDocument();
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
