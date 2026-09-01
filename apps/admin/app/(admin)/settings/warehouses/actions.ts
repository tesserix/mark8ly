"use server";

// Server actions for the warehouses settings page (#177 PR 5c).

import { headers } from "next/headers";
import { revalidatePath } from "next/cache";

import {
  createWarehouse,
  updateWarehouse,
  removeWarehouse,
  setDefaultWarehouse,
  reorderWarehouses,
} from "@/lib/api/warehouses-api";
import type {
  WarehouseWriteInput,
  WarehouseRemoval,
} from "@/lib/api/warehouses-api";
import { canEditSettings, resolveStoreId } from "@/lib/auth/serverSession";
import type { TenantRole } from "@/lib/api/platform-api";

export type ActionResult<T = undefined> =
  | { ok: true; data?: T }
  | { ok: false; code: string; message: string };

async function getSession() {
  const h = await headers();
  const userId = h.get("x-session-user-id") ?? "";
  const tenantId = h.get("x-session-tenant-id") ?? "";
  const role = (h.get("x-session-role") ?? "viewer") as TenantRole;
  const storeId = await resolveStoreId();
  return { userId, tenantId, role, storeId };
}

// guard returns the session when the caller may write, or the refusal to
// hand straight back. Every action starts with it — a missing check on one
// action is how a whole surface ends up half-protected.
async function guard() {
  const session = await getSession();
  if (!session.userId || !session.tenantId || !session.storeId) {
    return {
      refusal: {
        ok: false as const,
        code: "no_session",
        message: "Session expired. Please sign in again.",
      },
    };
  }
  if (!canEditSettings(session.role)) {
    return {
      refusal: {
        ok: false as const,
        code: "forbidden",
        message: "You do not have permission to edit warehouses.",
      },
    };
  }
  return { session };
}

function refresh() {
  revalidatePath("/settings/warehouses");
  // The shipping page reads warehouses too — its readiness banner is
  // computed from them, so a stale cache there would keep telling the
  // merchant a carrier cannot quote after they just fixed the address.
  revalidatePath("/settings/shipping");
}

export async function saveWarehouse(
  id: string | null,
  input: WarehouseWriteInput,
): Promise<ActionResult> {
  const { session, refusal } = await guard();
  if (refusal) return refusal;

  const creds = { userId: session.userId, tenantId: session.tenantId };
  const result = id
    ? await updateWarehouse(session.storeId, id, input, creds)
    : await createWarehouse(session.storeId, input, creds);

  if (!result.ok) {
    return { ok: false, code: result.error.code, message: result.error.message };
  }
  refresh();
  return { ok: true };
}

/**
 * Remove is ONE verb by design. The API decides between deleting and
 * archiving and reports which it did, so the merchant is told what
 * happened instead of being asked to choose between two words for the
 * same intent.
 */
export async function deleteWarehouse(
  id: string,
): Promise<ActionResult<WarehouseRemoval>> {
  const { session, refusal } = await guard();
  if (refusal) return refusal;

  const result = await removeWarehouse(session.storeId, id, {
    userId: session.userId,
    tenantId: session.tenantId,
  });
  if (!result.ok) {
    return { ok: false, code: result.error.code, message: result.error.message };
  }
  refresh();
  return { ok: true, data: result.data };
}

export async function makeDefaultWarehouse(id: string): Promise<ActionResult> {
  const { session, refusal } = await guard();
  if (refusal) return refusal;

  const result = await setDefaultWarehouse(session.storeId, id, {
    userId: session.userId,
    tenantId: session.tenantId,
  });
  if (!result.ok) {
    return { ok: false, code: result.error.code, message: result.error.message };
  }
  refresh();
  return { ok: true };
}

/** order must be the complete live set — see reorderWarehouses. */
export async function reorderWarehouseList(
  order: string[],
): Promise<ActionResult> {
  const { session, refusal } = await guard();
  if (refusal) return refusal;

  const result = await reorderWarehouses(session.storeId, order, {
    userId: session.userId,
    tenantId: session.tenantId,
  });
  if (!result.ok) {
    return { ok: false, code: result.error.code, message: result.error.message };
  }
  refresh();
  return { ok: true };
}
