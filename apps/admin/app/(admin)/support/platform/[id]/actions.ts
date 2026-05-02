"use server";

import { revalidatePath } from "next/cache";
import { getServerSessionContext } from "@/lib/auth/serverSession";
import { replyToPlatformTicket } from "@/lib/api/tesserix";

export type PlatformReplyActionState =
  | null
  | { error: string }
  | { success: true };

export async function replyToPlatformTicketAction(
  ticketId: string,
  _prev: PlatformReplyActionState,
  formData: FormData,
): Promise<PlatformReplyActionState> {
  const content = (formData.get("content") as string)?.trim() ?? "";
  if (!content) {
    return { error: "Reply cannot be empty." };
  }
  if (content.length > 10_000) {
    return { error: "Reply is too long." };
  }

  const session = await getServerSessionContext();
  if (!session.tenantId || !session.userId) {
    return { error: "You must be signed in to reply." };
  }

  const tenantNameClean =
    session.tenantName && session.tenantName !== "your store"
      ? session.tenantName
      : "";
  const authorName =
    tenantNameClean || session.email.split("@")[0] || "Merchant";

  const result = await replyToPlatformTicket({
    tenantId: session.tenantId,
    ticketId,
    content,
    authorName,
    authorEmail: session.email,
    authorUserId: session.userId,
  });

  if (!result.ok) {
    return { error: result.error.message };
  }

  revalidatePath(`/support/platform/${ticketId}`);
  revalidatePath("/support/platform");
  return { success: true };
}
