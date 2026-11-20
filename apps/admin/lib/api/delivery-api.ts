// Typed client for own-delivery settings (marketplace-api delivery_settings.go).
// Responses are NOT wrapped in {data: ...}.

import type {
  SessionHeaders,
  MutationResult,
  MutationError,
} from "./marketplace-api";

const MARKETPLACE_API_URL =
  process.env.MARKETPLACE_API_URL ?? "http://localhost:8088";
const MARKETPLACE_INTERNAL_AUTH =
  process.env.MARKETPLACE_INTERNAL_AUTH_SECRET ?? "";

export type ShippingMode = "carrier" | "own";

export interface DeliveryOption {
  id: string;
  name: string;
  price: string;
  estimate: string | null;
  enabled: boolean;
  position: number;
}

export interface Courier {
  id: string;
  name: string;
  tracking_url_template: string | null;
}

export interface DeliverySettings {
  mode: ShippingMode;
  free_shipping_min: string | null;
  options: DeliveryOption[];
  couriers: Courier[];
}

export interface DeliveryOptionInput {
  id?: string;
  name: string;
  price: string;
  estimate?: string;
  enabled: boolean;
}

export interface CourierInput {
  name: string;
  tracking_url_template?: string;
}

function headers(session: SessionHeaders, json = false): HeadersInit {
  const h: Record<string, string> = {
    "X-User-Id": session.userId,
    "X-Tenant-Id": session.tenantId,
    Accept: "application/json",
  };
  if (json) h["Content-Type"] = "application/json";
  if (MARKETPLACE_INTERNAL_AUTH) h["X-Internal-Auth"] = MARKETPLACE_INTERNAL_AUTH;
  return h;
}

async function parseMutationError(res: Response): Promise<MutationError> {
  const body = (await res.json().catch(() => null)) as {
    error?: string;
    message?: string;
    details?: Record<string, unknown>;
  } | null;
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

function url(storeId: string, path = ""): string {
  return `${MARKETPLACE_API_URL}/api/v1/admin/stores/${storeId}/settings/delivery${path}`;
}

async function send(
  storeId: string,
  path: string,
  method: string,
  session: SessionHeaders,
  body?: unknown,
): Promise<MutationResult<true>> {
  const res = await fetch(url(storeId, path), {
    method,
    cache: "no-store",
    headers: headers(session, body !== undefined),
    body: body === undefined ? undefined : JSON.stringify(body),
  });
  if (res.ok) return { ok: true, data: true };
  return { ok: false, error: await parseMutationError(res) };
}

/** null when the store has no readable delivery settings (page falls back to carrier-only). */
export async function getDeliverySettings(
  storeId: string,
  session: SessionHeaders,
): Promise<DeliverySettings | null> {
  const res = await fetch(url(storeId), {
    cache: "no-store",
    headers: headers(session),
  });
  if (!res.ok) return null;
  const s = (await res.json()) as DeliverySettings;
  return { ...s, options: s.options ?? [], couriers: s.couriers ?? [] };
}

export const setShippingMode = (
  storeId: string,
  mode: ShippingMode,
  session: SessionHeaders,
) => send(storeId, "/mode", "PUT", session, { mode });

// free_shipping_min is required by the API (null = none): always send it.
export const saveDeliveryOptions = (
  storeId: string,
  options: DeliveryOptionInput[],
  freeShippingMin: string | null,
  session: SessionHeaders,
) =>
  send(storeId, "/options", "PUT", session, {
    options,
    free_shipping_min: freeShippingMin,
  });

export const createCourier = (
  storeId: string,
  input: CourierInput,
  session: SessionHeaders,
) => send(storeId, "/couriers", "POST", session, input);

export const updateCourier = (
  storeId: string,
  courierId: string,
  input: CourierInput,
  session: SessionHeaders,
) => send(storeId, `/couriers/${encodeURIComponent(courierId)}`, "PATCH", session, input);

export const deleteCourier = (
  storeId: string,
  courierId: string,
  session: SessionHeaders,
) => send(storeId, `/couriers/${encodeURIComponent(courierId)}`, "DELETE", session);
