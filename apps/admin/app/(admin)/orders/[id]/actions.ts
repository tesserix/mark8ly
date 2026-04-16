"use server";

// app/orders/[id]/actions.ts
//
// Server actions for the order detail page. Each action:
// 1. Reads the session from middleware-forwarded headers
// 2. Validates the form input minimally (the API enforces the real rules)
// 3. Calls the marketplace-api client function
// 4. Revalidates the order path on success
// 5. Returns a typed result for the client form to render inline

import { revalidatePath } from "next/cache";
import { headers } from "next/headers";

import {
  cancelOrder,
  confirmOrder,
  fulfillOrder,
  refundOrder,
  type PaymentStatus,
} from "@/lib/api/marketplace-api";

export interface OrderActionResult {
  ok: boolean;
  error?: {
    code: string;
    message: string;
    field?: string;
  };
}

interface ActionContext {
  userId: string;
  tenantId: string;
  storeId: string;
}

async function readContext(storeId: string): Promise<ActionContext | null> {
  const h = await headers();
  const userId = h.get("x-session-user-id") ?? "";
  const tenantId = h.get("x-session-tenant-id") ?? "";
  if (!userId || !tenantId || !storeId) return null;
  return { userId, tenantId, storeId };
}

function noSession(): OrderActionResult {
  return {
    ok: false,
    error: {
      code: "no_session",
      message: "Your session has expired. Please sign in again.",
    },
  };
}

// ─────────────────────────────────────────────────────────────────────────
// Confirm
// ─────────────────────────────────────────────────────────────────────────

export interface ConfirmOrderActionInput {
  paymentStatus?: PaymentStatus;
  reason?: string;
}

export async function confirmOrderAction(
  storeId: string,
  orderId: string,
  input: ConfirmOrderActionInput,
): Promise<OrderActionResult> {
  const ctx = await readContext(storeId);
  if (!ctx) return noSession();

  const result = await confirmOrder(
    ctx.storeId,
    orderId,
    {
      payment_status: input.paymentStatus,
      reason: input.reason ?? "",
    },
    { userId: ctx.userId, tenantId: ctx.tenantId },
  );
  if (!result.ok) {
    return { ok: false, error: { code: result.error.code, message: result.error.message } };
  }
  revalidatePath(`/orders/${orderId}`);
  revalidatePath(`/orders`);
  return { ok: true };
}

// ─────────────────────────────────────────────────────────────────────────
// Fulfill
// ─────────────────────────────────────────────────────────────────────────

export async function fulfillOrderAction(
  storeId: string,
  orderId: string,
): Promise<OrderActionResult> {
  const ctx = await readContext(storeId);
  if (!ctx) return noSession();

  const result = await fulfillOrder(ctx.storeId, orderId, {
    userId: ctx.userId,
    tenantId: ctx.tenantId,
  });
  if (!result.ok) {
    return { ok: false, error: { code: result.error.code, message: result.error.message } };
  }
  revalidatePath(`/orders/${orderId}`);
  revalidatePath(`/orders`);
  return { ok: true };
}

// ─────────────────────────────────────────────────────────────────────────
// Cancel
// ─────────────────────────────────────────────────────────────────────────

export interface CancelOrderActionInput {
  reason: string;
}

export async function cancelOrderAction(
  storeId: string,
  orderId: string,
  input: CancelOrderActionInput,
): Promise<OrderActionResult> {
  const ctx = await readContext(storeId);
  if (!ctx) return noSession();

  if (!input.reason || input.reason.trim() === "") {
    return {
      ok: false,
      error: {
        code: "validation_failed",
        message: "Reason is required.",
        field: "reason",
      },
    };
  }

  const result = await cancelOrder(
    ctx.storeId,
    orderId,
    { reason: input.reason.trim() },
    { userId: ctx.userId, tenantId: ctx.tenantId },
  );
  console.log(
    `[cancel-debug-server] orderId=${orderId} storeId=${ctx.storeId} userId=${ctx.userId} tenantId=${ctx.tenantId} ok=${result.ok}` +
    (!result.ok
      ? ` err.code=${result.error.code} err.msg=${JSON.stringify(result.error.message)} err.details=${JSON.stringify(result.error.details ?? null)}`
      : ""),
  );
  if (!result.ok) {
    return { ok: false, error: { code: result.error.code, message: result.error.message } };
  }
  revalidatePath(`/orders/${orderId}`);
  revalidatePath(`/orders`);
  return { ok: true };
}

// ─────────────────────────────────────────────────────────────────────────
// Refund
// ─────────────────────────────────────────────────────────────────────────

export interface RefundOrderActionInput {
  amount: string;
  paymentStatus: PaymentStatus;
  reason?: string;
}

export async function refundOrderAction(
  storeId: string,
  orderId: string,
  input: RefundOrderActionInput,
): Promise<OrderActionResult> {
  const ctx = await readContext(storeId);
  if (!ctx) return noSession();

  // Defense-in-depth: the API enforces the real rule (positive, ≤ grand_total).
  const amount = Number.parseFloat(input.amount);
  if (!Number.isFinite(amount) || amount <= 0) {
    return {
      ok: false,
      error: {
        code: "validation_failed",
        message: "Amount must be a positive number.",
        field: "amount",
      },
    };
  }
  if (
    input.paymentStatus !== "partially_refunded" &&
    input.paymentStatus !== "refunded"
  ) {
    return {
      ok: false,
      error: {
        code: "validation_failed",
        message: "Payment status target must be partially_refunded or refunded.",
        field: "payment_status",
      },
    };
  }

  const result = await refundOrder(
    ctx.storeId,
    orderId,
    {
      amount: input.amount,
      payment_status: input.paymentStatus,
      reason: input.reason ?? "",
    },
    { userId: ctx.userId, tenantId: ctx.tenantId },
  );
  if (!result.ok) {
    return { ok: false, error: { code: result.error.code, message: result.error.message } };
  }
  revalidatePath(`/orders/${orderId}`);
  revalidatePath(`/orders`);
  return { ok: true };
}
