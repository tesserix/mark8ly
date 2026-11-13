"use server";

// Server-side entry points for MediaTab's delete, reorder and crop calls.
//
// MediaTab is a client component. It imported these functions straight from
// lib/api/marketplace-api, which fetches MARKETPLACE_API_URL — an in-cluster
// address the browser cannot reach. The fetch failed in the browser, so a
// deleted image vanished from the screen and came back on reload, and crop
// never started. Running them as server actions puts the fetch on the
// server, where the address resolves.
//
// The session argument the client passes is ignored: identity comes from the
// middleware-forwarded headers, never from the browser.

import { headers } from "next/headers";

import {
  deleteMedia,
  recropMedia,
  updateMedia,
  type MutationResult,
  type RecropMediaInput,
  type RecropMediaResult,
  type SessionHeaders,
  type UpdateMediaInput,
} from "@/lib/api/marketplace-api";

async function serverSession(): Promise<SessionHeaders | null> {
  const h = await headers();
  const userId = h.get("x-session-user-id") ?? "";
  const tenantId = h.get("x-session-tenant-id") ?? "";
  const email = h.get("x-session-email") ?? undefined;
  return userId && tenantId ? { userId, tenantId, email } : null;
}

const expired = {
  ok: false as const,
  error: { code: "no_session", message: "Your session has expired. Please sign in again." },
};

export async function deleteMediaAction(
  storeId: string,
  productId: string,
  mediaId: string,
  _session?: SessionHeaders,
): Promise<MutationResult<true>> {
  const session = await serverSession();
  if (!session) return expired;
  return deleteMedia(storeId, productId, mediaId, session);
}

export async function updateMediaAction(
  storeId: string,
  productId: string,
  mediaId: string,
  body: UpdateMediaInput,
  _session?: SessionHeaders,
): Promise<MutationResult<true>> {
  const session = await serverSession();
  if (!session) return expired;
  return updateMedia(storeId, productId, mediaId, body, session);
}

export async function recropMediaAction(
  storeId: string,
  productId: string,
  mediaId: string,
  body: RecropMediaInput,
  _session?: SessionHeaders,
): Promise<MutationResult<RecropMediaResult>> {
  const session = await serverSession();
  if (!session) return expired;
  return recropMedia(storeId, productId, mediaId, body, session);
}
