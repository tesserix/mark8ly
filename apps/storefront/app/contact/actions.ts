"use server";

// Server action for the storefront contact form. Proxies to the
// marketplace-api POST /storefront/stores/:storeSlug/support/tickets
// endpoint — the marketplace-api enforces rate limiting and writes the
// ticket to the same tickets table the admin dashboard reads.

const MARKETPLACE_API_URL =
  process.env.MARKETPLACE_API_URL ?? "http://localhost:8088";
const STOREFRONT_KEY = process.env.MARKETPLACE_STOREFRONT_KEY ?? "";

export type SubmitTicketResult =
  | { ok: true; ticket_number: string }
  | { ok: false; code: string; message: string };

export interface SubmitTicketInput {
  storeSlug: string;
  name: string;
  email: string;
  subject: string;
  description: string;
  priority?: "low" | "medium" | "high";
}

export async function submitSupportTicket(
  input: SubmitTicketInput,
): Promise<SubmitTicketResult> {
  const name = input.name.trim();
  const email = input.email.trim().toLowerCase();
  const subject = input.subject.trim();
  const description = input.description.trim();

  if (!name || !email || !subject || !description) {
    return {
      ok: false,
      code: "validation_error",
      message: "Please fill in every field.",
    };
  }

  const headers: Record<string, string> = {
    Accept: "application/json",
    "Content-Type": "application/json",
  };
  if (STOREFRONT_KEY) headers["X-Storefront-Key"] = STOREFRONT_KEY;

  try {
    const res = await fetch(
      `${MARKETPLACE_API_URL}/api/v1/storefront/stores/${encodeURIComponent(
        input.storeSlug,
      )}/support/tickets`,
      {
        method: "POST",
        headers,
        cache: "no-store",
        body: JSON.stringify({
          name,
          email,
          subject,
          description,
          priority: input.priority ?? "medium",
        }),
      },
    );

    if (res.status === 429) {
      return {
        ok: false,
        code: "rate_limited",
        message: "Too many requests. Please wait a moment and try again.",
      };
    }

    if (!res.ok) {
      const body = (await res.json().catch(() => null)) as {
        error?: string;
        message?: string;
      } | null;
      return {
        ok: false,
        code: body?.error ?? "submit_failed",
        message:
          body?.message ??
          "We couldn't submit your message. Please try again shortly.",
      };
    }

    const body = (await res.json()) as { ticket_number: string };
    return { ok: true, ticket_number: body.ticket_number };
  } catch (err: unknown) {
    return {
      ok: false,
      code: "network_error",
      message:
        err instanceof Error
          ? err.message
          : "We couldn't reach our servers. Please try again.",
    };
  }
}
