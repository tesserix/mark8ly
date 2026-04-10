import { describe, it, expect } from "vitest";
import { render, screen } from "@testing-library/react";

import { CsvPreviewTable } from "./CsvPreviewTable";

describe("CsvPreviewTable", () => {
  const headers = ["title", "price", "sku"];
  const rows = [
    ["T-Shirt", "19.99", "TS-001"],
    ["Hoodie", "39.99", "HD-001"],
  ];

  it("renders column headers", () => {
    render(<CsvPreviewTable headers={headers} rows={rows} />);
    expect(screen.getByText("title")).toBeInTheDocument();
    expect(screen.getByText("price")).toBeInTheDocument();
    expect(screen.getByText("sku")).toBeInTheDocument();
  });

  it("renders data rows", () => {
    render(<CsvPreviewTable headers={headers} rows={rows} />);
    expect(screen.getByText("T-Shirt")).toBeInTheDocument();
    expect(screen.getByText("39.99")).toBeInTheDocument();
  });

  it("renders empty state when no rows", () => {
    render(<CsvPreviewTable headers={[]} rows={[]} />);
    expect(screen.getByText(/no preview/i)).toBeInTheDocument();
  });
});
