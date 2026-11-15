"use server";

// Server-side entry points for the sale screen. MARKETPLACE_API_URL is
// in-cluster, so the browser cannot call it; identity comes from the
// middleware-forwarded headers, never from the browser.

import { headers } from "next/headers";

import {
  type InStoreQuote,
  type InStoreQuoteBody,
  type InStoreSaleBody,
  type SaleResult,
  quoteInStoreSale,
  recordInStoreSale,
} from "@/lib/api/instore-api";
import type { MutationResult, SessionHeaders } from "@/lib/api/marketplace-api";

async function serverSession(): Promise<SessionHeaders | null> {
  const h = await headers();
  const userId = h.get("x-session-user-id") ?? "";
  const tenantId = h.get("x-session-tenant-id") ?? "";
  const email = h.get("x-session-email") ?? undefined;
  return userId && tenantId ? { userId, tenantId, email } : null;
}

export async function quoteSaleAction(storeId: string, body: InStoreQuoteBody): Promise<MutationResult<InStoreQuote>> {
  const session = await serverSession();
  if (!session) return { ok: false, error: { code: "no_session", message: "Your session has expired. Please sign in again." } };
  return quoteInStoreSale(storeId, body, session);
}

export async function recordSaleAction(storeId: string, body: InStoreSaleBody): Promise<SaleResult> {
  const session = await serverSession();
  // Nothing was sent, so nothing was recorded.
  if (!session) return { ok: false, kind: "rejected", message: "Your session has expired. Please sign in again." };
  return recordInStoreSale(storeId, body, session);
}
