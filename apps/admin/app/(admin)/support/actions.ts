"use server";

import { headers } from "next/headers";
import { redirect } from "next/navigation";
import { revalidatePath } from "next/cache";
import {
  createTicket,
  replyToTicket,
  updateTicketStatus,
  type TicketStatus,
} from "@/lib/api/marketplace-api";
import { resolveStoreId } from "@/lib/auth/serverSession";

// resolveStoreId falls back to the tenant's first store when the
// session cookie doesn't include a store id — without that fallback,
// admin actions fail with "No store found" for any freshly-signed-in
// user whose middleware hasn't populated x-session-store-id yet.
async function getSessionFromHeaders(): Promise<{
  userId: string;
  tenantId: string;
  storeId: string;
  email: string;
}> {
  const h = await headers();
  const [userId, tenantId, email, storeId] = [
    h.get("x-session-user-id") ?? "",
    h.get("x-session-tenant-id") ?? "",
    h.get("x-session-email") ?? "",
    await resolveStoreId(),
  ];
  return { userId, tenantId, email, storeId };
}

export async function createTicketAction(
  _prev: { error?: string } | null,
  formData: FormData,
): Promise<{ error?: string } | null> {
  const subject = (formData.get("subject") as string)?.trim() ?? "";
  const description = (formData.get("description") as string)?.trim() ?? "";
  const priority = (formData.get("priority") as string) ?? "medium";

  if (!subject) {
    return { error: "Subject is required." };
  }
  if (!description || description.length < 20) {
    return { error: "Description must be at least 20 characters." };
  }

  const { userId, tenantId, email, storeId } = await getSessionFromHeaders();
  if (!storeId) {
    return { error: "No store found. Please create a store first." };
  }

  const result = await createTicket(
    storeId,
    {
      subject,
      description,
      priority: priority as "low" | "medium" | "high",
    },
    { userId, tenantId, email },
  );

  if (!result.ok) {
    return { error: result.error.message };
  }

  redirect(`/support/tickets/${result.data.id}`);
}

// Reply and status-change actions return a discriminated result so the
// client can show a toast instead of redirecting. Both revalidate the
// ticket page so the next render pulls the fresh thread + status from
// marketplace-api.
export type TicketActionResult =
  | { ok: true }
  | { ok: false; error: string };

export async function replyToTicketAction(
  _prev: TicketActionResult | null,
  formData: FormData,
): Promise<TicketActionResult> {
  const ticketId = (formData.get("ticketId") as string) ?? "";
  const content = (formData.get("body") as string)?.trim() ?? "";

  if (!content) {
    return { ok: false, error: "Reply cannot be empty." };
  }

  const { userId, tenantId, email, storeId } = await getSessionFromHeaders();
  if (!storeId) {
    return { ok: false, error: "No store found." };
  }

  const result = await replyToTicket(
    storeId,
    ticketId,
    { content },
    { userId, tenantId, email },
  );

  if (!result.ok) {
    return { ok: false, error: result.error.message };
  }

  revalidatePath(`/support/tickets/${ticketId}`);
  revalidatePath(`/support/tickets`);
  return { ok: true };
}

export async function updateTicketStatusAction(
  _prev: TicketActionResult | null,
  formData: FormData,
): Promise<TicketActionResult> {
  const ticketId = (formData.get("ticketId") as string) ?? "";
  const status = (formData.get("status") as string) ?? "";

  const { userId, tenantId, email, storeId } = await getSessionFromHeaders();
  if (!storeId) {
    return { ok: false, error: "No store found." };
  }

  const result = await updateTicketStatus(
    storeId,
    ticketId,
    status as TicketStatus,
    { userId, tenantId, email },
  );

  if (!result.ok) {
    return { ok: false, error: result.error.message };
  }

  revalidatePath(`/support/tickets/${ticketId}`);
  revalidatePath(`/support/tickets`);
  return { ok: true };
}
