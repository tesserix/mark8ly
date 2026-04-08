"use server";

import { headers } from "next/headers";
import { revalidatePath } from "next/cache";

import {
  listStoresByTenant,
  PlatformApiError,
  updateStore,
  type TenantRole,
} from "@/lib/api/platform-api";
import {
  canEditSettings,
} from "@/lib/auth/serverSession";
import type { StorefrontTheme } from "@repo/ui/storefront-theme";

export type UpdateStorefrontThemeResult =
  | { ok: true }
  | { ok: false; code: string; message: string };

export async function updateStorefrontTheme(
  storefrontTheme: StorefrontTheme,
): Promise<UpdateStorefrontThemeResult> {
  const h = await headers();
  const tenantId = h.get("x-session-tenant-id") ?? "";
  const uid = h.get("x-session-user-id") ?? "";
  const role = (h.get("x-session-role") ?? "viewer") as TenantRole;
  const currentStoreId = h.get("x-session-store-id") ?? "";

  if (!tenantId || !uid) {
    return {
      ok: false,
      code: "no_session",
      message: "Your session has expired. Please sign in again.",
    };
  }

  if (!canEditSettings(role)) {
    return {
      ok: false,
      code: "forbidden",
      message: "You do not have permission to edit storefront settings.",
    };
  }

  try {
    let storeId = currentStoreId;
    if (!storeId) {
      const stores = await listStoresByTenant(tenantId);
      const current = stores[0];
      if (!current) {
        return {
          ok: false,
          code: "no_store",
          message: "We couldn't find a store for your account.",
        };
      }
      storeId = current.id;
    }

    await updateStore(storeId, {
      uid,
      storefront_theme: storefrontTheme,
    });
  } catch (err) {
    if (err instanceof PlatformApiError) {
      return { ok: false, code: err.code, message: err.message };
    }
    return {
      ok: false,
      code: "unknown_error",
      message: "Something went wrong. Please try again.",
    };
  }

  revalidatePath("/settings/storefront");
  revalidatePath("/", "layout");
  return { ok: true };
}
