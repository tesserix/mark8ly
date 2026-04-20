// apps/admin/lib/api/settings-api.ts
//
// Typed API client for payment, shipping, and tax settings endpoints.
// Follows the same calling convention as marketplace-api.ts — server
// components pass SessionHeaders from getServerSessionContext().

import type { SessionHeaders, MutationResult, MutationError } from "./marketplace-api";

const MARKETPLACE_API_URL =
  process.env.MARKETPLACE_API_URL ?? "http://localhost:8088";

// ─────────────────────────────────────────────────────────────────────────
// Wire DTOs — match the backend JSON shapes in settings.go
// ─────────────────────────────────────────────────────────────────────────

export interface PaymentConfig {
  id: string;
  provider: string;
  api_key: string; // masked
  secret_key: string; // masked
  webhook_secret: string; // masked; empty when not configured
  mode: "test" | "live";
  is_active: boolean;
  created_at: string;
  updated_at: string;
}

export interface PaymentConfigUpsertInput {
  api_key: string;
  secret_key?: string;
  // webhook_secret semantics match the backend pointer:
  //   undefined (omit)    → preserve existing value
  //   ""       (empty)    → clear the column (fall back to API secret)
  //   "sk_..." (non-empty) → set the dedicated webhook signing secret
  webhook_secret?: string;
  mode: "test" | "live";
  is_active: boolean;
}

export interface TestConnectionResult {
  success: boolean;
  error?: string;
}

export interface ShippingConfig {
  id: string;
  provider: string;
  api_key: string; // masked
  secret_key: string; // masked
  mode: "test" | "live";
  enabled: boolean;
  handling_fee: string;
  free_shipping_threshold: string;
  warehouse_name?: string;
  warehouse_line1?: string;
  warehouse_line2?: string;
  warehouse_city?: string;
  warehouse_region?: string;
  warehouse_postal?: string;
  warehouse_country?: string;
  warehouse_phone?: string;
  /** Whether a Delhivery pickup is auto-scheduled when a label is created. */
  auto_schedule_pickup?: boolean;
  /** "HH:MM:SS" — pickup slot start; fed back to Delhivery as pickup_time. */
  default_pickup_slot_start?: string;
  /** "HH:MM:SS" — UI-only slot end, rendered in the admin pickup row. */
  default_pickup_slot_end?: string;
  created_at: string;
  updated_at: string;
}

export interface ShippingConfigUpsertInput {
  api_key: string;
  secret_key?: string;
  mode: "test" | "live";
  is_active: boolean;
  handling_fee: number;
  free_shipping_min: number;
  warehouse_name?: string;
  warehouse_line1?: string;
  warehouse_line2?: string;
  warehouse_city?: string;
  warehouse_region?: string;
  warehouse_postal?: string;
  warehouse_country?: string;
  warehouse_phone?: string;
  auto_schedule_pickup?: boolean;
  default_pickup_slot_start?: string;
  default_pickup_slot_end?: string;
}

export interface TaxConfig {
  strategy: string;
  rate?: number;
  taxjar?: {
    configured: boolean;
    api_key?: string;
    mode?: string;
    is_active: boolean;
  };
  india_gst?: {
    description: string;
  };
}

export interface TaxJarUpsertInput {
  api_key: string;
  mode: "test" | "live";
}

// SupportedProviders is the country-driven catalogue of payment gateways,
// shipping carriers, and tax strategy allowed for a store. The admin UI
// renders one configuration card per entry in payment_providers and
// shipping_carriers, turning the previously empty state into a live picker.
export interface SupportedProviders {
  country_code: string;
  country_name: string;
  currency_code: string;
  payment_providers: string[];
  shipping_carriers: string[];
  tax_strategy: string;
  tax_rate?: number;
}

// ─────────────────────────────────────────────────────────────────────────
// Helpers — same patterns as marketplace-api.ts
// ─────────────────────────────────────────────────────────────────────────

function commonHeaders(session: SessionHeaders): HeadersInit {
  return {
    "X-User-Id": session.userId,
    "X-Tenant-Id": session.tenantId,
    Accept: "application/json",
    "Content-Type": "application/json",
  };
}

function readHeaders(session: SessionHeaders): HeadersInit {
  return {
    "X-User-Id": session.userId,
    "X-Tenant-Id": session.tenantId,
    Accept: "application/json",
  };
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

function settingsUrl(storeId: string, path: string): string {
  return `${MARKETPLACE_API_URL}/api/v1/admin/stores/${storeId}/settings/${path}`;
}

// ─────────────────────────────────────────────────────────────────────────
// Settings metadata — country-driven provider catalogue
// ─────────────────────────────────────────────────────────────────────────

// getSupportedProviders fetches the payment providers, shipping carriers,
// and tax strategy allowed for the store's country. Returns null when the
// store or country is not configured — the UI renders an error state.
export async function getSupportedProviders(
  storeId: string,
  session: SessionHeaders,
): Promise<SupportedProviders | null> {
  const res = await fetch(settingsUrl(storeId, "supported-providers"), {
    cache: "no-store",
    headers: readHeaders(session),
  });
  if (res.status === 401 || res.status === 403 || res.status === 404) {
    return null;
  }
  if (!res.ok) {
    throw new Error(`marketplace-api: getSupportedProviders ${res.status}`);
  }
  const body = (await res.json()) as { data: SupportedProviders };
  return body.data ?? null;
}

// ─────────────────────────────────────────────────────────────────────────
// Payment settings
// ─────────────────────────────────────────────────────────────────────────

export async function listPaymentConfigs(
  storeId: string,
  session: SessionHeaders,
): Promise<PaymentConfig[]> {
  const res = await fetch(settingsUrl(storeId, "payments"), {
    cache: "no-store",
    headers: readHeaders(session),
  });
  if (res.status === 401 || res.status === 403 || res.status === 404) {
    return [];
  }
  if (!res.ok) {
    throw new Error(`marketplace-api: listPaymentConfigs ${res.status}`);
  }
  const body = (await res.json()) as { data: PaymentConfig[] };
  return body.data ?? [];
}

export async function upsertPaymentConfig(
  storeId: string,
  provider: string,
  body: PaymentConfigUpsertInput,
  session: SessionHeaders,
): Promise<MutationResult<PaymentConfig>> {
  const res = await fetch(
    settingsUrl(storeId, `payments/${encodeURIComponent(provider)}`),
    {
      method: "PUT",
      cache: "no-store",
      headers: commonHeaders(session),
      body: JSON.stringify(body),
    },
  );
  if (!res.ok) {
    return { ok: false, error: await parseMutationError(res) };
  }
  const data = (await res.json()) as { data: PaymentConfig };
  return { ok: true, data: data.data };
}

export async function deletePaymentConfig(
  storeId: string,
  provider: string,
  session: SessionHeaders,
): Promise<MutationResult<true>> {
  const res = await fetch(
    settingsUrl(storeId, `payments/${encodeURIComponent(provider)}`),
    {
      method: "DELETE",
      cache: "no-store",
      headers: {
        "X-User-Id": session.userId,
        "X-Tenant-Id": session.tenantId,
      },
    },
  );
  if (res.status === 204 || res.ok) {
    return { ok: true, data: true };
  }
  return { ok: false, error: await parseMutationError(res) };
}

export async function testPaymentConnection(
  storeId: string,
  provider: string,
  session: SessionHeaders,
): Promise<TestConnectionResult> {
  const res = await fetch(
    settingsUrl(storeId, `payments/${encodeURIComponent(provider)}/test`),
    {
      method: "POST",
      cache: "no-store",
      headers: commonHeaders(session),
    },
  );
  if (!res.ok) {
    return { success: false, error: `Request failed with status ${res.status}` };
  }
  const body = (await res.json()) as { data: TestConnectionResult };
  return body.data;
}

// ─────────────────────────────────────────────────────────────────────────
// Shipping settings
// ─────────────────────────────────────────────────────────────────────────

export async function listShippingConfigs(
  storeId: string,
  session: SessionHeaders,
): Promise<ShippingConfig[]> {
  const res = await fetch(settingsUrl(storeId, "shipping"), {
    cache: "no-store",
    headers: readHeaders(session),
  });
  if (res.status === 401 || res.status === 403 || res.status === 404) {
    return [];
  }
  if (!res.ok) {
    throw new Error(`marketplace-api: listShippingConfigs ${res.status}`);
  }
  const body = (await res.json()) as { data: ShippingConfig[] };
  return body.data ?? [];
}

export async function upsertShippingConfig(
  storeId: string,
  provider: string,
  body: ShippingConfigUpsertInput,
  session: SessionHeaders,
): Promise<MutationResult<ShippingConfig>> {
  const res = await fetch(
    settingsUrl(storeId, `shipping/${encodeURIComponent(provider)}`),
    {
      method: "PUT",
      cache: "no-store",
      headers: commonHeaders(session),
      body: JSON.stringify(body),
    },
  );
  if (!res.ok) {
    return { ok: false, error: await parseMutationError(res) };
  }
  const data = (await res.json()) as { data: ShippingConfig };
  return { ok: true, data: data.data };
}

export async function deleteShippingConfig(
  storeId: string,
  provider: string,
  session: SessionHeaders,
): Promise<MutationResult<true>> {
  const res = await fetch(
    settingsUrl(storeId, `shipping/${encodeURIComponent(provider)}`),
    {
      method: "DELETE",
      cache: "no-store",
      headers: {
        "X-User-Id": session.userId,
        "X-Tenant-Id": session.tenantId,
      },
    },
  );
  if (res.status === 204 || res.ok) {
    return { ok: true, data: true };
  }
  return { ok: false, error: await parseMutationError(res) };
}

// ─────────────────────────────────────────────────────────────────────────
// Tax settings
// ─────────────────────────────────────────────────────────────────────────

export async function getTaxConfig(
  storeId: string,
  session: SessionHeaders,
): Promise<TaxConfig | null> {
  const res = await fetch(settingsUrl(storeId, "tax"), {
    cache: "no-store",
    headers: readHeaders(session),
  });
  if (res.status === 401 || res.status === 403 || res.status === 404) {
    return null;
  }
  if (!res.ok) {
    throw new Error(`marketplace-api: getTaxConfig ${res.status}`);
  }
  const body = (await res.json()) as { data: TaxConfig };
  return body.data;
}

export async function upsertTaxJarConfig(
  storeId: string,
  body: TaxJarUpsertInput,
  session: SessionHeaders,
): Promise<MutationResult<true>> {
  const res = await fetch(
    settingsUrl(storeId, "tax/taxjar"),
    {
      method: "PUT",
      cache: "no-store",
      headers: commonHeaders(session),
      body: JSON.stringify(body),
    },
  );
  if (!res.ok) {
    return { ok: false, error: await parseMutationError(res) };
  }
  return { ok: true, data: true };
}
