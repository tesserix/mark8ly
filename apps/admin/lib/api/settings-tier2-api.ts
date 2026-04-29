// apps/admin/lib/api/settings-tier2-api.ts
//
// Typed API client for Settings Tier 2 (S1–S5) endpoints:
// S1 Account, S2 Custom Domains, S3 Subscription, S4 Audit Logs,
// S5 Notifications. Follows the same calling convention as
// marketplace-api.ts and settings-api.ts.

import type { SessionHeaders, MutationResult, MutationError } from "./marketplace-api";

const MARKETPLACE_API_URL =
  process.env.MARKETPLACE_API_URL ?? "http://localhost:8088";

// MARKETPLACE_INTERNAL_AUTH is the shared secret defense-in-depth
// header marketplace-api requires on every admin call. Missing it
// gets a blanket 401 from auth middleware before any handler runs —
// which is the bug that was hiding /admin/account, /admin/account/sessions
// and the rest of the Settings Tier 2 surface behind a stale "empty
// profile" UI even when the DB row was complete.
const MARKETPLACE_INTERNAL_AUTH =
  process.env.MARKETPLACE_INTERNAL_AUTH_SECRET ?? "";

// ─────────────────────────────────────────────────────────────────────────
// Helpers
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
  if (session.email) {
    headers["X-User-Email"] = session.email;
  }
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
  if (session.email) {
    headers["X-User-Email"] = session.email;
  }
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
// S1 — Account
// ─────────────────────────────────────────────────────────────────────────

export interface AccountProfile {
  user_id: string;
  name: string;
  email: string;
  phone: string;
  avatar_url: string;
  mfa_enabled: boolean;
  created_at: string;
}

export interface AvatarUploadURLResponse {
  upload_url: string;
  public_url: string;
  storage_key: string;
  expires_at: string;
}

export interface AccountSession {
  id: string;
  device: string;
  ip_address: string;
  last_active: string;
  current: boolean;
}

export async function getAccountProfile(
  session: SessionHeaders,
): Promise<AccountProfile | null> {
  const res = await fetch(
    `${MARKETPLACE_API_URL}/api/v1/admin/account`,
    { cache: "no-store", headers: readHeaders(session) },
  );
  if (res.status === 401 || res.status === 403 || res.status === 404) {
    return null;
  }
  if (!res.ok) {
    throw new Error(`marketplace-api: getAccountProfile ${res.status}`);
  }
  const body = (await res.json()) as
    | { data: AccountProfile }
    | AccountProfile;
  return "data" in body ? body.data : (body as AccountProfile);
}

export async function updateAccountProfile(
  body: { name?: string; phone?: string; avatar_url?: string },
  session: SessionHeaders,
): Promise<MutationResult<AccountProfile>> {
  const payload: Record<string, unknown> = {};
  if (body.name !== undefined) payload.display_name = body.name;
  if (body.phone !== undefined) payload.phone = body.phone;
  if (body.avatar_url !== undefined) payload.avatar_url = body.avatar_url;

  const res = await fetch(
    `${MARKETPLACE_API_URL}/api/v1/admin/account`,
    {
      method: "PATCH",
      cache: "no-store",
      headers: commonHeaders(session),
      body: JSON.stringify(payload),
    },
  );
  if (!res.ok) {
    return { ok: false, error: await parseMutationError(res) };
  }
  const raw = (await res.json()) as
    | { data: AccountProfile }
    | AccountProfile;
  const data = "data" in raw ? raw.data : (raw as AccountProfile);
  return { ok: true, data };
}

export async function createAvatarUploadURL(
  body: { filename: string; content_type: string },
  session: SessionHeaders,
): Promise<MutationResult<AvatarUploadURLResponse>> {
  const res = await fetch(
    `${MARKETPLACE_API_URL}/api/v1/admin/account/avatar/upload-url`,
    {
      method: "POST",
      cache: "no-store",
      headers: commonHeaders(session),
      body: JSON.stringify(body),
    },
  );
  if (!res.ok) {
    return { ok: false, error: await parseMutationError(res) };
  }
  const data = (await res.json()) as AvatarUploadURLResponse;
  return { ok: true, data };
}

export async function listAccountSessions(
  session: SessionHeaders,
): Promise<AccountSession[]> {
  const res = await fetch(
    `${MARKETPLACE_API_URL}/api/v1/admin/account/sessions`,
    { cache: "no-store", headers: readHeaders(session) },
  );
  if (res.status === 401 || res.status === 403 || res.status === 404) {
    return [];
  }
  if (!res.ok) {
    throw new Error(`marketplace-api: listAccountSessions ${res.status}`);
  }
  const body = (await res.json()) as { data: AccountSession[] };
  return body.data ?? [];
}

export interface EnableMFAResult {
  qr_code_url: string;
  otpauth_url: string;
  secret: string;
}

export async function enableMFA(
  session: SessionHeaders,
): Promise<MutationResult<EnableMFAResult>> {
  const res = await fetch(
    `${MARKETPLACE_API_URL}/api/v1/admin/account/mfa/enable`,
    {
      method: "POST",
      cache: "no-store",
      headers: commonHeaders(session),
    },
  );
  if (!res.ok) {
    return { ok: false, error: await parseMutationError(res) };
  }
  const data = (await res.json()) as { data: EnableMFAResult };
  return { ok: true, data: data.data };
}

export async function verifyMFA(
  code: string,
  session: SessionHeaders,
): Promise<MutationResult<true>> {
  const res = await fetch(
    `${MARKETPLACE_API_URL}/api/v1/admin/account/mfa/verify`,
    {
      method: "POST",
      cache: "no-store",
      headers: commonHeaders(session),
      body: JSON.stringify({ code }),
    },
  );
  if (!res.ok) {
    return { ok: false, error: await parseMutationError(res) };
  }
  return { ok: true, data: true };
}

export async function disableMFA(
  session: SessionHeaders,
): Promise<MutationResult<true>> {
  const res = await fetch(
    `${MARKETPLACE_API_URL}/api/v1/admin/account/mfa/disable`,
    {
      method: "POST",
      cache: "no-store",
      headers: commonHeaders(session),
    },
  );
  if (!res.ok) {
    return { ok: false, error: await parseMutationError(res) };
  }
  return { ok: true, data: true };
}

export async function revokeSession(
  sessionId: string,
  session: SessionHeaders,
): Promise<MutationResult<true>> {
  const res = await fetch(
    `${MARKETPLACE_API_URL}/api/v1/admin/account/sessions/${sessionId}`,
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

export async function deleteAccount(
  confirmation: string,
  session: SessionHeaders,
): Promise<MutationResult<true>> {
  const res = await fetch(
    `${MARKETPLACE_API_URL}/api/v1/admin/account`,
    {
      method: "DELETE",
      cache: "no-store",
      headers: commonHeaders(session),
      body: JSON.stringify({ confirmation }),
    },
  );
  if (res.status === 204 || res.ok) {
    return { ok: true, data: true };
  }
  return { ok: false, error: await parseMutationError(res) };
}

// ─────────────────────────────────────────────────────────────────────────
// S2 — Custom Domains
// ─────────────────────────────────────────────────────────────────────────

export type DNSMethod = "manual" | "cloudflare";

export interface CustomDomain {
  id: string;
  domain: string;
  dns_method: DNSMethod;
  cname_target?: string | null;
  status: "pending" | "verifying" | "active" | "error" | "removing";
  ssl_status: "pending" | "active" | "error";
  verified_at: string | null;
  /** Verification/registration error from the backend. Populated when status is "error"
   *  or when verification can't find the expected DNS record. */
  error?: string | null;
  created_at: string;
}

export async function listDomains(
  storeId: string,
  session: SessionHeaders,
): Promise<CustomDomain[]> {
  const res = await fetch(
    `${MARKETPLACE_API_URL}/api/v1/admin/stores/${storeId}/domains`,
    { cache: "no-store", headers: readHeaders(session) },
  );
  if (res.status === 401 || res.status === 403 || res.status === 404) {
    return [];
  }
  if (!res.ok) {
    throw new Error(`marketplace-api: listDomains ${res.status}`);
  }
  const body = (await res.json()) as { data: CustomDomain[] };
  return body.data ?? [];
}

export interface ValidateDomainResult {
  valid: boolean;
  canonical?: string;
  error?: string;
}

// Pre-flight existence check used by the Domains form. The backend
// always returns HTTP 200 with a discriminated body (valid + reason)
// so we don't need to pattern-match status codes here.
export async function validateDomain(
  storeId: string,
  domain: string,
  session: SessionHeaders,
): Promise<ValidateDomainResult> {
  const res = await fetch(
    `${MARKETPLACE_API_URL}/api/v1/admin/stores/${storeId}/domains/validate`,
    {
      method: "POST",
      cache: "no-store",
      headers: commonHeaders(session),
      body: JSON.stringify({ domain }),
    },
  );
  if (!res.ok) {
    return { valid: false, error: "Couldn't reach validation service." };
  }
  const body = (await res.json()) as ValidateDomainResult;
  return body;
}

export async function addDomain(
  storeId: string,
  body: { domain: string; dns_method: DNSMethod; cf_api_token?: string },
  session: SessionHeaders,
): Promise<MutationResult<CustomDomain>> {
  const res = await fetch(
    `${MARKETPLACE_API_URL}/api/v1/admin/stores/${storeId}/domains`,
    {
      method: "POST",
      cache: "no-store",
      headers: commonHeaders(session),
      body: JSON.stringify(body),
    },
  );
  if (!res.ok) {
    return { ok: false, error: await parseMutationError(res) };
  }
  const data = (await res.json()) as { data: CustomDomain };
  return { ok: true, data: data.data };
}

export async function removeDomain(
  storeId: string,
  domainId: string,
  session: SessionHeaders,
): Promise<MutationResult<true>> {
  const res = await fetch(
    `${MARKETPLACE_API_URL}/api/v1/admin/stores/${storeId}/domains/${domainId}`,
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

export async function verifyDomain(
  storeId: string,
  domainId: string,
  session: SessionHeaders,
): Promise<MutationResult<CustomDomain>> {
  const res = await fetch(
    `${MARKETPLACE_API_URL}/api/v1/admin/stores/${storeId}/domains/${domainId}/verify`,
    {
      method: "POST",
      cache: "no-store",
      headers: commonHeaders(session),
    },
  );
  if (!res.ok) {
    return { ok: false, error: await parseMutationError(res) };
  }
  const data = (await res.json()) as { data: CustomDomain };
  return { ok: true, data: data.data };
}

export async function refreshDomainStatus(
  storeId: string,
  domainId: string,
  session: SessionHeaders,
): Promise<MutationResult<CustomDomain>> {
  const res = await fetch(
    `${MARKETPLACE_API_URL}/api/v1/admin/stores/${storeId}/domains/${domainId}/refresh-status`,
    {
      method: "POST",
      cache: "no-store",
      headers: commonHeaders(session),
    },
  );
  if (!res.ok) {
    return { ok: false, error: await parseMutationError(res) };
  }
  const data = (await res.json()) as { data: CustomDomain };
  return { ok: true, data: data.data };
}

// ─────────────────────────────────────────────────────────────────────────
// S3 — Subscription
// ─────────────────────────────────────────────────────────────────────────

export interface StoreSubscription {
  id: string;
  plan: "free" | "starter" | "pro" | "enterprise";
  status: "active" | "trialing" | "past_due" | "cancelled" | "incomplete";
  current_period_start: string | null;
  current_period_end: string | null;
  cancel_at_period_end: boolean;
}

export async function getSubscription(
  storeId: string,
  session: SessionHeaders,
): Promise<StoreSubscription | null> {
  const res = await fetch(
    `${MARKETPLACE_API_URL}/api/v1/admin/stores/${storeId}/subscription`,
    { cache: "no-store", headers: readHeaders(session) },
  );
  if (res.status === 401 || res.status === 403 || res.status === 404) {
    return null;
  }
  if (!res.ok) {
    throw new Error(`marketplace-api: getSubscription ${res.status}`);
  }
  const body = (await res.json()) as { data: StoreSubscription };
  return body.data;
}

export async function createCheckoutSession(
  storeId: string,
  plan: string,
  session: SessionHeaders,
): Promise<MutationResult<{ checkout_url: string }>> {
  const res = await fetch(
    `${MARKETPLACE_API_URL}/api/v1/admin/stores/${storeId}/subscription/checkout`,
    {
      method: "POST",
      cache: "no-store",
      headers: commonHeaders(session),
      body: JSON.stringify({ plan }),
    },
  );
  if (!res.ok) {
    return { ok: false, error: await parseMutationError(res) };
  }
  const data = (await res.json()) as { data: { checkout_url: string } };
  return { ok: true, data: data.data };
}

export async function createPortalSession(
  storeId: string,
  session: SessionHeaders,
): Promise<MutationResult<{ portal_url: string }>> {
  const res = await fetch(
    `${MARKETPLACE_API_URL}/api/v1/admin/stores/${storeId}/subscription/portal`,
    {
      method: "POST",
      cache: "no-store",
      headers: commonHeaders(session),
    },
  );
  if (!res.ok) {
    return { ok: false, error: await parseMutationError(res) };
  }
  const data = (await res.json()) as { data: { portal_url: string } };
  return { ok: true, data: data.data };
}

// ─────────────────────────────────────────────────────────────────────────
// S4 — Audit Logs
// ─────────────────────────────────────────────────────────────────────────

export interface AuditLogEntry {
  id: string;
  timestamp: string;
  user_email: string;
  /** "user" | "system" | "api" — drives the actor label in the UI when
   *  user_email is empty (system events from storefront checkout, signup,
   *  background jobs). */
  actor_type: "user" | "system" | "api";
  action: string;
  resource_type: string;
  resource_id: string;
  status: string;
  severity: string;
  ip_address: string;
  user_agent: string;
  metadata: Record<string, unknown>;
}

export interface AuditLogsQuery {
  user?: string;
  action?: string;
  resource_type?: string;
  severity?: string;
  date_from?: string;
  date_to?: string;
  search?: string;
  page?: number;
  pageSize?: number;
}

export interface AuditLogsResponse {
  data: AuditLogEntry[];
  meta: { page: number; page_size: number; total: number; total_pages: number };
}

export async function listAuditLogs(
  storeId: string,
  query: AuditLogsQuery,
  session: SessionHeaders,
): Promise<AuditLogsResponse | null> {
  const params = new URLSearchParams();
  if (query.user) params.set("user", query.user);
  if (query.action) params.set("action", query.action);
  if (query.resource_type) params.set("resource_type", query.resource_type);
  if (query.severity) params.set("severity", query.severity);
  if (query.date_from) params.set("date_from", query.date_from);
  if (query.date_to) params.set("date_to", query.date_to);
  if (query.search) params.set("search", query.search);
  if (query.page) params.set("page", String(query.page));
  if (query.pageSize) params.set("page_size", String(query.pageSize));
  const qs = params.toString();

  const url = `${MARKETPLACE_API_URL}/api/v1/admin/stores/${storeId}/audit-logs${
    qs ? `?${qs}` : ""
  }`;

  // Never throw from a server-component reader: the audit logs page
  // is a leaf route and any unhandled throw trips the production
  // error boundary with a digest and an opaque "We couldn't load
  // these settings" screen. Return null on all failure modes and let
  // the page render a graceful empty/error state.
  try {
    const res = await fetch(url, {
      cache: "no-store",
      headers: readHeaders(session),
    });
    if (!res.ok) {
      const errBody = (await res.json().catch(() => null)) as ApiError | null;
      // eslint-disable-next-line no-console
      console.error(
        `listAuditLogs failed: ${res.status} ${errBody?.message ?? ""}`,
      );
      return null;
    }
    return (await res.json()) as AuditLogsResponse;
  } catch (err) {
    // eslint-disable-next-line no-console
    console.error(
      `listAuditLogs threw: ${err instanceof Error ? err.message : String(err)}`,
    );
    return null;
  }
}

export async function exportAuditLogsCSV(
  storeId: string,
  query: AuditLogsQuery,
  session: SessionHeaders,
): Promise<string> {
  const params = new URLSearchParams();
  if (query.user) params.set("user", query.user);
  if (query.action) params.set("action", query.action);
  if (query.resource_type) params.set("resource_type", query.resource_type);
  if (query.severity) params.set("severity", query.severity);
  if (query.date_from) params.set("date_from", query.date_from);
  if (query.date_to) params.set("date_to", query.date_to);
  if (query.search) params.set("search", query.search);
  const qs = params.toString();

  const url = `${MARKETPLACE_API_URL}/api/v1/admin/stores/${storeId}/audit-logs/export${
    qs ? `?${qs}` : ""
  }`;
  const res = await fetch(url, {
    cache: "no-store",
    headers: {
      "X-User-Id": session.userId,
      "X-Tenant-Id": session.tenantId,
      Accept: "text/csv",
    },
  });

  if (!res.ok) {
    throw new Error(`marketplace-api: exportAuditLogsCSV ${res.status}`);
  }
  return await res.text();
}

// ─────────────────────────────────────────────────────────────────────────
// S5 — Notifications
// ─────────────────────────────────────────────────────────────────────────

export interface Notification {
  id: string;
  type: string;
  title: string;
  message: string | null;
  resource_type: string | null;
  resource_id: string | null;
  is_read: boolean;
  created_at: string;
}

export interface NotificationPreferences {
  new_order: boolean;
  low_stock: boolean;
  return_requested: boolean;
  payment_received: boolean;
  review_submitted: boolean;
}

export async function listNotifications(
  storeId: string,
  unreadOnly: boolean,
  session: SessionHeaders,
): Promise<Notification[]> {
  const params = new URLSearchParams();
  if (unreadOnly) params.set("unread_only", "true");
  const qs = params.toString();

  const url = `${MARKETPLACE_API_URL}/api/v1/admin/stores/${storeId}/notifications${
    qs ? `?${qs}` : ""
  }`;
  const res = await fetch(url, {
    cache: "no-store",
    headers: readHeaders(session),
  });
  if (res.status === 401 || res.status === 403 || res.status === 404) {
    return [];
  }
  if (!res.ok) {
    throw new Error(`marketplace-api: listNotifications ${res.status}`);
  }
  const body = (await res.json()) as { notifications: Notification[] };
  return body.notifications ?? [];
}

export async function getUnreadCount(
  storeId: string,
  session: SessionHeaders,
): Promise<number> {
  const res = await fetch(
    `${MARKETPLACE_API_URL}/api/v1/admin/stores/${storeId}/notifications/unread-count`,
    { cache: "no-store", headers: readHeaders(session) },
  );
  if (!res.ok) {
    return 0;
  }
  const body = (await res.json()) as { unread_count: number };
  return body.unread_count ?? 0;
}

export async function markNotificationRead(
  storeId: string,
  notificationId: string,
  session: SessionHeaders,
): Promise<MutationResult<true>> {
  const res = await fetch(
    `${MARKETPLACE_API_URL}/api/v1/admin/stores/${storeId}/notifications/${notificationId}/read`,
    {
      method: "PATCH",
      cache: "no-store",
      headers: commonHeaders(session),
    },
  );
  if (!res.ok) {
    return { ok: false, error: await parseMutationError(res) };
  }
  return { ok: true, data: true };
}

export async function markAllNotificationsRead(
  storeId: string,
  session: SessionHeaders,
): Promise<MutationResult<true>> {
  const res = await fetch(
    `${MARKETPLACE_API_URL}/api/v1/admin/stores/${storeId}/notifications/read-all`,
    {
      method: "PATCH",
      cache: "no-store",
      headers: commonHeaders(session),
    },
  );
  if (!res.ok) {
    return { ok: false, error: await parseMutationError(res) };
  }
  return { ok: true, data: true };
}

export async function getNotificationPreferences(
  storeId: string,
  session: SessionHeaders,
): Promise<NotificationPreferences | null> {
  const res = await fetch(
    `${MARKETPLACE_API_URL}/api/v1/admin/stores/${storeId}/notification-preferences`,
    { cache: "no-store", headers: readHeaders(session) },
  );
  if (res.status === 401 || res.status === 403 || res.status === 404) {
    return null;
  }
  if (!res.ok) {
    throw new Error(`marketplace-api: getNotificationPreferences ${res.status}`);
  }
  const body = (await res.json()) as {
    store_id: string;
    preferences: NotificationPreferences;
  };
  return body.preferences ?? null;
}

export async function updateNotificationPreferences(
  storeId: string,
  prefs: Partial<NotificationPreferences>,
  session: SessionHeaders,
): Promise<MutationResult<NotificationPreferences>> {
  const res = await fetch(
    `${MARKETPLACE_API_URL}/api/v1/admin/stores/${storeId}/notification-preferences`,
    {
      method: "PATCH",
      cache: "no-store",
      headers: commonHeaders(session),
      body: JSON.stringify({ preferences: prefs }),
    },
  );
  if (!res.ok) {
    return { ok: false, error: await parseMutationError(res) };
  }
  const body = (await res.json()) as {
    store_id: string;
    preferences: NotificationPreferences;
  };
  return { ok: true, data: body.preferences };
}
