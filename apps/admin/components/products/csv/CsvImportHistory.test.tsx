import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { render, screen } from "@testing-library/react";

import { CsvImportHistory } from "./CsvImportHistory";
import type { CsvImportJob } from "@/lib/api/csvImports";

function makeJob(overrides: Partial<CsvImportJob> = {}): CsvImportJob {
  return {
    id: "job-1",
    store_id: "s1",
    user_id: "u1",
    gcs_path: "uploads/tenant-1/products-2026-04-09.csv",
    content_hash: "abc",
    status: "completed",
    total_rows: 100,
    last_processed_row: 100,
    success_count: 95,
    error_count: 5,
    created_at: "2026-04-09T10:30:00Z",
    updated_at: "2026-04-09T10:31:00Z",
    ...overrides,
  };
}

// Mock fetch for the API route
const originalFetch = globalThis.fetch;

describe("CsvImportHistory", () => {
  beforeEach(() => {
    globalThis.fetch = vi.fn();
  });

  afterEach(() => {
    globalThis.fetch = originalFetch;
  });

  const mockedFetch = () => globalThis.fetch as ReturnType<typeof vi.fn>;

  it("renders job rows with status badges after loading", async () => {
    const jobs = [
      makeJob({ id: "job-1", status: "completed" }),
      makeJob({ id: "job-2", status: "failed", gcs_path: "uploads/tenant-1/failed-import.csv" }),
    ];

    mockedFetch().mockResolvedValue({
      ok: true,
      json: async () => ({
        data: jobs,
        meta: { page: 1, page_size: 10, total: 2, total_pages: 1 },
      }),
    } as Response);

    render(<CsvImportHistory />);

    // Wait for the async fetch to resolve
    expect(await screen.findByText("completed")).toBeInTheDocument();
    expect(await screen.findByText("failed")).toBeInTheDocument();
  });

  it("extracts filename from gcs_path", async () => {
    mockedFetch().mockResolvedValue({
      ok: true,
      json: async () => ({
        data: [makeJob({ gcs_path: "uploads/tenant-1/my-products.csv" })],
        meta: { page: 1, page_size: 10, total: 1, total_pages: 1 },
      }),
    } as Response);

    render(<CsvImportHistory />);

    expect(await screen.findByText("my-products.csv")).toBeInTheDocument();
  });

  it("renders empty state when no jobs", async () => {
    mockedFetch().mockResolvedValue({
      ok: true,
      json: async () => ({
        data: [],
        meta: { page: 1, page_size: 10, total: 0, total_pages: 0 },
      }),
    } as Response);

    render(<CsvImportHistory />);

    expect(await screen.findByText(/no import history/i)).toBeInTheDocument();
  });
});
