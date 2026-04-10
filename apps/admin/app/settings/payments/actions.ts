"use server";

// Server actions for payment settings — P6.
//
// Each action reads session headers from middleware, validates inputs,
// and delegates to the settings-api client. Returns a discriminated
// union so the client form can pattern-match on .ok without try/catch.

import { headers } from "next/headers";
import { revalidatePath } from "next/cache";

import {
  upsertPaymentConfig,
  deletePaymentConfig,
  testPaymentConnection,
} from "@/lib/api/settings-api";
import type { PaymentConfigUpsertInput } from "@/lib/api/settings-api";
import { canEditSettings } from "@/lib/auth/serverSession";
import type { TenantRole } from "@/lib/api/platform-api";

export type ActionResult =
  | { ok: true }
  | { ok: false; code: string; message: string };

export type TestResult =
  | { ok: true; success: boolean; error?: string }
  | { ok: false; code: string; message: string };

async function getSession() {
  const h = await headers();
  const userId = h.get("x-session-user-id") ?? "";
  const tenantId = h.get("x-session-tenant-id") ?? "";
  const role = (h.get("x-session-role") ?? "viewer") as TenantRole;
  const storeId = h.get("x-session-store-id") ?? "";
  return { userId, tenantId, role, storeId };
}

export async function savePaymentConfig(
  provider: string,
  input: PaymentConfigUpsertInput,
): Promise<ActionResult> {
  const { userId, tenantId, role, storeId } = await getSession();
  if (!userId || !tenantId || !storeId) {
    return { ok: false, code: "no_session", message: "Session expired. Please sign in again." };
  }
  if (!canEditSettings(role)) {
    return { ok: false, code: "forbidden", message: "You do not have permission to edit settings." };
  }
  if (!input.api_key.trim()) {
    return { ok: false, code: "validation", message: "API key is required." };
  }

  const result = await upsertPaymentConfig(storeId, provider, input, { userId, tenantId });
  if (!result.ok) {
    return { ok: false, code: result.error.code, message: result.error.message };
  }

  revalidatePath("/settings/payments");
  return { ok: true };
}

export async function removePaymentConfig(
  provider: string,
): Promise<ActionResult> {
  const { userId, tenantId, role, storeId } = await getSession();
  if (!userId || !tenantId || !storeId) {
    return { ok: false, code: "no_session", message: "Session expired. Please sign in again." };
  }
  if (!canEditSettings(role)) {
    return { ok: false, code: "forbidden", message: "You do not have permission to edit settings." };
  }

  const result = await deletePaymentConfig(storeId, provider, { userId, tenantId });
  if (!result.ok) {
    return { ok: false, code: result.error.code, message: result.error.message };
  }

  revalidatePath("/settings/payments");
  return { ok: true };
}

export async function testPayment(
  provider: string,
): Promise<TestResult> {
  const { userId, tenantId, storeId } = await getSession();
  if (!userId || !tenantId || !storeId) {
    return { ok: false, code: "no_session", message: "Session expired. Please sign in again." };
  }

  const result = await testPaymentConnection(storeId, provider, { userId, tenantId });
  return { ok: true, success: result.success, error: result.error };
}
