"use server";

// Server actions for Settings Tier 2 (S1–S5).
//
// Each action reads session headers from middleware, validates inputs,
// and delegates to the settings-tier2-api client. Returns a discriminated
// union so client components can pattern-match on .ok without try/catch.

import { headers } from "next/headers";
import { revalidatePath } from "next/cache";

import {
  updateAccountProfile,
  createAvatarUploadURL as createAvatarUploadURLApi,
  enableMFA as enableMFAApi,
  disableMFA as disableMFAApi,
  revokeSession as revokeSessionApi,
  deleteAccount as deleteAccountApi,
  addDomain as addDomainApi,
  removeDomain as removeDomainApi,
  verifyDomain as verifyDomainApi,
  refreshDomainStatus as refreshDomainStatusApi,
  createCheckoutSession as createCheckoutApi,
  createPortalSession as createPortalApi,
  markNotificationRead as markReadApi,
  markAllNotificationsRead as markAllReadApi,
  updateNotificationPreferences as updatePrefsApi,
  exportAuditLogsCSV as exportCSVApi,
} from "@/lib/api/settings-tier2-api";
import type { AvatarUploadURLResponse } from "@/lib/api/settings-tier2-api";
import type {
  NotificationPreferences,
  AuditLogsQuery,
} from "@/lib/api/settings-tier2-api";
import { canEditSettings, resolveStoreId } from "@/lib/auth/serverSession";
import type { TenantRole } from "@/lib/api/platform-api";

export type ActionResult =
  | { ok: true }
  | { ok: false; code: string; message: string };

export type ActionResultWithData<T> =
  | { ok: true; data: T }
  | { ok: false; code: string; message: string };

async function getSession() {
  const h = await headers();
  const userId = h.get("x-session-user-id") ?? "";
  const tenantId = h.get("x-session-tenant-id") ?? "";
  const role = (h.get("x-session-role") ?? "viewer") as TenantRole;
  const storeId = await resolveStoreId();
  return { userId, tenantId, role, storeId };
}

function noSession(): ActionResult {
  return { ok: false, code: "no_session", message: "Session expired. Please sign in again." };
}

function forbidden(): ActionResult {
  return { ok: false, code: "forbidden", message: "You do not have permission to perform this action." };
}

// ─── S1: Account ──────────────────────────────────────────────────────

export async function updateProfile(
  input: { name?: string; phone?: string; avatar_url?: string },
): Promise<ActionResult> {
  const { userId, tenantId } = await getSession();
  if (!userId || !tenantId) return noSession();

  const result = await updateAccountProfile(input, { userId, tenantId });
  if (!result.ok) {
    return { ok: false, code: result.error.code, message: result.error.message };
  }

  revalidatePath("/settings/account");
  return { ok: true };
}

export async function createAvatarUploadURL(
  input: { filename: string; content_type: string },
): Promise<ActionResultWithData<AvatarUploadURLResponse>> {
  const { userId, tenantId } = await getSession();
  if (!userId || !tenantId) {
    return { ok: false, code: "no_session", message: "Session expired." };
  }
  const result = await createAvatarUploadURLApi(input, { userId, tenantId });
  if (!result.ok) {
    return { ok: false, code: result.error.code, message: result.error.message };
  }
  return { ok: true, data: result.data };
}

export async function toggleMFA(
  enable: boolean,
): Promise<ActionResultWithData<{ qr_code_url?: string }>> {
  const { userId, tenantId } = await getSession();
  if (!userId || !tenantId) {
    return { ok: false, code: "no_session", message: "Session expired." };
  }

  if (enable) {
    const result = await enableMFAApi({ userId, tenantId });
    if (!result.ok) {
      return { ok: false, code: result.error.code, message: result.error.message };
    }
    revalidatePath("/settings/account");
    return { ok: true, data: { qr_code_url: result.data.qr_code_url } };
  }

  const result = await disableMFAApi({ userId, tenantId });
  if (!result.ok) {
    return { ok: false, code: result.error.code, message: result.error.message };
  }
  revalidatePath("/settings/account");
  return { ok: true, data: {} };
}

export async function revokeSessionAction(
  sessionId: string,
): Promise<ActionResult> {
  const { userId, tenantId } = await getSession();
  if (!userId || !tenantId) return noSession();

  const result = await revokeSessionApi(sessionId, { userId, tenantId });
  if (!result.ok) {
    return { ok: false, code: result.error.code, message: result.error.message };
  }

  revalidatePath("/settings/account");
  return { ok: true };
}

export async function deleteAccountAction(
  confirmation: string,
): Promise<ActionResult> {
  const { userId, tenantId, role } = await getSession();
  if (!userId || !tenantId) return noSession();
  if (!canEditSettings(role)) return forbidden();

  if (confirmation !== "delete my store") {
    return { ok: false, code: "validation", message: "Confirmation text does not match." };
  }

  const result = await deleteAccountApi(confirmation, { userId, tenantId });
  if (!result.ok) {
    return { ok: false, code: result.error.code, message: result.error.message };
  }
  return { ok: true };
}

// ─── S2: Custom Domains ──────────────────────────────────────────────

export async function addDomainAction(
  domain: string,
  dnsMethod: "manual" | "cloudflare",
  cfApiToken?: string,
): Promise<ActionResult> {
  const { userId, tenantId, role, storeId } = await getSession();
  if (!userId || !tenantId || !storeId) return noSession();
  if (!canEditSettings(role)) return forbidden();

  if (!domain.trim()) {
    return { ok: false, code: "validation", message: "Domain is required." };
  }
  if (dnsMethod === "cloudflare" && !cfApiToken?.trim()) {
    return { ok: false, code: "validation", message: "Cloudflare API token is required." };
  }

  const result = await addDomainApi(
    storeId,
    {
      domain: domain.trim(),
      dns_method: dnsMethod,
      cf_api_token: dnsMethod === "cloudflare" ? cfApiToken?.trim() : undefined,
    },
    { userId, tenantId },
  );
  if (!result.ok) {
    return { ok: false, code: result.error.code, message: result.error.message };
  }

  revalidatePath("/settings/domains");
  return { ok: true };
}

export async function removeDomainAction(
  domainId: string,
): Promise<ActionResult> {
  const { userId, tenantId, role, storeId } = await getSession();
  if (!userId || !tenantId || !storeId) return noSession();
  if (!canEditSettings(role)) return forbidden();

  const result = await removeDomainApi(storeId, domainId, { userId, tenantId });
  if (!result.ok) {
    return { ok: false, code: result.error.code, message: result.error.message };
  }

  revalidatePath("/settings/domains");
  return { ok: true };
}

export async function refreshDomainStatusAction(
  domainId: string,
): Promise<VerifyDomainResult> {
  const { userId, tenantId, role, storeId } = await getSession();
  if (!userId || !tenantId || !storeId) {
    return { ok: false, code: "no_session", message: "Session expired. Please sign in again." };
  }
  if (!canEditSettings(role)) {
    return { ok: false, code: "forbidden", message: "You do not have permission to perform this action." };
  }
  const result = await refreshDomainStatusApi(storeId, domainId, { userId, tenantId });
  if (!result.ok) {
    return { ok: false, code: result.error.code, message: result.error.message };
  }
  revalidatePath("/settings/domains");
  return {
    ok: true,
    status: result.data.status,
    ssl_status: result.data.ssl_status,
    error: result.data.error ?? null,
  };
}

export type VerifyDomainResult =
  | { ok: true; status: string; ssl_status: string; error?: string | null }
  | { ok: false; code: string; message: string };

export async function verifyDomainAction(
  domainId: string,
): Promise<VerifyDomainResult> {
  const { userId, tenantId, role, storeId } = await getSession();
  if (!userId || !tenantId || !storeId) {
    return { ok: false, code: "no_session", message: "Session expired. Please sign in again." };
  }
  if (!canEditSettings(role)) {
    return { ok: false, code: "forbidden", message: "You do not have permission to perform this action." };
  }

  const result = await verifyDomainApi(storeId, domainId, { userId, tenantId });
  if (!result.ok) {
    return { ok: false, code: result.error.code, message: result.error.message };
  }

  revalidatePath("/settings/domains");
  return {
    ok: true,
    status: result.data.status,
    ssl_status: result.data.ssl_status,
    error: result.data.error ?? null,
  };
}

// ─── S3: Subscription ────────────────────────────────────────────────

export async function createCheckout(
  plan: string,
): Promise<ActionResultWithData<{ checkout_url: string }>> {
  const { userId, tenantId, role, storeId } = await getSession();
  if (!userId || !tenantId || !storeId) {
    return { ok: false, code: "no_session", message: "Session expired." };
  }
  if (!canEditSettings(role)) {
    return { ok: false, code: "forbidden", message: "You do not have permission." };
  }

  const result = await createCheckoutApi(storeId, plan, { userId, tenantId });
  if (!result.ok) {
    return { ok: false, code: result.error.code, message: result.error.message };
  }
  return { ok: true, data: result.data };
}

export async function createPortal(): Promise<ActionResultWithData<{ portal_url: string }>> {
  const { userId, tenantId, role, storeId } = await getSession();
  if (!userId || !tenantId || !storeId) {
    return { ok: false, code: "no_session", message: "Session expired." };
  }
  if (!canEditSettings(role)) {
    return { ok: false, code: "forbidden", message: "You do not have permission." };
  }

  const result = await createPortalApi(storeId, { userId, tenantId });
  if (!result.ok) {
    return { ok: false, code: result.error.code, message: result.error.message };
  }
  return { ok: true, data: result.data };
}

// ─── S5: Notifications ──────────────────────────────────────────────

export async function markNotificationReadAction(
  notificationId: string,
): Promise<ActionResult> {
  const { userId, tenantId, storeId } = await getSession();
  if (!userId || !tenantId || !storeId) return noSession();

  const result = await markReadApi(storeId, notificationId, { userId, tenantId });
  if (!result.ok) {
    return { ok: false, code: result.error.code, message: result.error.message };
  }

  revalidatePath("/settings/notifications");
  return { ok: true };
}

export async function markAllNotificationsReadAction(): Promise<ActionResult> {
  const { userId, tenantId, storeId } = await getSession();
  if (!userId || !tenantId || !storeId) return noSession();

  const result = await markAllReadApi(storeId, { userId, tenantId });
  if (!result.ok) {
    return { ok: false, code: result.error.code, message: result.error.message };
  }

  revalidatePath("/settings/notifications");
  return { ok: true };
}

export async function updateNotificationPreferencesAction(
  prefs: Partial<NotificationPreferences>,
): Promise<ActionResult> {
  const { userId, tenantId, role, storeId } = await getSession();
  if (!userId || !tenantId || !storeId) return noSession();
  if (!canEditSettings(role)) return forbidden();

  const result = await updatePrefsApi(storeId, prefs, { userId, tenantId });
  if (!result.ok) {
    return { ok: false, code: result.error.code, message: result.error.message };
  }

  revalidatePath("/settings/notifications");
  return { ok: true };
}

// ─── S4: Audit Logs CSV Export ──────────────────────────────────────

export async function exportAuditCSV(
  query: AuditLogsQuery,
): Promise<ActionResultWithData<string>> {
  const { userId, tenantId, storeId } = await getSession();
  if (!userId || !tenantId || !storeId) {
    return { ok: false, code: "no_session", message: "Session expired." };
  }

  try {
    const csv = await exportCSVApi(storeId, query, { userId, tenantId });
    return { ok: true, data: csv };
  } catch {
    return { ok: false, code: "export_error", message: "Failed to export audit logs." };
  }
}
