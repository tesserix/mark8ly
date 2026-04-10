import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";

import {
  submitCsvImport,
  listCsvImports,
  getCsvImportStatus,
  cancelCsvImport,
  downloadErrorCsv,
  exportProductsCsv,
  type CsvImportJob,
  type CsvImportJobStatus,
  type SessionHeaders,
} from "./csvImports";

const mockSession: SessionHeaders = { userId: "u1", tenantId: "t1" };

function makeFetchResponse(body: unknown, status = 200): Response {
  return {
    ok: status >= 200 && status < 300,
    status,
    json: async () => body,
    blob: async () => new Blob(["csv-content"], { type: "text/csv" }),
    headers: new Headers(),
  } as unknown as Response;
}

function makeJob(overrides: Partial<CsvImportJob> = {}): CsvImportJob {
  return {
    id: "job-1",
    store_id: "s1",
    user_id: "u1",
    gcs_path: "uploads/test.csv",
    content_hash: "abc123",
    status: "queued" as CsvImportJobStatus,
    last_processed_row: 0,
    success_count: 0,
    error_count: 0,
    created_at: "2026-04-09T00:00:00Z",
    updated_at: "2026-04-09T00:00:00Z",
    ...overrides,
  };
}

describe("csvImports API client", () => {
  const originalFetch = globalThis.fetch;

  beforeEach(() => {
    globalThis.fetch = vi.fn();
  });

  afterEach(() => {
    globalThis.fetch = originalFetch;
  });

  const mockedFetch = () => globalThis.fetch as ReturnType<typeof vi.fn>;

  describe("submitCsvImport", () => {
    it("sends multipart FormData with file", async () => {
      const job = makeJob();
      mockedFetch().mockResolvedValue(makeFetchResponse(job, 201));

      const file = new File(["title,price\nT,10"], "products.csv", {
        type: "text/csv",
      });
      const result = await submitCsvImport(mockSession, "s1", file);

      expect(result).toEqual({ ok: true, data: job });
      expect(mockedFetch()).toHaveBeenCalledTimes(1);

      const [url, opts] = mockedFetch().mock.calls[0]!;
      expect(url).toContain("/api/v1/admin/stores/s1/products/csv-imports");
      expect(opts.method).toBe("POST");
      // FormData should be the body (not JSON)
      expect(opts.body).toBeInstanceOf(FormData);
      // Should NOT set Content-Type (browser sets multipart boundary)
      expect(opts.headers["Content-Type"]).toBeUndefined();
    });

    it("returns error on failure", async () => {
      mockedFetch().mockResolvedValue(
        makeFetchResponse({ error: "too_large", message: "File too large" }, 400),
      );

      const file = new File(["x"], "big.csv", { type: "text/csv" });
      const result = await submitCsvImport(mockSession, "s1", file);

      expect(result.ok).toBe(false);
      if (!result.ok) {
        expect(result.error.code).toBe("too_large");
      }
    });
  });

  describe("listCsvImports", () => {
    it("fetches paginated list", async () => {
      const body = {
        data: [makeJob()],
        meta: { page: 1, page_size: 20, total: 1, total_pages: 1 },
      };
      mockedFetch().mockResolvedValue(makeFetchResponse(body));

      const result = await listCsvImports(mockSession, "s1", 1, 20);

      expect(result).toEqual(body);
      const [url] = mockedFetch().mock.calls[0]!;
      expect(url).toContain("page=1");
      expect(url).toContain("page_size=20");
    });

    it("returns null on 404", async () => {
      mockedFetch().mockResolvedValue(makeFetchResponse(null, 404));

      const result = await listCsvImports(mockSession, "s1", 1, 20);
      expect(result).toBeNull();
    });
  });

  describe("getCsvImportStatus", () => {
    it("returns typed job object", async () => {
      const job = makeJob({ status: "running", total_rows: 100, success_count: 50 });
      mockedFetch().mockResolvedValue(makeFetchResponse(job));

      const result = await getCsvImportStatus(mockSession, "s1", "job-1");

      expect(result).toEqual(job);
      const [url] = mockedFetch().mock.calls[0]!;
      expect(url).toContain("/csv-imports/job-1");
    });

    it("returns null on 404", async () => {
      mockedFetch().mockResolvedValue(makeFetchResponse(null, 404));
      const result = await getCsvImportStatus(mockSession, "s1", "missing");
      expect(result).toBeNull();
    });
  });

  describe("cancelCsvImport", () => {
    it("posts cancel and returns ok on 204", async () => {
      mockedFetch().mockResolvedValue(makeFetchResponse(null, 204));

      const result = await cancelCsvImport(mockSession, "s1", "job-1");

      expect(result).toEqual({ ok: true });
      const [url, opts] = mockedFetch().mock.calls[0]!;
      expect(url).toContain("/csv-imports/job-1/cancel");
      expect(opts.method).toBe("POST");
    });

    it("returns error on failure", async () => {
      mockedFetch().mockResolvedValue(
        makeFetchResponse({ error: "not_cancellable", message: "Already completed" }, 409),
      );
      const result = await cancelCsvImport(mockSession, "s1", "job-1");
      expect(result.ok).toBe(false);
    });
  });

  describe("downloadErrorCsv", () => {
    it("returns blob on success", async () => {
      mockedFetch().mockResolvedValue(makeFetchResponse(null));

      const blob = await downloadErrorCsv(mockSession, "s1", "job-1");

      expect(blob).toBeInstanceOf(Blob);
      const [url] = mockedFetch().mock.calls[0]!;
      expect(url).toContain("/csv-imports/job-1/errors.csv");
    });
  });

  describe("exportProductsCsv", () => {
    it("builds query string from ids", async () => {
      mockedFetch().mockResolvedValue(makeFetchResponse(null));

      await exportProductsCsv(mockSession, "s1", { ids: ["p1", "p2"] });

      const [url] = mockedFetch().mock.calls[0]!;
      expect(url).toContain("ids=p1%2Cp2");
    });

    it("builds query string from filters", async () => {
      mockedFetch().mockResolvedValue(makeFetchResponse(null));

      await exportProductsCsv(mockSession, "s1", {
        filters: { status: "active", search: "shoes" },
      });

      const [url] = mockedFetch().mock.calls[0]!;
      expect(url).toContain("status=active");
      expect(url).toContain("search=shoes");
    });

    it("returns blob on success", async () => {
      mockedFetch().mockResolvedValue(makeFetchResponse(null));

      const blob = await exportProductsCsv(mockSession, "s1", {});
      expect(blob).toBeInstanceOf(Blob);
    });
  });
});
