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
import {
  updateBranding,
  type UpdateBrandingInput,
} from "@/lib/api/marketplace-api";

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

    // Mirror the preset's resolved colors back into the legacy branding
    // columns (color_background/text/accent/button_bg/button_text) so
    // storefront components that still read branding.color_* keep
    // rendering the merchant's chosen palette. Dark presets use their
    // background for the button too; light presets use the text color as
    // the button background (high contrast CTA). This keeps the dual-
    // stack working while the storefront migration finishes.
    const c = storefrontTheme.colors;
    await updateBranding(
      storeId,
      {
        color_background: c.background,
        color_text: c.text,
        color_accent: c.accent,
        color_button_bg: c.text,
        color_button_text: c.background,
      },
      { userId: uid, tenantId },
    );
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

  revalidatePath("/settings/themes");
  revalidatePath("/", "layout");
  return { ok: true };
}

// ─── B1: Branding (marketplace-api) ─────────────────────────────────

export type UpdateBrandingResult =
  | { ok: true }
  | { ok: false; code: string; message: string };

export async function updateBrandingAction(
  input: UpdateBrandingInput,
): Promise<UpdateBrandingResult> {
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
      message: "You do not have permission to edit branding settings.",
    };
  }

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

  const result = await updateBranding(storeId, input, {
    userId: uid,
    tenantId,
  });

  if (!result.ok) {
    return {
      ok: false,
      code: result.error.code,
      message: result.error.message,
    };
  }

  revalidatePath("/settings/themes");
  return { ok: true };
}
