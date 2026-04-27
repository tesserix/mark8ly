// marketplace-api client for order-document email dispatch.
//
// One small client wrapping POST /api/v1/admin/stores/:storeId/orders/
// :id/{invoice,receipt}/email which renders the email server-side and
// hands it to SendGrid.

import type { SessionHeaders } from "./marketplace-api";

const MARKETPLACE_API_URL =
  process.env.MARKETPLACE_API_URL ?? "http://localhost:8088";

function readHeaders(s: SessionHeaders): Record<string, string> {
  return {
    "X-User-Id": s.userId,
    "X-Tenant-Id": s.tenantId,
  };
}

export interface SendDocEmailOk {
  ok: true;
  data: { sent: true; recipient: string };
}

export interface SendDocEmailErr {
  ok: false;
  status?: number;
  error: { code: string; message: string };
}

export type SendDocEmailResult = SendDocEmailOk | SendDocEmailErr;

export interface SendDocEmailOptions {
  /**
   * Optional personal note from the merchant — rendered as a "Note from
   * {store}" block in the email body. Empty string suppresses the block,
   * matching the canonical template. Whitespace is trimmed server-side
   * and the value is capped at 800 chars before render.
   */
  note?: string;
}

export async function sendOrderDocumentEmail(
  storeId: string,
  orderId: string,
  kind: "invoice" | "receipt",
  session: SessionHeaders,
  options: SendDocEmailOptions = {},
): Promise<SendDocEmailResult> {
  const url = `${MARKETPLACE_API_URL}/api/v1/admin/stores/${storeId}/orders/${orderId}/${kind}/email`;
  // The marketplace-api endpoint accepts an optional JSON body with a
  // `note` field. Send `{}` when no note is set — Gin's ShouldBindJSON
  // tolerates missing fields, so the empty body keeps prior behaviour.
  const body = options.note ? JSON.stringify({ note: options.note }) : "{}";
  let res: Response;
  try {
    res = await fetch(url, {
      method: "POST",
      cache: "no-store",
      headers: { ...readHeaders(session), "Content-Type": "application/json" },
      body,
    });
  } catch (e) {
    return {
      ok: false,
      error: {
        code: "network",
        message: e instanceof Error ? e.message : "marketplace-api unreachable",
      },
    };
  }

  if (res.ok) {
    const data = (await res.json()) as { sent: true; recipient: string };
    return { ok: true, data };
  }

  const errBody = (await res
    .json()
    .catch(() => null)) as { error?: string; message?: string } | null;
  return {
    ok: false,
    status: res.status,
    error: {
      code: errBody?.error ?? "send_failed",
      message: errBody?.message ?? `marketplace-api returned ${res.status}`,
    },
  };
}
