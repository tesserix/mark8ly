"use server";

// Server actions for tax settings — P6.

import { headers } from "next/headers";
import { revalidatePath } from "next/cache";

import { upsertTaxJarConfig } from "@/lib/api/settings-api";
import type { TaxJarUpsertInput } from "@/lib/api/settings-api";
import { canEditSettings } from "@/lib/auth/serverSession";
import type { TenantRole } from "@/lib/api/platform-api";

export type ActionResult =
  | { ok: true }
  | { ok: false; code: string; message: string };

export async function saveTaxJarConfig(
  input: TaxJarUpsertInput,
): Promise<ActionResult> {
  const h = await headers();
  const userId = h.get("x-session-user-id") ?? "";
  const tenantId = h.get("x-session-tenant-id") ?? "";
  const role = (h.get("x-session-role") ?? "viewer") as TenantRole;
  const storeId = h.get("x-session-store-id") ?? "";

  if (!userId || !tenantId || !storeId) {
    return { ok: false, code: "no_session", message: "Session expired. Please sign in again." };
  }
  if (!canEditSettings(role)) {
    return { ok: false, code: "forbidden", message: "You do not have permission to edit settings." };
  }
  if (!input.api_key.trim()) {
    return { ok: false, code: "validation", message: "TaxJar API key is required." };
  }

  const result = await upsertTaxJarConfig(storeId, input, { userId, tenantId });
  if (!result.ok) {
    return { ok: false, code: result.error.code, message: result.error.message };
  }

  revalidatePath("/settings/tax");
  return { ok: true };
}
