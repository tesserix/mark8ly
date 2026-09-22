import { readHeaders, type SessionHeaders } from "./auth-headers";
// apps/admin/lib/api/csvImports.ts
//
// Typed client for CSV import/export endpoints on marketplace-api.
// Follows the same patterns as marketplace-api.ts: session headers,
// consistent error envelopes, no-store caching, and typed returns.
//
// M7e: tasks 11-17 (admin CSV import/export UI).

const MARKETPLACE_API_URL =
  process.env.MARKETPLACE_API_URL ?? "http://localhost:8088";

// ─── Types ───────────────────────────────────────────────────────────

export type { SessionHeaders } from "./auth-headers";

export type CsvImportJobStatus =
  | "queued"
  | "running"
  | "paused"
  | "completed"
  | "failed"
  | "cancelled";

export interface CsvImportJob {
  id: string;
  store_id: string;
  user_id: string;
  gcs_path: string;
  content_hash: string;
  error_csv_gcs_path?: string;
  status: CsvImportJobStatus;
  total_rows?: number;
  last_processed_row: number;
  success_count: number;
  error_count: number;
  heartbeat_at?: string;
  created_at: string;
  updated_at: string;
}

export interface ListCsvImportsResponse {
  data: CsvImportJob[];
  meta: {
    page: number;
    page_size: number;
    total: number;
    total_pages: number;
  };
}

interface ApiError {
  error: string;
  message: string;
  details?: Record<string, unknown>;
}

interface MutationError {
  code: string;
  message: string;
  field?: string;
  details?: Record<string, unknown>;
}

type MutationResult<T> =
  | { ok: true; data: T }
  | { ok: false; error: MutationError };

type VoidResult = { ok: true } | { ok: false; error: MutationError };

// ─── Helpers ─────────────────────────────────────────────────────────

// Delegates to the shared builder: this file omitted X-Internal-Auth
// and so every CSV-import call 401'd (#890). readHeaders deliberately
// sets no Content-Type, which is also what the multipart upload needs.
function baseHeaders(session: SessionHeaders): Record<string, string> {
  return readHeaders(session);
}

async function parseMutationError(res: Response): Promise<MutationError> {
  const body = (await res.json().catch(() => null)) as ApiError | null;
  return {
    code: body?.error ?? "unknown_error",
    message: body?.message ?? `marketplace-api returned ${res.status}`,
    field:
      typeof body?.details?.field === "string" ? body.details.field : undefined,
    details: body?.details,
  };
}

function csvImportsUrl(storeId: string, suffix = ""): string {
  // The route group is storeRoute.Group("/csv-imports") — there is no
  // /products segment (routes.go:266). This built /products/csv-imports
  // and 404'd for as long as the feature has existed (#881).
  return `${MARKETPLACE_API_URL}/api/v1/admin/stores/${storeId}/csv-imports${suffix}`;
}

// ─── API functions ───────────────────────────────────────────────────

/**
 * Upload a CSV file to start an import job.
 * Sends multipart/form-data — does NOT set Content-Type so the browser
 * can attach the multipart boundary automatically.
 */
export async function submitCsvImport(
  session: SessionHeaders,
  storeId: string,
  file: File,
  columnMapping?: Record<string, string>,
): Promise<MutationResult<CsvImportJob>> {
  const formData = new FormData();
  formData.append("file", file);
  // The server rewrites the CSV's header row with this before storing it,
  // so the parser sees the columns the merchant chose. Omitted when empty:
  // a CSV whose headers are already canonical needs no mapping.
  if (columnMapping && Object.keys(columnMapping).length > 0) {
    formData.append("column_mapping", JSON.stringify(columnMapping));
  }

  const res = await fetch(csvImportsUrl(storeId), {
    method: "POST",
    cache: "no-store",
    headers: baseHeaders(session),
    body: formData,
  });

  if (!res.ok) {
    return { ok: false, error: await parseMutationError(res) };
  }
  return { ok: true, data: (await res.json()) as CsvImportJob };
}

/**
 * List CSV import jobs for a store (paginated).
 * Returns null on 401/403/404 (same no-leak pattern as listProducts).
 */
export async function listCsvImports(
  session: SessionHeaders,
  storeId: string,
  page: number,
  pageSize: number,
): Promise<ListCsvImportsResponse | null> {
  const params = new URLSearchParams({
    page: String(page),
    page_size: String(pageSize),
  });

  const res = await fetch(`${csvImportsUrl(storeId)}?${params.toString()}`, {
    cache: "no-store",
    headers: baseHeaders(session),
  });

  if (res.status === 401 || res.status === 403 || res.status === 404) {
    return null;
  }
  if (!res.ok) {
    const errBody = (await res.json().catch(() => null)) as ApiError | null;
    throw new Error(
      `marketplace-api: listCsvImports ${res.status}: ${errBody?.message ?? "unknown error"}`,
    );
  }
  return (await res.json()) as ListCsvImportsResponse;
}

/**
 * Get the status of a single CSV import job.
 * Returns null on 404 (job not found).
 */
export async function getCsvImportStatus(
  session: SessionHeaders,
  storeId: string,
  jobId: string,
): Promise<CsvImportJob | null> {
  const res = await fetch(csvImportsUrl(storeId, `/${jobId}`), {
    cache: "no-store",
    headers: baseHeaders(session),
  });

  if (res.status === 401 || res.status === 403 || res.status === 404) {
    return null;
  }
  if (!res.ok) {
    throw new Error(`marketplace-api: getCsvImportStatus ${res.status}`);
  }
  return (await res.json()) as CsvImportJob;
}

/**
 * Cancel a running or queued CSV import job.
 */
export async function cancelCsvImport(
  session: SessionHeaders,
  storeId: string,
  jobId: string,
): Promise<VoidResult> {
  const res = await fetch(csvImportsUrl(storeId, `/${jobId}/cancel`), {
    method: "POST",
    cache: "no-store",
    headers: baseHeaders(session),
  });

  if (res.status === 204 || res.ok) {
    return { ok: true };
  }
  return { ok: false, error: await parseMutationError(res) };
}

/**
 * Download the error CSV for a completed/failed import job.
 */
export async function downloadErrorCsv(
  session: SessionHeaders,
  storeId: string,
  jobId: string,
): Promise<Blob> {
  const res = await fetch(csvImportsUrl(storeId, `/${jobId}/errors`), {
    cache: "no-store",
    headers: baseHeaders(session),
  });

  if (!res.ok) {
    throw new Error(`marketplace-api: downloadErrorCsv ${res.status}`);
  }
  return res.blob();
}

/**
 * Export products as CSV. Supports filtering by product ids or query filters.
 */
export async function exportProductsCsv(
  session: SessionHeaders,
  storeId: string,
  params: { ids?: string[]; filters?: Record<string, string> },
): Promise<Blob> {
  const qs = new URLSearchParams();
  if (params.ids && params.ids.length > 0) {
    qs.set("ids", params.ids.join(","));
  }
  if (params.filters) {
    for (const [key, value] of Object.entries(params.filters)) {
      qs.set(key, value);
    }
  }

  const query = qs.toString();
  const url = `${MARKETPLACE_API_URL}/api/v1/admin/stores/${storeId}/products/export.csv${query ? `?${query}` : ""}`;

  const res = await fetch(url, {
    cache: "no-store",
    headers: baseHeaders(session),
  });

  if (!res.ok) {
    throw new Error(`marketplace-api: exportProductsCsv ${res.status}`);
  }
  return res.blob();
}
