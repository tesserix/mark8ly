"use server";

// app/orders/[id]/shipping-actions.ts
//
// Server actions for the shipping label panel. Follows the same pattern
// as actions.ts: readContext, typed result, revalidatePath on success.

import { revalidatePath } from "next/cache";
import { headers } from "next/headers";

import {
  createShipment,
  updateShipmentStatus,
  getOrderShipment,
  emailShipmentLabel,
  refreshShipmentTracking,
  deleteShipment,
  type CreateShipmentInput,
  type ShipmentResponse,
} from "@/lib/api/shipping-api";

export interface ShippingActionResult {
  ok: boolean;
  data?: ShipmentResponse;
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

function noSession(): ShippingActionResult {
  return {
    ok: false,
    error: {
      code: "no_session",
      message: "Your session has expired. Please sign in again.",
    },
  };
}

// ─────────────────────────────────────────────────────────────────────────
// Get shipment
// ─────────────────────────────────────────────────────────────────────────

export async function getShipmentAction(
  storeId: string,
  orderId: string,
): Promise<ShipmentResponse | null> {
  const ctx = await readContext(storeId);
  if (!ctx) return null;

  return getOrderShipment(ctx.storeId, orderId, {
    userId: ctx.userId,
    tenantId: ctx.tenantId,
  });
}

// ─────────────────────────────────────────────────────────────────────────
// Create shipment
// ─────────────────────────────────────────────────────────────────────────

export async function createShipmentAction(
  storeId: string,
  orderId: string,
  input: CreateShipmentInput,
): Promise<ShippingActionResult> {
  const ctx = await readContext(storeId);
  if (!ctx) return noSession();

  if (!input.provider || input.provider.trim() === "") {
    return {
      ok: false,
      error: {
        code: "validation_failed",
        message: "Carrier is required.",
        field: "provider",
      },
    };
  }
  if (!input.service || input.service.trim() === "") {
    return {
      ok: false,
      error: {
        code: "validation_failed",
        message: "Service level is required.",
        field: "service",
      },
    };
  }

  const result = await createShipment(
    ctx.storeId,
    orderId,
    { provider: input.provider.trim(), service: input.service.trim() },
    { userId: ctx.userId, tenantId: ctx.tenantId },
  );
  if (!result.ok) {
    return {
      ok: false,
      error: { code: result.error.code, message: result.error.message },
    };
  }
  revalidatePath(`/orders/${orderId}`);
  return { ok: true, data: result.data };
}

// ─────────────────────────────────────────────────────────────────────────
// Advance shipment status — drives the customer-facing delivery timeline.
// ─────────────────────────────────────────────────────────────────────────

export async function updateShipmentStatusAction(
  storeId: string,
  orderId: string,
  shipmentId: string,
  input: { status: string; description?: string },
): Promise<ShippingActionResult> {
  const ctx = await readContext(storeId);
  if (!ctx) return noSession();

  const result = await updateShipmentStatus(
    ctx.storeId,
    orderId,
    shipmentId,
    input,
    { userId: ctx.userId, tenantId: ctx.tenantId },
  );
  if (!result.ok) {
    return {
      ok: false,
      error: { code: result.error.code, message: result.error.message },
    };
  }
  revalidatePath(`/orders/${orderId}`);
  return { ok: true, data: result.data };
}

// ─────────────────────────────────────────────────────────────────────────
// Email the label PDF to a nominated recipient.
// ─────────────────────────────────────────────────────────────────────────

export async function emailShipmentLabelAction(
  storeId: string,
  orderId: string,
  shipmentId: string,
  recipient: string,
): Promise<{ ok: true } | { ok: false; error: { code: string; message: string } }> {
  const ctx = await readContext(storeId);
  if (!ctx) {
    return {
      ok: false,
      error: { code: "no_session", message: "Your session has expired." },
    };
  }
  const trimmed = recipient.trim();
  if (!trimmed) {
    return {
      ok: false,
      error: { code: "validation_failed", message: "Recipient email is required." },
    };
  }
  const result = await emailShipmentLabel(
    ctx.storeId,
    orderId,
    shipmentId,
    { recipient: trimmed },
    { userId: ctx.userId, tenantId: ctx.tenantId },
  );
  if (!result.ok) {
    return {
      ok: false,
      error: { code: result.error.code, message: result.error.message },
    };
  }
  return { ok: true };
}

// ─────────────────────────────────────────────────────────────────────────
// Refresh the carrier's tracking state and sync it back to the shipment.
// ─────────────────────────────────────────────────────────────────────────

export async function refreshShipmentTrackingAction(
  storeId: string,
  orderId: string,
  shipmentId: string,
): Promise<ShippingActionResult> {
  const ctx = await readContext(storeId);
  if (!ctx) return noSession();

  const result = await refreshShipmentTracking(ctx.storeId, orderId, shipmentId, {
    userId: ctx.userId,
    tenantId: ctx.tenantId,
  });
  if (!result.ok) {
    return {
      ok: false,
      error: { code: result.error.code, message: result.error.message },
    };
  }
  revalidatePath(`/orders/${orderId}`);
  return { ok: true, data: result.data };
}

// ─────────────────────────────────────────────────────────────────────────
// Delete a shipment record (used to clear pre-fix TEST-DLV-* stubs so
// Create shipping label can be retried against real Delhivery).
// ─────────────────────────────────────────────────────────────────────────

export async function deleteShipmentAction(
  storeId: string,
  orderId: string,
  shipmentId: string,
): Promise<{ ok: true } | { ok: false; error: { code: string; message: string } }> {
  const ctx = await readContext(storeId);
  if (!ctx) {
    return {
      ok: false,
      error: { code: "no_session", message: "Your session has expired." },
    };
  }
  const result = await deleteShipment(ctx.storeId, orderId, shipmentId, {
    userId: ctx.userId,
    tenantId: ctx.tenantId,
  });
  if (!result.ok) {
    return {
      ok: false,
      error: { code: result.error.code, message: result.error.message },
    };
  }
  revalidatePath(`/orders/${orderId}`);
  return { ok: true };
}
