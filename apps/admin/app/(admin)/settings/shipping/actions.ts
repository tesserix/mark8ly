"use server";

// Server actions for shipping settings — P6.

import { headers } from "next/headers";
import { revalidatePath } from "next/cache";

import {
  upsertShippingConfig,
  deleteShippingConfig,
} from "@/lib/api/settings-api";
import type { ShippingConfigUpsertInput } from "@/lib/api/settings-api";
import { canEditSettings } from "@/lib/auth/serverSession";
import type { TenantRole } from "@/lib/api/platform-api";

export type ActionResult =
  | { ok: true }
  | { ok: false; code: string; message: string };

async function getSession() {
  const h = await headers();
  const userId = h.get("x-session-user-id") ?? "";
  const tenantId = h.get("x-session-tenant-id") ?? "";
  const role = (h.get("x-session-role") ?? "viewer") as TenantRole;
  const storeId = h.get("x-session-store-id") ?? "";
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
  if (!input.api_key.trim()) {
    return { ok: false, code: "validation", message: "API key is required." };
  }

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
