// apps/admin/lib/api/shipping-api.ts
//
// Shipping API client for admin server components/actions. Follows the
// same conventions as marketplace-api.ts: commonHeaders, parseMutationError,
// MutationResult discriminated union.

import type {
  MutationResult,
  SessionHeaders,
} from "@/lib/api/marketplace-api";

const MARKETPLACE_API_URL =
  process.env.MARKETPLACE_API_URL ?? "http://localhost:8088";

const MARKETPLACE_INTERNAL_AUTH =
  process.env.MARKETPLACE_INTERNAL_AUTH_SECRET ?? "";

// ─────────────────────────────────────────────────────────────────────────
// Types
// ─────────────────────────────────────────────────────────────────────────

export interface ShipmentResponse {
  id: string;
  order_id: string;
  provider: string;
  provider_shipment_id: string;
  tracking_number: string;
  label_url: string;
  service: string;
  status: string;
  currency_code: string;
  estimated_delivery?: string;
  created_at: string;
}

export interface CreateShipmentInput {
  provider: string;
  service: string;
}

// ─────────────────────────────────────────────────────────────────────────
// Helpers (same pattern as marketplace-api.ts)
// ─────────────────────────────────────────────────────────────────────────

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

function commonHeaders(session: SessionHeaders): HeadersInit {
  const headers: Record<string, string> = {
    "X-User-Id": session.userId,
    "X-Tenant-Id": session.tenantId,
    Accept: "application/json",
    "Content-Type": "application/json",
  };
  if (MARKETPLACE_INTERNAL_AUTH) {
    headers["X-Internal-Auth"] = MARKETPLACE_INTERNAL_AUTH;
  }
  return headers;
}

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

// ─────────────────────────────────────────────────────────────────────────
// API functions
// ─────────────────────────────────────────────────────────────────────────

/**
 * Fetch the shipment for a given order. Returns null when no shipment
 * exists (the API returns `null` with 200 in that case).
 */
export async function getOrderShipment(
  storeId: string,
  orderId: string,
  session: SessionHeaders,
): Promise<ShipmentResponse | null> {
  const url = `${MARKETPLACE_API_URL}/api/v1/admin/stores/${storeId}/orders/${orderId}/shipments`;
  const res = await fetch(url, {
    cache: "no-store",
    headers: readHeaders(session),
  });

  if (res.status === 401 || res.status === 403 || res.status === 404) {
    return null;
  }
  if (!res.ok) {
    const errBody = (await res.json().catch(() => null)) as ApiError | null;
    throw new Error(
      `marketplace-api: getOrderShipment ${res.status}: ${
        errBody?.message ?? "unknown error"
      }`,
    );
  }
  return (await res.json()) as ShipmentResponse | null;
}

/**
 * Create a shipment for an order. Calls the carrier to generate a label
 * and tracking number, then persists the record.
 */
export async function createShipment(
  storeId: string,
  orderId: string,
  input: CreateShipmentInput,
  session: SessionHeaders,
): Promise<MutationResult<ShipmentResponse>> {
  const url = `${MARKETPLACE_API_URL}/api/v1/admin/stores/${storeId}/orders/${orderId}/shipments`;
  const res = await fetch(url, {
    method: "POST",
    cache: "no-store",
    headers: commonHeaders(session),
    body: JSON.stringify(input),
  });
  if (!res.ok) {
    return { ok: false, error: await parseMutationError(res) };
  }
  return { ok: true, data: (await res.json()) as ShipmentResponse };
}

/**
 * Advance a shipment through its tracking lifecycle:
 *   in_transit → out_for_delivery → delivered
 * (exception is also accepted and does not progress the chain).
 * Writes an order_events row on the backend so the customer-facing
 * timeline reflects the change on its next poll.
 */
export async function updateShipmentStatus(
  storeId: string,
  orderId: string,
  shipmentId: string,
  input: { status: string; description?: string },
  session: SessionHeaders,
): Promise<MutationResult<ShipmentResponse>> {
  const url = `${MARKETPLACE_API_URL}/api/v1/admin/stores/${storeId}/orders/${orderId}/shipments/${shipmentId}/status`;
  const res = await fetch(url, {
    method: "PATCH",
    cache: "no-store",
    headers: commonHeaders(session),
    body: JSON.stringify(input),
  });
  if (!res.ok) {
    return { ok: false, error: await parseMutationError(res) };
  }
  return { ok: true, data: (await res.json()) as ShipmentResponse };
}
