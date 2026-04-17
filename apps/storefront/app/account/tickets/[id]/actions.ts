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
