"use server";

// Server actions for shipping settings — P6.

import { headers } from "next/headers";
import { revalidatePath } from "next/cache";

import {
  upsertShippingConfig,
  deleteShippingConfig,
} from "@/lib/api/settings-api";
import type { ShippingConfigUpsertInput } from "@/lib/api/settings-api";
import { canEditSettings, resolveStoreId } from "@/lib/auth/serverSession";
import type { TenantRole } from "@/lib/api/platform-api";

export type ActionResult =
  | { ok: true }
  | { ok: false; code: string; message: string };

async function getSession() {
  const h = await headers();
  const userId = h.get("x-session-user-id") ?? "";
  const tenantId = h.get("x-session-tenant-id") ?? "";
  const role = (h.get("x-session-role") ?? "viewer") as TenantRole;
  const storeId = await resolveStoreId();
  return { userId, tenantId, role, storeId };
}

export async function saveShippingConfig(
  provider: string,
  input: ShippingConfigUpsertInput,
): Promise<ActionResult> {
  const { userId, tenantId, role, storeId } = await getSession();
  if (!userId || !tenantId || !storeId) {
    return { ok: false, code: "no_session", message: "Session expired. Please sign in again." };
  }
  if (!canEditSettings(role)) {
    return { ok: false, code: "forbidden", message: "You do not have permission to edit settings." };
  }
  // Intentionally NOT requiring api_key here. A blank key on an existing
  // carrier means "keep the stored credential" so the merchant can edit
  // the warehouse address (or fees, or mode) without re-entering the key.
  // The backend distinguishes create from edit and returns a proper
  // "api_key is required" error only when creating a brand-new carrier,
  // so this check would just wrongly block every warehouse-only edit.

  const result = await upsertShippingConfig(storeId, provider, input, { userId, tenantId });
  if (!result.ok) {
    return { ok: false, code: result.error.code, message: result.error.message };
  }

  revalidatePath("/settings/shipping");
  return { ok: true };
}

export async function removeShippingConfig(
  provider: string,
): Promise<ActionResult> {
  const { userId, tenantId, role, storeId } = await getSession();
  if (!userId || !tenantId || !storeId) {
    return { ok: false, code: "no_session", message: "Session expired. Please sign in again." };
  }
  if (!canEditSettings(role)) {
    return { ok: false, code: "forbidden", message: "You do not have permission to edit settings." };
  }

  const result = await deleteShippingConfig(storeId, provider, { userId, tenantId });
  if (!result.ok) {
    return { ok: false, code: result.error.code, message: result.error.message };
  }

  revalidatePath("/settings/shipping");
  return { ok: true };
}
