"use server";

import { revalidatePath } from "next/cache";

import { setInStoreSettings, type InStoreSettings } from "@/lib/api/instore-api";
import { getServerSessionContext } from "@/lib/auth/serverSession";

// Changes the current store's in-store settings. Owner and admin only; the
// API re-checks.
export async function setInStoreSettingAction(
  change: Partial<InStoreSettings>,
): Promise<{ ok: true; settings: InStoreSettings } | { ok: false; message: string }> {
  const { role, currentStore, userId, tenantId, email } = await getServerSessionContext();
  if (role !== "owner" && role !== "admin") {
    return { ok: false, message: "Only the owner or an admin can change this." };
  }
  if (!currentStore) return { ok: false, message: "Select a store first." };
  const res = await setInStoreSettings(currentStore.id, change, { userId, tenantId, email });
  if (!res.ok) return { ok: false, message: res.error.message };
  // The sidebar's In-store sale link follows the switch.
  revalidatePath("/", "layout");
  return { ok: true, settings: res.data };
}
