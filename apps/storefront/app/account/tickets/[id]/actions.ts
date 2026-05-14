"use server";

// Server action for customer ticket replies. Proxies to
// POST /storefront/stores/:storeSlug/account/tickets/:id/reply on the
// marketplace-api, which enforces ownership by submitter email + blocks
// replies on closed tickets.

import { cookies, headers } from "next/headers";

import { resolveStoreSlug } from "@/lib/slug";

const MARKETPLACE_API_URL =
  process.env.MARKETPLACE_API_URL ?? "http://localhost:8088";
const STOREFRONT_KEY = process.env.MARKETPLACE_STOREFRONT_KEY ?? "";

export type ReplyResult =
  | { ok: true }
  | { ok: false; code: string; message: string };

export async function replyToTicket(
  ticketId: string,
  content: string,
): Promise<ReplyResult> {
  const trimmed = content.trim();
  if (!trimmed) {
    return {
      ok: false,
      code: "validation_error",
      message: "Message cannot be empty.",
    };
  }

  const c = await cookies();
  const sessionCookie = c.get("mp_customer_session")?.value ?? "";
  if (!sessionCookie) {
    return {
      ok: false,
      code: "unauthorized",
      message: "Please sign in to reply.",
    };
  }

  const h = await headers();
  const storeSlug = await resolveStoreSlug(h.get("host"));

  const apiHeaders: Record<string, string> = {
    Accept: "application/json",
    "Content-Type": "application/json",
    Cookie: `mp_customer_session=${sessionCookie}`,
  };
  if (STOREFRONT_KEY) apiHeaders["X-Storefront-Key"] = STOREFRONT_KEY;

  try {
    const res = await fetch(
      `${MARKETPLACE_API_URL}/api/v1/storefront/stores/${encodeURIComponent(
        storeSlug,
      )}/account/tickets/${encodeURIComponent(ticketId)}/reply`,
      {
        method: "POST",
        headers: apiHeaders,
        cache: "no-store",
        body: JSON.stringify({ content: trimmed }),
      },
    );

    if (res.status === 429) {
      return {
        ok: false,
        code: "rate_limited",
        message: "Too many replies. Please wait a moment and try again.",
      };
    }
    if (res.status === 401) {
      return {
        ok: false,
        code: "unauthorized",
        message: "Your session expired. Please sign in again.",
      };
    }
    if (res.status === 404) {
      return {
        ok: false,
        code: "not_found",
        message: "This ticket is no longer available.",
      };
    }
    if (!res.ok) {
      const body = (await res.json().catch(() => null)) as {
        error?: string;
        message?: string;
      } | null;
      return {
        ok: false,
        code: body?.error ?? "reply_failed",
        message:
          body?.message ??
          "We couldn't post your reply. Please try again shortly.",
      };
    }
    return { ok: true };
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

// CustomerTicketStatus mirrors the four states the marketplace-api
// state machine recognises. The customer can drive the ticket toward
// resolved / closed / open (reopen) — the in_progress state is set by
// merchant pickup and is not part of the customer-allowed transition
// set (the backend rejects with 409 if attempted).
export type CustomerTicketStatus =
  | "open"
  | "in_progress"
  | "resolved"
  | "closed";

export type CustomerStatusTarget = "resolved" | "closed" | "open";

export type StatusUpdateResult =
  | { ok: true }
  | { ok: false; code: string; message: string };

// updateTicketStatus proxies to the marketplace-api endpoint that lets
// the signed-in shopper resolve, close, or reopen their own ticket.
// The backend enforces ownership (via mp_customer_session) and rejects
// transitions that don't belong to the customer-allowed set.
export async function updateTicketStatus(
  ticketId: string,
  target: CustomerStatusTarget,
): Promise<StatusUpdateResult> {
  const c = await cookies();
  const sessionCookie = c.get("mp_customer_session")?.value ?? "";
  if (!sessionCookie) {
    return {
      ok: false,
      code: "unauthorized",
      message: "Please sign in to update this ticket.",
    };
  }

  const h = await headers();
  const storeSlug = await resolveStoreSlug(h.get("host"));

  const apiHeaders: Record<string, string> = {
    Accept: "application/json",
    "Content-Type": "application/json",
    Cookie: `mp_customer_session=${sessionCookie}`,
  };
  if (STOREFRONT_KEY) apiHeaders["X-Storefront-Key"] = STOREFRONT_KEY;

  try {
    const res = await fetch(
      `${MARKETPLACE_API_URL}/api/v1/storefront/stores/${encodeURIComponent(
        storeSlug,
      )}/account/tickets/${encodeURIComponent(ticketId)}/status`,
      {
        method: "POST",
        headers: apiHeaders,
        cache: "no-store",
        body: JSON.stringify({ status: target }),
      },
    );

    if (res.status === 429) {
      return {
        ok: false,
        code: "rate_limited",
        message: "Too many updates in a row. Please wait a moment.",
      };
    }
    if (res.status === 401) {
      return {
        ok: false,
        code: "unauthorized",
        message: "Your session expired. Please sign in again.",
      };
    }
    if (res.status === 404) {
      return {
        ok: false,
        code: "not_found",
        message: "This ticket is no longer available.",
      };
    }
    if (res.status === 409) {
      return {
        ok: false,
        code: "invalid_transition",
        message: "This change isn't allowed for the ticket's current state.",
      };
    }
    if (!res.ok) {
      const body = (await res.json().catch(() => null)) as {
        error?: string;
        message?: string;
      } | null;
      return {
        ok: false,
        code: body?.error ?? "update_failed",
        message:
          body?.message ??
          "We couldn't update the ticket. Please try again shortly.",
      };
    }
    return { ok: true };
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

// TranscriptMessage matches the marketplace-api projection — the otto
// sender_type enum is collapsed to "customer" | "support" | "system" so
// the UI doesn't have to know about staff/agent/ai role nuance.
export interface TranscriptMessage {
  id: string;
  author_type: "customer" | "support" | "system";
  author_name: string;
  content: string;
  created_at: string;
}

export interface TranscriptData {
  case_id: string;
  status: string;
  created_at: string;
  closed_at?: string;
  messages: TranscriptMessage[];
}

export type TranscriptResult =
  | { ok: true; transcript: TranscriptData }
  | { ok: false; code: string; message: string };

// fetchTicketTranscript pulls the chat that spawned this ticket — only
// returns OK when the ticket has a conversation_id AND otto is wired.
// The UI consumes this lazily (on expand) so the page paints fast even
// if otto is briefly slow.
export async function fetchTicketTranscript(
  ticketId: string,
): Promise<TranscriptResult> {
  const c = await cookies();
  const sessionCookie = c.get("mp_customer_session")?.value ?? "";
  if (!sessionCookie) {
    return {
      ok: false,
      code: "unauthorized",
      message: "Please sign in to view this transcript.",
    };
  }

  const h = await headers();
  const storeSlug = await resolveStoreSlug(h.get("host"));

  const apiHeaders: Record<string, string> = {
    Accept: "application/json",
    Cookie: `mp_customer_session=${sessionCookie}`,
  };
  if (STOREFRONT_KEY) apiHeaders["X-Storefront-Key"] = STOREFRONT_KEY;

  try {
    const res = await fetch(
      `${MARKETPLACE_API_URL}/api/v1/storefront/stores/${encodeURIComponent(
        storeSlug,
      )}/account/tickets/${encodeURIComponent(ticketId)}/transcript`,
      { method: "GET", headers: apiHeaders, cache: "no-store" },
    );

    if (res.status === 404) {
      return {
        ok: false,
        code: "not_found",
        message: "No chat transcript is available for this ticket.",
      };
    }
    if (res.status === 401) {
      return {
        ok: false,
        code: "unauthorized",
        message: "Your session expired. Please sign in again.",
      };
    }
    if (!res.ok) {
      return {
        ok: false,
        code: "transcript_failed",
        message: "We couldn't load the transcript. Please try again shortly.",
      };
    }

    const body = (await res.json()) as { data: TranscriptData };
    return { ok: true, transcript: body.data };
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
