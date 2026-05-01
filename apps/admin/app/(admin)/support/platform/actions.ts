"use server";

import { revalidatePath } from "next/cache";
import { redirect } from "next/navigation";
import { getServerSessionContext } from "@/lib/auth/serverSession";
import { filePlatformTicket } from "@/lib/api/tesserix";

export type PlatformTicketActionState =
  | null
  | { error: string }
  | { success: string };

export async function filePlatformTicketAction(
  _prev: PlatformTicketActionState,
  formData: FormData,
): Promise<PlatformTicketActionState> {
  const subject = (formData.get("subject") as string)?.trim() ?? "";
  const description = (formData.get("description") as string)?.trim() ?? "";
  const priority = (formData.get("priority") as string) ?? "medium";

  if (!subject) {
    return { error: "Subject is required." };
  }
  if (!description || description.length < 20) {
    return { error: "Description must be at least 20 characters." };
  }
  if (!isPriority(priority)) {
    return { error: "Invalid priority." };
  }

  const session = await getServerSessionContext();
  if (!session.tenantId || !session.userId) {
    return { error: "You must be signed in to a company to file a ticket." };
  }

  // Prefer the company name (tenantName) — super-admins triaging the
  // ticket want to see "Acme Co" rather than an email local-part. The
  // helper falls back to "your store" when the tenant fetch failed; we
  // strip that sentinel so it never leaks into the ticket record.
  const tenantNameClean =
    session.tenantName && session.tenantName !== "your store"
      ? session.tenantName
      : "";
  const submittedByName =
    tenantNameClean || session.email.split("@")[0] || "Merchant";

  const result = await filePlatformTicket({
    tenantId: session.tenantId,
    subject,
    description,
    priority,
    submittedByName,
    submittedByEmail: session.email,
    submittedByUserId: session.userId,
  });

  if (!result.ok) {
    return { error: result.error.message };
  }

  revalidatePath("/support/platform");
  redirect("/support/platform?filed=1");
}

function isPriority(
  value: string,
): value is "low" | "medium" | "high" | "urgent" {
  return (
    value === "low" || value === "medium" || value === "high" || value === "urgent"
  );
}
