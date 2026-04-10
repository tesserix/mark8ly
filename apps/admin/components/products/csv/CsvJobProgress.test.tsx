import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { render, screen, act } from "@testing-library/react";

import { CsvJobProgress } from "./CsvJobProgress";
import type { CsvImportJob } from "@/lib/api/csvImports";

// Mock next/navigation
vi.mock("next/navigation", () => ({
  useRouter: () => ({
    refresh: vi.fn(),
  }),
}));

function makeJob(overrides: Partial<CsvImportJob> = {}): CsvImportJob {
  return {
    id: "job-1",
    store_id: "s1",
    user_id: "u1",
    gcs_path: "uploads/test.csv",
    content_hash: "abc",
    status: "running",
    total_rows: 100,
    last_processed_row: 50,
    success_count: 45,
    error_count: 5,
    created_at: "2026-04-09T00:00:00Z",
    updated_at: "2026-04-09T00:00:00Z",
    ...overrides,
  };
}

describe("CsvJobProgress", () => {
  beforeEach(() => {
    vi.useFakeTimers();
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it("renders progress bar with row counts", () => {
    render(
      <CsvJobProgress
        job={makeJob()}
        storeId="s1"
        onCancel={vi.fn()}
      />,
    );

    expect(screen.getByText("45")).toBeInTheDocument(); // success count
    expect(screen.getByText("5")).toBeInTheDocument(); // error count
    expect(screen.getByRole("progressbar")).toBeInTheDocument();
  });

  it("shows cancel button when status is running", () => {
    render(
      <CsvJobProgress
        job={makeJob({ status: "running" })}
        storeId="s1"
        onCancel={vi.fn()}
      />,
    );

    expect(screen.getByRole("button", { name: /cancel/i })).toBeInTheDocument();
  });

  it("hides cancel button when status is completed", () => {
    render(
      <CsvJobProgress
        job={makeJob({ status: "completed" })}
        storeId="s1"
        onCancel={vi.fn()}
      />,
    );

    expect(screen.queryByRole("button", { name: /cancel/i })).not.toBeInTheDocument();
  });

  it("shows status badge with correct text", () => {
    render(
      <CsvJobProgress
        job={makeJob({ status: "completed" })}
        storeId="s1"
        onCancel={vi.fn()}
      />,
    );

    expect(screen.getByText("completed")).toBeInTheDocument();
  });

  it("shows queued status and cancel button", () => {
    render(
      <CsvJobProgress
        job={makeJob({ status: "queued", total_rows: undefined, last_processed_row: 0 })}
        storeId="s1"
        onCancel={vi.fn()}
      />,
    );

    expect(screen.getByText("queued")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /cancel/i })).toBeInTheDocument();
  });
});
