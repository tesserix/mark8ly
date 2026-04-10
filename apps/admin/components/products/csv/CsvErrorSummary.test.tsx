import { describe, it, expect, vi } from "vitest";
import { render, screen } from "@testing-library/react";

import { CsvErrorSummary } from "./CsvErrorSummary";

describe("CsvErrorSummary", () => {
  it("renders download link when errorCount > 0", () => {
    render(
      <CsvErrorSummary jobId="job-1" storeId="s1" errorCount={5} />,
    );

    expect(screen.getByText("5")).toBeInTheDocument();
    expect(
      screen.getByRole("link", { name: /download error report/i }),
    ).toBeInTheDocument();
  });

  it("hides when errorCount is 0", () => {
    const { container } = render(
      <CsvErrorSummary jobId="job-1" storeId="s1" errorCount={0} />,
    );

    expect(container.innerHTML).toBe("");
  });

  it("link points to the error CSV API route", () => {
    render(
      <CsvErrorSummary jobId="job-1" storeId="s1" errorCount={3} />,
    );

    const link = screen.getByRole("link", {
      name: /download error report/i,
    });
    expect(link.getAttribute("href")).toContain("/csv-imports/job-1/errors.csv");
  });
});
