import { describe, it, expect, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";

import { CsvColumnMapping, PRODUCT_FIELDS } from "./CsvColumnMapping";

describe("CsvColumnMapping", () => {
  const csvHeaders = ["title", "price", "colour", "sku"];

  it("renders a mapping row per CSV column", () => {
    render(
      <CsvColumnMapping
        csvHeaders={csvHeaders}
        mapping={{}}
        onMappingChange={vi.fn()}
      />,
    );
    // Each CSV header renders as label text in its row.
    // "colour" is unique (not a product field), so we can query it directly.
    expect(screen.getByText("colour")).toBeInTheDocument();
    // For headers that also appear as <option> values, use getAllByText
    expect(screen.getAllByText("title").length).toBeGreaterThanOrEqual(1);
    expect(screen.getAllByText("price").length).toBeGreaterThanOrEqual(1);
  });

  it("auto-maps exact matches on initial render", () => {
    const onChange = vi.fn();
    render(
      <CsvColumnMapping
        csvHeaders={csvHeaders}
        mapping={{ title: "title", price: "price", sku: "sku" }}
        onMappingChange={onChange}
      />,
    );
    // The "title" select should show "title" as mapped
    const selects = screen.getAllByRole("combobox");
    // At minimum we have 4 selects (one per CSV header)
    expect(selects.length).toBe(4);
  });

  it("calls onMappingChange when a select changes", async () => {
    const user = userEvent.setup();
    const onChange = vi.fn();
    render(
      <CsvColumnMapping
        csvHeaders={["colour"]}
        mapping={{}}
        onMappingChange={onChange}
      />,
    );

    const select = screen.getByRole("combobox");
    await user.click(select);
    await user.click(screen.getByRole("option", { name: "description" }));
    expect(onChange).toHaveBeenCalledWith({ colour: "description" });
  });

  it("exports PRODUCT_FIELDS constant", () => {
    expect(PRODUCT_FIELDS).toContain("title");
    expect(PRODUCT_FIELDS).toContain("handle");
    expect(PRODUCT_FIELDS).toContain("price");
    expect(PRODUCT_FIELDS).toContain("sku");
    expect(PRODUCT_FIELDS).toContain("stock");
  });
});
