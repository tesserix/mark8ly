import { describe, it, expect, vi } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import { VariantBulkBar } from "./VariantBulkBar";

describe("VariantBulkBar", () => {
  it("applies a bulk price as string", () => {
    const onBulkPatch = vi.fn();
    render(
      <VariantBulkBar variantCount={5} currencyCode="USD" onBulkPatch={onBulkPatch} />,
    );
    fireEvent.click(screen.getByRole("button", { name: /set all prices/i }));
    const input = screen.getByLabelText(/price/i);
    fireEvent.change(input, { target: { value: "29.99" } });
    fireEvent.click(screen.getByRole("button", { name: /apply/i }));
    expect(onBulkPatch).toHaveBeenCalledWith({ field: "price", value: "29.99" });
  });

  it("applies a bulk stock as number", () => {
    const onBulkPatch = vi.fn();
    render(
      <VariantBulkBar variantCount={5} currencyCode="USD" onBulkPatch={onBulkPatch} />,
    );
    fireEvent.click(screen.getByRole("button", { name: /set all stock/i }));
    const input = screen.getByLabelText(/stock/i);
    fireEvent.change(input, { target: { value: "42" } });
    fireEvent.click(screen.getByRole("button", { name: /apply/i }));
    expect(onBulkPatch).toHaveBeenCalledWith({ field: "stock", value: 42 });
  });

  it("cancel does not emit", () => {
    const onBulkPatch = vi.fn();
    render(
      <VariantBulkBar variantCount={5} currencyCode="USD" onBulkPatch={onBulkPatch} />,
    );
    fireEvent.click(screen.getByRole("button", { name: /set all prices/i }));
    fireEvent.change(screen.getByLabelText(/price/i), { target: { value: "10.00" } });
    fireEvent.click(screen.getByRole("button", { name: /cancel/i }));
    expect(onBulkPatch).not.toHaveBeenCalled();
    expect(screen.getByRole("button", { name: /set all prices/i })).toBeInTheDocument();
  });
});
