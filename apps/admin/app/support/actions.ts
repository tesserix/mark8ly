"use server";

import { headers } from "next/headers";
import { redirect } from "next/navigation";
import {
  createTicket,
  replyToTicket,
  updateTicketStatus,
  type TicketStatus,
} from "@/lib/api/marketplace-api";

async function getSessionFromHeaders(): Promise<{
  userId: string;
  tenantId: string;
  storeId: string;
}> {
  const h = await headers();
  return {
    userId: h.get("x-session-user-id") ?? "",
    tenantId: h.get("x-session-tenant-id") ?? "",
    storeId: h.get("x-session-store-id") ?? "",
  };
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

  const { userId, tenantId, storeId } = await getSessionFromHeaders();
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
    { userId, tenantId },
  );

  if (!result.ok) {
    return { error: result.error.message };
  }

  redirect(`/support/tickets/${result.data.id}`);
}

export async function replyToTicketAction(
  _prev: { error?: string } | null,
  formData: FormData,
): Promise<{ error?: string } | null> {
  const ticketId = (formData.get("ticketId") as string) ?? "";
  const body = (formData.get("body") as string)?.trim() ?? "";

  if (!body) {
    return { error: "Reply cannot be empty." };
  }

  const { userId, tenantId, storeId } = await getSessionFromHeaders();
  if (!storeId) {
    return { error: "No store found." };
  }

  const result = await replyToTicket(
    storeId,
    ticketId,
    { body },
    { userId, tenantId },
  );

  if (!result.ok) {
    return { error: result.error.message };
  }

  redirect(`/support/tickets/${ticketId}`);
}

export async function updateTicketStatusAction(
  _prev: { error?: string } | null,
  formData: FormData,
): Promise<{ error?: string } | null> {
  const ticketId = (formData.get("ticketId") as string) ?? "";
  const status = (formData.get("status") as string) ?? "";

  const { userId, tenantId, storeId } = await getSessionFromHeaders();
  if (!storeId) {
    return { error: "No store found." };
  }

  const result = await updateTicketStatus(
    storeId,
    ticketId,
    status as TicketStatus,
    { userId, tenantId },
  );

  if (!result.ok) {
    return { error: result.error.message };
  }

  redirect(`/support/tickets/${ticketId}`);
}
