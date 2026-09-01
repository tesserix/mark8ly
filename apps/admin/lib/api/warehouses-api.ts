// apps/admin/lib/api/warehouses-api.ts
//
// Typed client for the per-store warehouse endpoints (#177 PR 5b).
//
// Warehouses live OUTSIDE /settings on the backend, and deliberately so: a
// warehouse is a property of the store, not of a carrier account. Burying it
// under a carrier's settings is exactly how it became a side effect of the
// shipping form, where a mistyped name silently created a second, stockless
// warehouse and the orders it was picked for never shipped.

import type {
  SessionHeaders,
  MutationResult,
  MutationError,
} from "./marketplace-api";

const MARKETPLACE_API_URL =
  process.env.MARKETPLACE_API_URL ?? "http://localhost:8088";

const MARKETPLACE_INTERNAL_AUTH =
  process.env.MARKETPLACE_INTERNAL_AUTH_SECRET ?? "";

// ─────────────────────────────────────────────────────────────────────────
// Wire DTOs — match WarehouseResponse in internal/handlers/admin/warehouses.go
// ─────────────────────────────────────────────────────────────────────────

export interface Warehouse {
  id: string;
  name: string;
  line1: string;
  line2?: string;
  city: string;
  region: string;
  postal_code: string;
  country_code: string;
  phone: string;
  email?: string;
  contact_person?: string;
  is_default: boolean;
  priority: number;
  archived_at?: string;
}

export interface WarehouseWriteInput {
  name: string;
  line1: string;
  line2?: string;
  city: string;
  region?: string;
  postal_code: string;
  country_code: string;
  phone: string;
  email?: string;
  contact_person?: string;
}

/**
 * What DELETE actually did.
 *
 * The API decides between deleting and archiving so the merchant does not
 * have to — a warehouse with allocation history can never be deleted
 * (order_allocations.warehouse_id is ON DELETE RESTRICT), and one holding
 * stock would otherwise be permanently unremovable. `units_remaining` is
 * the number of units the archive just stranded: archiving does not move
 * stock, and those units stop being sellable the moment the allocator
 * skips the row.
 */
export interface WarehouseRemoval {
  id: string;
  outcome: "deleted" | "archived";
  reason?: "holds_stock" | "unshipped_parcel" | "allocation_history";
  units_remaining: number;
}

// ─────────────────────────────────────────────────────────────────────────
// Helpers — same conventions as settings-api.ts
// ─────────────────────────────────────────────────────────────────────────

function readHeaders(session: SessionHeaders): HeadersInit {
  const headers: Record<string, string> = {
    "X-User-Id": session.userId,
    "X-Tenant-Id": session.tenantId,
    Accept: "application/json",
  };
  if (MARKETPLACE_INTERNAL_AUTH) {
    headers["X-Internal-Auth"] = MARKETPLACE_INTERNAL_AUTH;
  }
  return headers;
}

function writeHeaders(session: SessionHeaders): HeadersInit {
  return { ...readHeaders(session), "Content-Type": "application/json" };
}

interface ApiError {
  error: string;
  message: string;
  details?: Record<string, unknown>;
}

async function parseMutationError(res: Response): Promise<MutationError> {
  const body = (await res.json().catch(() => null)) as ApiError | null;
  return {
    code: body?.error ?? "unknown_error",
    message: body?.message ?? `marketplace-api returned ${res.status}`,
    field:
      typeof body?.details?.field === "string"
        ? (body.details.field as string)
        : undefined,
    details: body?.details,
  };
}

function warehousesUrl(storeId: string, path = ""): string {
  return `${MARKETPLACE_API_URL}/api/v1/admin/stores/${storeId}/warehouses${path}`;
}

// ─────────────────────────────────────────────────────────────────────────
// Reads
// ─────────────────────────────────────────────────────────────────────────

export async function listWarehouses(
  storeId: string,
  session: SessionHeaders,
): Promise<Warehouse[]> {
  const res = await fetch(warehousesUrl(storeId), {
    cache: "no-store",
    headers: readHeaders(session),
  });
  if (res.status === 401 || res.status === 403 || res.status === 404) {
    return [];
  }
  if (!res.ok) {
    throw new Error(`marketplace-api: listWarehouses ${res.status}`);
  }
  const body = (await res.json()) as { data: Warehouse[] };
  return body.data ?? [];
}

// ─────────────────────────────────────────────────────────────────────────
// Writes
//
// Every one of these goes through writeHeaders — including the DELETE.
// A hand-rolled header literal on a DELETE is what made removing a payment
// or shipping config 401 for months while the UI showed nothing (#523).
// ─────────────────────────────────────────────────────────────────────────

export async function createWarehouse(
  storeId: string,
  input: WarehouseWriteInput,
  session: SessionHeaders,
): Promise<MutationResult<Warehouse>> {
  const res = await fetch(warehousesUrl(storeId), {
    method: "POST",
    cache: "no-store",
    headers: writeHeaders(session),
    body: JSON.stringify(input),
  });
  if (!res.ok) {
    return { ok: false, error: await parseMutationError(res) };
  }
  const body = (await res.json()) as { data: Warehouse };
  return { ok: true, data: body.data };
}

export async function updateWarehouse(
  storeId: string,
  id: string,
  input: WarehouseWriteInput,
  session: SessionHeaders,
): Promise<MutationResult<Warehouse>> {
  const res = await fetch(warehousesUrl(storeId, `/${encodeURIComponent(id)}`), {
    method: "PATCH",
    cache: "no-store",
    headers: writeHeaders(session),
    body: JSON.stringify(input),
  });
  if (!res.ok) {
    return { ok: false, error: await parseMutationError(res) };
  }
  const body = (await res.json()) as { data: Warehouse };
  return { ok: true, data: body.data };
}

export async function removeWarehouse(
  storeId: string,
  id: string,
  session: SessionHeaders,
): Promise<MutationResult<WarehouseRemoval>> {
  const res = await fetch(warehousesUrl(storeId, `/${encodeURIComponent(id)}`), {
    method: "DELETE",
    cache: "no-store",
    headers: writeHeaders(session),
  });
  if (!res.ok) {
    return { ok: false, error: await parseMutationError(res) };
  }
  const body = (await res.json()) as { data: WarehouseRemoval };
  return { ok: true, data: body.data };
}

export async function setDefaultWarehouse(
  storeId: string,
  id: string,
  session: SessionHeaders,
): Promise<MutationResult<Warehouse>> {
  const res = await fetch(
    warehousesUrl(storeId, `/${encodeURIComponent(id)}/default`),
    {
      method: "PUT",
      cache: "no-store",
      headers: writeHeaders(session),
    },
  );
  if (!res.ok) {
    return { ok: false, error: await parseMutationError(res) };
  }
  const body = (await res.json()) as { data: Warehouse };
  return { ok: true, data: body.data };
}

/**
 * Reorder takes the COMPLETE ordered set of live warehouse ids, not a
 * delta — the backend rejects anything else with a 400. A delta applied to
 * a list that changed underneath (a warehouse archived in another tab)
 * reorders the wrong rows and the caller never finds out.
 */
export async function reorderWarehouses(
  storeId: string,
  order: string[],
  session: SessionHeaders,
): Promise<MutationResult<Warehouse[]>> {
  const res = await fetch(warehousesUrl(storeId, "/reorder"), {
    method: "PUT",
    cache: "no-store",
    headers: writeHeaders(session),
    body: JSON.stringify({ order }),
  });
  if (!res.ok) {
    return { ok: false, error: await parseMutationError(res) };
  }
  const body = (await res.json()) as { data: Warehouse[] };
  return { ok: true, data: body.data };
}
