"use server";

// Server action that POSTs to the same-origin email endpoint, which in
// turn forwards the request to marketplace-api. Wrapping it as a server
// action lets the docs panel call it from a client component without
// fetching cross-origin and without touching auth headers manually.

import { headers } from "next/headers";

export interface DocumentEmailResult {
  ok: boolean;
  data?: { sent: true; recipient: string };
  error?: { code: string; message: string };
}

export async function resendDocumentEmail(
  storeId: string,
  orderId: string,
  kind: "invoice" | "receipt",
  note?: string,
): Promise<DocumentEmailResult> {
  const h = await headers();
  const proto = h.get("x-forwarded-proto") ?? "https";
  const host = h.get("x-forwarded-host") ?? h.get("host") ?? "";
  if (!host) {
    return { ok: false, error: { code: "no_host", message: "Could not resolve host." } };
  }
  // Forward the BFF cookie so the same-origin route can authenticate.
  const cookie = h.get("cookie") ?? "";

  const trimmedNote = (note ?? "").trim();

  try {
    const res = await fetch(
      `${proto}://${host}/api/admin/stores/${storeId}/orders/${orderId}/${kind}/email`,
      {
        method: "POST",
        cache: "no-store",
        headers: {
          "Content-Type": "application/json",
          ...(cookie ? { cookie } : {}),
        },
        body: JSON.stringify(trimmedNote ? { note: trimmedNote } : {}),
      },
    );
    if (res.status === 401 || res.status === 403) {
      return { ok: false, error: { code: "unauthorized", message: "Session expired." } };
    }
    if (res.status === 409) {
      const body = (await res.json().catch(() => ({}))) as { message?: string };
      return {
        ok: false,
        error: {
          code: "not_ready",
          message: body.message ?? "Document not yet available for this order.",
        },
      };
    }
    if (res.status === 422) {
      // The recipient is missing on the order — actionable user error,
      // surface it verbatim so the admin sees the fix path.
      const body = (await res.json().catch(() => ({}))) as { message?: string };
      return {
        ok: false,
        error: {
          code: "no_recipient",
          message:
            body.message ??
            "This order has no customer email on file. Add one before resending.",
        },
      };
    }
    if (!res.ok) {
      return {
        ok: false,
        error: { code: "send_failed", message: `Email service returned ${res.status}.` },
      };
    }
    const body = (await res.json()) as { sent: true; recipient: string };
    return { ok: true, data: body };
  } catch (e) {
    return {
      ok: false,
      error: {
        code: "network",
        message: e instanceof Error ? e.message : "Network error sending email.",
      },
    };
  }
}
