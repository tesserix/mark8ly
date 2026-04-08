"use server";

// Server action for Phase N — general settings form.
//
// Reads the session tenant id from the cookie (via middleware-forwarded
// headers) and PATCHes the tenant row through the platform-api client.
// The path param MUST come from the session, never from the browser,
// so a malicious client can't target another tenant's id.

import { headers } from "next/headers";
import { revalidatePath } from "next/cache";

import { updateTenant, PlatformApiError } from "@/lib/api/platform-api";

export interface UpdateGeneralSettingsInput {
  name: string;
}

export type UpdateGeneralSettingsResult =
  | { ok: true }
  | { ok: false; code: string; message: string };

export async function updateGeneralSettings(
  input: UpdateGeneralSettingsInput,
): Promise<UpdateGeneralSettingsResult> {
  const h = await headers();
  const tenantId = h.get("x-session-tenant-id") ?? "";
  if (!tenantId) {
    return {
      ok: false,
      code: "no_session",
      message: "Your session has expired. Please sign in again.",
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

  try {
    await updateTenant(tenantId, { name });
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

  // Server components on /settings/general and anywhere else that
  // renders tenantName (AdminShell top-bar) need to pick up the new
  // value on next navigation.
  revalidatePath("/settings/general");
  revalidatePath("/", "layout");

  return { ok: true };
}
