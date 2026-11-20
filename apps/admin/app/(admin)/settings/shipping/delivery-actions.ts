"use server";

// Server actions for own-delivery settings.

import { headers } from "next/headers";
import { revalidatePath } from "next/cache";

import * as api from "@/lib/api/delivery-api";
import type {
  CourierInput,
  DeliveryOptionInput,
  ShippingMode,
} from "@/lib/api/delivery-api";
import { canEditSettings, resolveStoreId } from "@/lib/auth/serverSession";
import type { TenantRole } from "@/lib/api/platform-api";
import type { MutationResult } from "@/lib/api/marketplace-api";

export type ActionResult =
  | { ok: true }
  | { ok: false; code: string; message: string };

async function run(
  call: (storeId: string, session: { userId: string; tenantId: string }) => Promise<MutationResult<true>>,
): Promise<ActionResult> {
  const h = await headers();
  const userId = h.get("x-session-user-id") ?? "";
  const tenantId = h.get("x-session-tenant-id") ?? "";
  const role = (h.get("x-session-role") ?? "viewer") as TenantRole;
  const storeId = await resolveStoreId();
  if (!userId || !tenantId || !storeId) {
    return { ok: false, code: "no_session", message: "Session expired. Please sign in again." };
  }
  if (!canEditSettings(role)) {
    return { ok: false, code: "forbidden", message: "You do not have permission to edit settings." };
  }
  const result = await call(storeId, { userId, tenantId });
  if (!result.ok) {
    return { ok: false, code: result.error.code, message: result.error.message };
  }
  revalidatePath("/settings/shipping");
  return { ok: true };
}

export async function setShippingMode(mode: ShippingMode): Promise<ActionResult> {
  return run((s, session) => api.setShippingMode(s, mode, session));
}

export async function saveDeliveryOptions(
  options: DeliveryOptionInput[],
  freeShippingMin: string | null,
): Promise<ActionResult> {
  return run((s, session) => api.saveDeliveryOptions(s, options, freeShippingMin, session));
}

export async function createCourier(input: CourierInput): Promise<ActionResult> {
  return run((s, session) => api.createCourier(s, input, session));
}

export async function updateCourier(courierId: string, input: CourierInput): Promise<ActionResult> {
  return run((s, session) => api.updateCourier(s, courierId, input, session));
}

export async function deleteCourier(courierId: string): Promise<ActionResult> {
  return run((s, session) => api.deleteCourier(s, courierId, session));
}
