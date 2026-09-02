// apps/admin/lib/api/webhooks.ts
//
// Admin client for the outbound webhooks API (#562 task 7), mounted at
// /api/v1/admin/stores/:storeId/webhooks. Follows the same calling
// convention as coupons-api.ts / settings-api.ts: server components (or
// server actions) pass SessionHeaders, and mutations return the shared
// MutationResult<T> so callers pattern-match on `ok` instead of throwing.
//
// The one endpoint that behaves differently from every other resource in
// this app is Create: it returns the signing secret exactly once, as a
// sibling field next to the subscription — never on the subscription
// object itself, and no other endpoint here ever returns it again. Keep
// that shape (CreateWebhookResult) distinct from WebhookSubscription so a
// future edit can't accidentally start threading `secret` through list/
// patch responses.

import type { SessionHeaders, MutationResult, MutationError } from "./marketplace-api";

const MARKETPLACE_API_URL =
  process.env.MARKETPLACE_API_URL ?? "http://localhost:8088";

const MARKETPLACE_INTERNAL_AUTH =
  process.env.MARKETPLACE_INTERNAL_AUTH_SECRET ?? "";

// ─────────────────────────────────────────────────────────────────────────
// Wire DTOs — match WebhookResponse / DeliveryResponse in webhooks.go
// ─────────────────────────────────────────────────────────────────────────

export interface WebhookSubscription {
  id: string;
  url: string;
  event_types: string[];
  enabled: boolean;
  disabled_reason?: string | null;
  disabled_at?: string | null;
  created_at: string;
  updated_at: string;
}

export type DeliveryStatus = "pending" | "delivered" | "failed";

export interface WebhookDelivery {
  id: string;
  event_type: string;
  aggregate_id: string;
  status: DeliveryStatus;
  attempts: number;
  next_attempt_at: string;
  last_status_code?: number | null;
  last_error?: string | null;
  delivered_at?: string | null;
  created_at: string;
}

export interface CreateWebhookInput {
  url: string;
  event_types: string[];
}

export interface PatchWebhookInput {
  url?: string;
  event_types?: string[];
  enabled?: boolean;
}

/**
 * Returned only from createWebhook. `secret` is the one and only time the
 * signing secret ever appears on the wire — the backend never returns it
 * from any other endpoint (list, get-via-patch, etc). Do not persist it
 * anywhere beyond the one-time reveal in the UI, and never log it.
 */
export interface CreateWebhookResult {
  subscription: WebhookSubscription;
  secret: string;
}

export interface TestSendResult {
  status_code: number;
  success: boolean;
  error?: string;
}

// ─────────────────────────────────────────────────────────────────────────
// Helpers — same patterns as marketplace-api.ts / coupons-api.ts
// ─────────────────────────────────────────────────────────────────────────

interface ApiError {
  error: string;
  message: string;
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

function webhooksUrl(storeId: string, path = ""): string {
  return `${MARKETPLACE_API_URL}/api/v1/admin/stores/${storeId}/webhooks${path}`;
}

// ─────────────────────────────────────────────────────────────────────────
// Subscriptions
// ─────────────────────────────────────────────────────────────────────────

export async function listWebhooks(
  storeId: string,
  session: SessionHeaders,
): Promise<WebhookSubscription[]> {
  const res = await fetch(webhooksUrl(storeId), {
    cache: "no-store",
    headers: readHeaders(session),
  });
  if (res.status === 401 || res.status === 403 || res.status === 404) {
    return [];
  }
  if (!res.ok) {
    throw new Error(`marketplace-api: listWebhooks ${res.status}`);
  }
  const body = (await res.json()) as { data: WebhookSubscription[] };
  return body.data ?? [];
}

export async function createWebhook(
  storeId: string,
  input: CreateWebhookInput,
  session: SessionHeaders,
): Promise<MutationResult<CreateWebhookResult>> {
  const res = await fetch(webhooksUrl(storeId), {
    method: "POST",
    cache: "no-store",
    headers: commonHeaders(session),
    body: JSON.stringify(input),
  });
  if (!res.ok) {
    return { ok: false, error: await parseMutationError(res) };
  }
  const body = (await res.json()) as { data: WebhookSubscription; secret: string };
  return { ok: true, data: { subscription: body.data, secret: body.secret } };
}

export async function patchWebhook(
  storeId: string,
  webhookId: string,
  input: PatchWebhookInput,
  session: SessionHeaders,
): Promise<MutationResult<WebhookSubscription>> {
  const res = await fetch(webhooksUrl(storeId, `/${webhookId}`), {
    method: "PATCH",
    cache: "no-store",
    headers: commonHeaders(session),
    body: JSON.stringify(input),
  });
  if (!res.ok) {
    return { ok: false, error: await parseMutationError(res) };
  }
  const body = (await res.json()) as { data: WebhookSubscription };
  return { ok: true, data: body.data };
}

export async function deleteWebhook(
  storeId: string,
  webhookId: string,
  session: SessionHeaders,
): Promise<MutationResult<true>> {
  const res = await fetch(webhooksUrl(storeId, `/${webhookId}`), {
    method: "DELETE",
    cache: "no-store",
    headers: readHeaders(session),
  });
  if (res.status === 204 || res.ok) {
    return { ok: true, data: true };
  }
  return { ok: false, error: await parseMutationError(res) };
}

// ─────────────────────────────────────────────────────────────────────────
// Test send
// ─────────────────────────────────────────────────────────────────────────

/**
 * Sends a synthetic `webhook.test` delivery and reports what came back.
 * The endpoint always answers 200 — success/failure of the actual delivery
 * lives in the response body, including the merchant's own status code, so
 * a 403 from their endpoint surfaces as `{ status_code: 403, success:
 * false }`, not as a thrown error.
 */
export async function testSendWebhook(
  storeId: string,
  webhookId: string,
  session: SessionHeaders,
): Promise<MutationResult<TestSendResult>> {
  const res = await fetch(webhooksUrl(storeId, `/${webhookId}/test`), {
    method: "POST",
    cache: "no-store",
    headers: commonHeaders(session),
  });
  if (!res.ok) {
    return { ok: false, error: await parseMutationError(res) };
  }
  const body = (await res.json()) as { data: TestSendResult };
  return { ok: true, data: body.data };
}

// ─────────────────────────────────────────────────────────────────────────
// Deliveries
// ─────────────────────────────────────────────────────────────────────────

export async function listDeliveries(
  storeId: string,
  webhookId: string,
  session: SessionHeaders,
): Promise<WebhookDelivery[]> {
  const res = await fetch(webhooksUrl(storeId, `/${webhookId}/deliveries`), {
    cache: "no-store",
    headers: readHeaders(session),
  });
  if (res.status === 401 || res.status === 403 || res.status === 404) {
    return [];
  }
  if (!res.ok) {
    throw new Error(`marketplace-api: listDeliveries ${res.status}`);
  }
  const body = (await res.json()) as { data: WebhookDelivery[] };
  return body.data ?? [];
}

export async function replayDelivery(
  storeId: string,
  webhookId: string,
  deliveryId: string,
  session: SessionHeaders,
): Promise<MutationResult<true>> {
  const res = await fetch(
    webhooksUrl(storeId, `/${webhookId}/deliveries/${deliveryId}/replay`),
    {
      method: "POST",
      cache: "no-store",
      headers: commonHeaders(session),
    },
  );
  if (res.ok) {
    return { ok: true, data: true };
  }
  return { ok: false, error: await parseMutationError(res) };
}
