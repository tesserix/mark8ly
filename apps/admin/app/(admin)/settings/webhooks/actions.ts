"use server";

// Server actions for the webhooks settings page (#562 task 9).
//
// Every action starts with `guard()`, which resolves the session and
// refuses write actions from anyone who can't edit settings. Read actions
// (list deliveries) don't need write permission, only a valid session.

import { headers } from "next/headers";
import { revalidatePath } from "next/cache";

import {
  listWebhooks,
  createWebhook,
  patchWebhook,
  deleteWebhook,
  testSendWebhook,
  listDeliveries,
  replayDelivery,
  type WebhookSubscription,
  type WebhookDelivery,
  type CreateWebhookInput,
  type PatchWebhookInput,
  type CreateWebhookResult,
  type TestSendResult,
} from "@/lib/api/webhooks";
import { canEditSettings, resolveStoreId } from "@/lib/auth/serverSession";
import type { TenantRole } from "@/lib/api/platform-api";

export type ActionResult<T = undefined> =
  | { ok: true; data: T }
  | { ok: false; code: string; message: string; field?: string };

async function getSession() {
  const h = await headers();
  const userId = h.get("x-session-user-id") ?? "";
  const tenantId = h.get("x-session-tenant-id") ?? "";
  const role = (h.get("x-session-role") ?? "viewer") as TenantRole;
  const storeId = await resolveStoreId();
  return { userId, tenantId, role, storeId };
}

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
        message: "You do not have permission to manage webhooks.",
      },
    };
  }
  return { session };
}

function refresh() {
  revalidatePath("/settings/webhooks");
}

export async function listWebhooksAction(): Promise<ActionResult<WebhookSubscription[]>> {
  const session = await getSession();
  if (!session.userId || !session.tenantId || !session.storeId) {
    return { ok: false, code: "no_session", message: "Session expired. Please sign in again." };
  }
  const data = await listWebhooks(session.storeId, {
    userId: session.userId,
    tenantId: session.tenantId,
  });
  return { ok: true, data };
}

export async function createWebhookAction(
  input: CreateWebhookInput,
): Promise<ActionResult<CreateWebhookResult>> {
  const { session, refusal } = await guard();
  if (refusal) return refusal;

  const result = await createWebhook(session.storeId, input, {
    userId: session.userId,
    tenantId: session.tenantId,
  });
  if (!result.ok) {
    return {
      ok: false,
      code: result.error.code,
      message: result.error.message,
      field: result.error.field,
    };
  }
  refresh();
  return { ok: true, data: result.data };
}

export async function patchWebhookAction(
  webhookId: string,
  input: PatchWebhookInput,
): Promise<ActionResult<WebhookSubscription>> {
  const { session, refusal } = await guard();
  if (refusal) return refusal;

  const result = await patchWebhook(session.storeId, webhookId, input, {
    userId: session.userId,
    tenantId: session.tenantId,
  });
  if (!result.ok) {
    return {
      ok: false,
      code: result.error.code,
      message: result.error.message,
      field: result.error.field,
    };
  }
  refresh();
  return { ok: true, data: result.data };
}

export async function deleteWebhookAction(webhookId: string): Promise<ActionResult> {
  const { session, refusal } = await guard();
  if (refusal) return refusal;

  const result = await deleteWebhook(session.storeId, webhookId, {
    userId: session.userId,
    tenantId: session.tenantId,
  });
  if (!result.ok) {
    return { ok: false, code: result.error.code, message: result.error.message };
  }
  refresh();
  return { ok: true, data: undefined };
}

// Test-send needs only a valid session, not write permission — it never
// mutates the subscription and staff/viewer debugging their own endpoint
// shouldn't need an admin to run it for them. It's still scoped to the
// caller's store by ownedSubscription() server-side.
export async function testSendWebhookAction(
  webhookId: string,
): Promise<ActionResult<TestSendResult>> {
  const session = await getSession();
  if (!session.userId || !session.tenantId || !session.storeId) {
    return { ok: false, code: "no_session", message: "Session expired. Please sign in again." };
  }
  const result = await testSendWebhook(session.storeId, webhookId, {
    userId: session.userId,
    tenantId: session.tenantId,
  });
  if (!result.ok) {
    return { ok: false, code: result.error.code, message: result.error.message };
  }
  return { ok: true, data: result.data };
}

export async function listDeliveriesAction(
  webhookId: string,
): Promise<ActionResult<WebhookDelivery[]>> {
  const session = await getSession();
  if (!session.userId || !session.tenantId || !session.storeId) {
    return { ok: false, code: "no_session", message: "Session expired. Please sign in again." };
  }
  const data = await listDeliveries(session.storeId, webhookId, {
    userId: session.userId,
    tenantId: session.tenantId,
  });
  return { ok: true, data };
}

export async function replayDeliveryAction(
  webhookId: string,
  deliveryId: string,
): Promise<ActionResult> {
  const { session, refusal } = await guard();
  if (refusal) return refusal;

  const result = await replayDelivery(session.storeId, webhookId, deliveryId, {
    userId: session.userId,
    tenantId: session.tenantId,
  });
  if (!result.ok) {
    return { ok: false, code: result.error.code, message: result.error.message };
  }
  refresh();
  return { ok: true, data: undefined };
}
