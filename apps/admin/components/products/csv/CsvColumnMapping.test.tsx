import { describe, it, expect, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ChangeEvent, ReactNode } from "react";

vi.mock("@tesserix/web", () => ({
  Select: ({
    value,
    onValueChange,
    children,
  }: {
    value: string;
    onValueChange: (value: string) => void;
    children: ReactNode;
  }) => (
    <select
      value={value}
      onChange={(event: ChangeEvent<HTMLSelectElement>) =>
        onValueChange(event.target.value)
      }
    >
      {children}
    </select>
  ),
  SelectTrigger: () => null,
  SelectValue: () => null,
  SelectContent: ({ children }: { children: ReactNode }) => <>{children}</>,
  SelectItem: ({
    value,
    children,
  }: {
    value: string;
    children: ReactNode;
  }) => <option value={value}>{children}</option>,
}));

import { readFileSync } from "node:fs";
import path from "node:path";

import {
  CsvColumnMapping,
  PRODUCT_FIELDS,
  autoMapColumns,
} from "./CsvColumnMapping";

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
        mapping={{ title: "title", price: "base_price", sku: "sku" }}
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
    await user.selectOptions(select, "description");
    expect(onChange).toHaveBeenCalledWith({ colour: "description" });
  });

  it("exports PRODUCT_FIELDS constant", () => {
    expect(PRODUCT_FIELDS).toContain("title");
    expect(PRODUCT_FIELDS).toContain("handle");
    expect(PRODUCT_FIELDS).toContain("base_price");
    expect(PRODUCT_FIELDS).toContain("sku");
    expect(PRODUCT_FIELDS).toContain("stock");
  });

  // The mapper offered "price" while the Go parser reads "base_price", so
  // that column could never map and auto-mapping silently left it on
  // skip. Every offered field must be one the parser actually reads.
  it("offers only the column names the Go parser reads", () => {
    const parserColumns = new Set(
      readFileSync(
        path.join(
          __dirname,
          "..","..","..","..","..",
          "services","marketplace-api","internal","csvjob","parser.go",
        ),
        "utf8",
      )
        .split("\n")
        .map((line) => /^\s*col[A-Za-z]+\s*=\s*"([a-z_]+)"/.exec(line)?.[1])
        .filter((v): v is string => Boolean(v)),
    );

    expect(parserColumns.size).toBeGreaterThan(0);
    for (const field of PRODUCT_FIELDS) {
      expect(parserColumns, `mapper offers "${field}"`).toContain(field);
    }
  });
});

describe("autoMapColumns", () => {
  it("maps canonical headers", () => {
    expect(autoMapColumns(["title", "handle", "base_price"])).toEqual({
      title: "title",
      handle: "handle",
      base_price: "base_price",
    });
  });

  // Mark8ly's pitch is migrating off Shopify and Woo, and neither writes
  // our column names.
  it("maps common merchant export headers", () => {
    expect(autoMapColumns(["Product Name", "URL Key", "Variant Price"])).toEqual({
      "Product Name": "title",
      "URL Key": "handle",
      "Variant Price": "base_price",
    });
  });

  // The server rejects two headers claiming one field, so auto-mapping
  // must not produce a mapping that cannot be submitted.
  it("never maps two headers onto the same field", () => {
    const mapping = autoMapColumns(["Price", "Variant Price"]);
    const targets = Object.values(mapping);
    expect(new Set(targets).size).toBe(targets.length);
  });

  it("leaves unrecognised headers unmapped", () => {
    expect(autoMapColumns(["colour"])).toEqual({});
  });
});
