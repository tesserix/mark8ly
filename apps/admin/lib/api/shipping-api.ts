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
  /** Stamped server-side when shipment moves into "in_transit". */
  shipped_at?: string;
  /**
   * Stamped server-side when shipment transitions to "delivered".
   * Consumed by the receipt PDF as the canonical delivery timestamp.
   */
  delivered_at?: string;
  /** Delhivery pr_id / pickup_id (or "already-scheduled" sentinel). */
  pickup_request_id?: string;
  /** UTC timestamp combining pickup date + slot start. */
  pickup_scheduled_for?: string;
  /**
   * Non-fatal problems from label creation — a failed auto-pickup
   * schedule, or a failed fulfilment-status transition.
   *
   * The backend deliberately returns 200 with this set rather than
   * failing the request: by that point real labels exist at the carrier,
   * so a 502 would be worse than a warning. That trade only pays off if
   * the merchant actually sees it, which is what this field is for.
   */
  pickup_warning?: string;
  /**
   * Set when the backend has auto-cancelled/returned this shipment
   * (full refund or order cancel) or when a manual cancel was
   * requested via cancelShipment(). Omitted when no cancel/return has
   * ever been attempted on this shipment.
   */
  cancel_action?: "cancel_forward" | "rto" | "reverse_pickup";
  /** Outcome of the cancel_action above. Omitted alongside cancel_action. */
  cancel_status?: "succeeded" | "failed" | "unsupported" | "requested";
  /** Carrier or backend detail explaining a failed/unsupported outcome. */
  cancel_reason?: string;
  created_at: string;
}

/**
 * Response from the manual cancel/return endpoint. Distinct from
 * ShipmentResponse — it reports only the outcome of the cancel attempt,
 * not the full shipment record (callers refetch the shipment for that).
 */
export interface ShipmentCancelOutcome {
  shipment_id: string;
  action: "cancel_forward" | "rto" | "reverse_pickup";
  status: "succeeded" | "failed" | "unsupported" | "requested";
  reason?: string;
}

export interface CreateShipmentInput {
  provider: string;
  service: string;
}

/**
 * Input for (re)scheduling a Delhivery pickup. Both fields are
 * optional; blank date falls back to the next business day and blank
 * slot falls back to the carrier config's default_pickup_slot_start.
 */
export interface SchedulePickupInput {
  date?: string; // "YYYY-MM-DD"
  slot_start?: string; // "HH:MM:SS"
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

/**
 * Fetch the raw shipping-label PDF bytes through the admin API (which
 * signs the carrier request with the store's API token). Returns the
 * binary body as an ArrayBuffer so the caller can trigger a browser
 * download without exposing the Delhivery token to the client.
 */
export async function downloadShipmentLabel(
  storeId: string,
  orderId: string,
  shipmentId: string,
  session: SessionHeaders,
): Promise<MutationResult<{ bytes: ArrayBuffer; contentType: string }>> {
  const url = `${MARKETPLACE_API_URL}/api/v1/admin/stores/${storeId}/orders/${orderId}/shipments/${shipmentId}/label`;
  const res = await fetch(url, {
    cache: "no-store",
    headers: readHeaders(session),
  });
  if (!res.ok) {
    return { ok: false, error: await parseMutationError(res) };
  }
  const bytes = await res.arrayBuffer();
  const contentType = res.headers.get("Content-Type") ?? "application/pdf";
  return { ok: true, data: { bytes, contentType } };
}

/**
 * Email the shipping-label PDF to a nominated address (e.g. the 3PL
 * warehouse operator). The backend fetches the PDF from the carrier
 * and attaches it; no PDF bytes cross through the browser.
 */
export async function emailShipmentLabel(
  storeId: string,
  orderId: string,
  shipmentId: string,
  input: { recipient: string },
  session: SessionHeaders,
): Promise<MutationResult<{ ok: true; recipient: string }>> {
  const url = `${MARKETPLACE_API_URL}/api/v1/admin/stores/${storeId}/orders/${orderId}/shipments/${shipmentId}/label/email`;
  const res = await fetch(url, {
    method: "POST",
    cache: "no-store",
    headers: commonHeaders(session),
    body: JSON.stringify(input),
  });
  if (!res.ok) {
    return { ok: false, error: await parseMutationError(res) };
  }
  return { ok: true, data: (await res.json()) as { ok: true; recipient: string } };
}

/**
 * Pull the carrier's latest tracking status and sync it back to the
 * shipment row. Returns the (possibly updated) shipment. Used by the
 * admin "Refresh tracking" button to reflect real Delhivery state on
 * demand without waiting for a scheduled poller.
 */
export async function refreshShipmentTracking(
  storeId: string,
  orderId: string,
  shipmentId: string,
  session: SessionHeaders,
): Promise<MutationResult<ShipmentResponse>> {
  const url = `${MARKETPLACE_API_URL}/api/v1/admin/stores/${storeId}/orders/${orderId}/shipments/${shipmentId}/tracking/refresh`;
  const res = await fetch(url, {
    method: "POST",
    cache: "no-store",
    headers: commonHeaders(session),
  });
  if (!res.ok) {
    return { ok: false, error: await parseMutationError(res) };
  }
  return { ok: true, data: (await res.json()) as ShipmentResponse };
}

/**
 * Manually cancel or return a shipment (pickup cancellation, RTO, or
 * reverse pickup depending on the shipment's current lifecycle state).
 * Mirrors refreshShipmentTracking: POST, no body. Used by the admin
 * "Cancel / return shipment" text link — the merchant-triggered
 * counterpart to the backend's auto-cancel-on-refund/cancel behavior.
 * Returns only the cancel outcome; callers refetch the shipment to see
 * the updated cancel_action/cancel_status/cancel_reason fields.
 */
export async function cancelShipment(
  storeId: string,
  orderId: string,
  shipmentId: string,
  session: SessionHeaders,
): Promise<MutationResult<ShipmentCancelOutcome>> {
  const url = `${MARKETPLACE_API_URL}/api/v1/admin/stores/${storeId}/orders/${orderId}/shipments/${shipmentId}/cancel`;
  const res = await fetch(url, {
    method: "POST",
    cache: "no-store",
    headers: commonHeaders(session),
  });
  if (!res.ok) {
    return { ok: false, error: await parseMutationError(res) };
  }
  return { ok: true, data: (await res.json()) as ShipmentCancelOutcome };
}

/**
 * Request a carrier pickup for the shipment. Used by the "Reschedule
 * pickup" button in the admin order panel AND as the fallback when
 * auto-schedule-on-create missed (e.g. wallet balance low). The
 * backend accepts an empty body and applies the sensible defaults
 * (next business day, carrier config's default slot).
 */
export async function schedulePickup(
  storeId: string,
  orderId: string,
  shipmentId: string,
  input: SchedulePickupInput,
  session: SessionHeaders,
): Promise<MutationResult<ShipmentResponse>> {
  const url = `${MARKETPLACE_API_URL}/api/v1/admin/stores/${storeId}/orders/${orderId}/shipments/${shipmentId}/pickup/schedule`;
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
 * Drop a shipment record. Needed for TEST-DLV-* stubs that predate the
 * real-integration fix, and for cancel-before-pickup cases. Does NOT
 * call the carrier's cancel endpoint — Delhivery charges for cancelled
 * scheduled pickups so that remains a separate operator action.
 */
export async function deleteShipment(
  storeId: string,
  orderId: string,
  shipmentId: string,
  session: SessionHeaders,
): Promise<MutationResult<{ ok: true }>> {
  const url = `${MARKETPLACE_API_URL}/api/v1/admin/stores/${storeId}/orders/${orderId}/shipments/${shipmentId}`;
  const res = await fetch(url, {
    method: "DELETE",
    cache: "no-store",
    headers: commonHeaders(session),
  });
  if (!res.ok) {
    return { ok: false, error: await parseMutationError(res) };
  }
  return { ok: true, data: { ok: true } };
}
