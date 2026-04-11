"use server";

import { revalidatePath } from "next/cache";
import { headers } from "next/headers";

import {
  blockCustomer,
  unblockCustomer,
  updateCustomerNotes,
  updateCustomerTags,
} from "@/lib/api/marketplace-api";

interface ActionResult {
  ok: boolean;
  error?: string;
}

interface ActionContext {
  userId: string;
  tenantId: string;
  storeId: string;
}

async function readContext(): Promise<ActionContext | null> {
  const h = await headers();
  const userId = h.get("x-session-user-id") ?? "";
  const tenantId = h.get("x-session-tenant-id") ?? "";
  const storeId = h.get("x-session-store-id") ?? "";
  if (!userId || !tenantId || !storeId) return null;
  return { userId, tenantId, storeId };
}

export async function updateTagsAction(
  customerId: string,
  tags: string[],
): Promise<ActionResult> {
  const ctx = await readContext();
  if (!ctx) return { ok: false, error: "Session expired. Please sign in again." };

  const result = await updateCustomerTags(
    ctx.storeId,
    customerId,
    tags,
    { userId: ctx.userId, tenantId: ctx.tenantId },
  );
  if (!result.ok) {
    return { ok: false, error: result.error.message };
  }
  revalidatePath(`/customers/${customerId}`);
  return { ok: true };
}

export async function updateNotesAction(
  customerId: string,
  notes: string,
): Promise<ActionResult> {
  const ctx = await readContext();
  if (!ctx) return { ok: false, error: "Session expired. Please sign in again." };

  const result = await updateCustomerNotes(
    ctx.storeId,
    customerId,
    notes,
    { userId: ctx.userId, tenantId: ctx.tenantId },
  );
  if (!result.ok) {
    return { ok: false, error: result.error.message };
  }
  revalidatePath(`/customers/${customerId}`);
  return { ok: true };
}

export async function blockCustomerAction(
  customerId: string,
  reason: string,
): Promise<ActionResult> {
  const ctx = await readContext();
  if (!ctx) return { ok: false, error: "Session expired. Please sign in again." };

  if (!reason.trim()) {
    return { ok: false, error: "Reason is required." };
  }

  const result = await blockCustomer(
    ctx.storeId,
    customerId,
    reason.trim(),
    { userId: ctx.userId, tenantId: ctx.tenantId },
  );
  if (!result.ok) {
    return { ok: false, error: result.error.message };
  }
  revalidatePath(`/customers/${customerId}`);
  revalidatePath("/customers");
  return { ok: true };
}

export async function unblockCustomerAction(
  customerId: string,
): Promise<ActionResult> {
  const ctx = await readContext();
  if (!ctx) return { ok: false, error: "Session expired. Please sign in again." };

  const result = await unblockCustomer(
    ctx.storeId,
    customerId,
    { userId: ctx.userId, tenantId: ctx.tenantId },
  );
  if (!result.ok) {
    return { ok: false, error: result.error.message };
  }
  revalidatePath(`/customers/${customerId}`);
  revalidatePath("/customers");
  return { ok: true };
}
