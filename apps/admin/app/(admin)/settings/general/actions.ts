"use server";

// Server action for Phase N — general settings form.
//
// Reads the session tenant id from the cookie (via middleware-forwarded
// headers) and PATCHes the tenant row through the platform-api client.
// The path param MUST come from the session, never from the browser,
// so a malicious client can't target another tenant's id.

import { headers } from "next/headers";
import { revalidatePath } from "next/cache";

import {
  listStoresByTenant,
  updateStore,
  PlatformApiError,
} from "@/lib/api/platform-api";
import { canEditSettings } from "@/lib/auth/serverSession";
import type { TenantRole } from "@/lib/api/platform-api";

export interface UpdateGeneralSettingsInput {
  name: string;
  timezone?: string;
}

export type UpdateGeneralSettingsResult =
  | { ok: true }
  | { ok: false; code: string; message: string };

export async function updateGeneralSettings(
  input: UpdateGeneralSettingsInput,
): Promise<UpdateGeneralSettingsResult> {
  const h = await headers();
  const tenantId = h.get("x-session-tenant-id") ?? "";
  const uid = h.get("x-session-user-id") ?? "";
  const role = (h.get("x-session-role") ?? "viewer") as TenantRole;
  if (!tenantId || !uid) {
    return {
      ok: false,
      code: "no_session",
      message: "Your session has expired. Please sign in again.",
    };
  }

  // Phase O: client-side gate to fail fast before we touch the
  // network. The real enforcement is platform-api's FGA Check on
  // PATCH /internal/tenants/:id; this is just UX so a "staff" user
  // gets a clean 403 instead of a generic network error.
  if (!canEditSettings(role)) {
    return {
      ok: false,
      code: "forbidden",
      message: "You do not have permission to edit store settings.",
    };
  }

  const name = input.name.trim();
  if (!name) {
    return {
      ok: false,
      code: "invalid_name",
      message: "Store name cannot be empty.",
    };
  }
  if (name.length > 200) {
    return {
      ok: false,
      code: "invalid_name",
      message: "Store name must be 200 characters or fewer.",
    };
  }

  const timezone = input.timezone?.trim();
  if (input.timezone !== undefined && !timezone) {
    return {
      ok: false,
      code: "invalid_timezone",
      message: "Timezone cannot be empty.",
    };
  }

  // Phase Q / Q.2: general settings edit the CURRENT STORE. The
  // current store id comes from the session cookie
  // (x-session-store-id header, populated by middleware from
  // auth-bff after a switch-store call). If the header is blank
  // we fall back to the first store under the tenant — the
  // initial post-signin state before any switch.
  const currentStoreId = h.get("x-session-store-id") ?? "";
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
    await updateStore(storeId, { name, timezone, uid });
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
  void uid;

  // Server components on /settings/stores (the merged general + stores
  // page) and anywhere else that renders tenantName (AdminShell top-bar)
  // need to pick up the new value on next navigation.
  revalidatePath("/settings/stores");
  revalidatePath("/", "layout");

  return { ok: true };
}
