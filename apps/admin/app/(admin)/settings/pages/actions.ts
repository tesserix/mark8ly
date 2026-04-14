"use server";

// apps/admin/app/(admin)/settings/pages/actions.ts
//
// Server actions for the Pages CMS editor. Each action:
// 1. Reads session from middleware-forwarded headers.
// 2. Gates the mutation on canEditSettings(role).
// 3. Resolves the current store id via resolveStoreId().
// 4. Calls the marketplace-api client.
// 5. Returns a discriminated union so form clients can surface errors inline.

import { headers } from "next/headers";
import { revalidatePath } from "next/cache";
import { redirect } from "next/navigation";

import {
  createPage,
  updatePage,
  deletePage,
  type AdminPage,
  type CreatePageInput,
  type UpdatePageInput,
} from "@/lib/api/marketplace-api";
import { canEditSettings, resolveStoreId } from "@/lib/auth/serverSession";
import type { TenantRole } from "@/lib/api/platform-api";

// ─── Shared types ─────────────────────────────────────────────────────────

type Result<T> =
  | { ok: true; data: T }
  | { ok: false; code: string; message: string };

// ─── Session helper ────────────────────────────────────────────────────────

interface PageActionContext {
  userId: string;
  tenantId: string;
  storeId: string;
}

async function readPageContext(): Promise<Result<PageActionContext>> {
  const h = await headers();
  const userId = h.get("x-session-user-id") ?? "";
  const tenantId = h.get("x-session-tenant-id") ?? "";
  const role = (h.get("x-session-role") ?? "viewer") as TenantRole;

  if (!userId || !tenantId) {
    return {
      ok: false,
      code: "no_session",
      message: "Your session has expired. Please sign in again.",
    };
  }

  if (!canEditSettings(role)) {
    return {
      ok: false,
      code: "forbidden",
      message: "You do not have permission to edit store settings.",
    };
  }

  const storeId = await resolveStoreId();
  if (!storeId) {
    return {
      ok: false,
      code: "no_store",
      message: "We couldn't find a store for your account.",
    };
  }

  return { ok: true, data: { userId, tenantId, storeId } };
}

// ─── Actions ──────────────────────────────────────────────────────────────

export async function createPageAction(
  input: CreatePageInput,
): Promise<Result<AdminPage>> {
  const ctxResult = await readPageContext();
  if (!ctxResult.ok) return ctxResult;
  const { userId, tenantId, storeId } = ctxResult.data;

  const result = await createPage(storeId, input, { userId, tenantId });
  if (!result.ok) {
    return { ok: false, code: result.error.code, message: result.error.message };
  }

  revalidatePath("/settings/pages");
  return { ok: true, data: result.data };
}

export async function updatePageAction(
  pageId: string,
  input: UpdatePageInput,
): Promise<Result<AdminPage>> {
  const ctxResult = await readPageContext();
  if (!ctxResult.ok) return ctxResult;
  const { userId, tenantId, storeId } = ctxResult.data;

  const result = await updatePage(storeId, pageId, input, { userId, tenantId });
  if (!result.ok) {
    return { ok: false, code: result.error.code, message: result.error.message };
  }

  revalidatePath("/settings/pages");
  revalidatePath(`/settings/pages/${pageId}`);
  return { ok: true, data: result.data };
}

export async function deletePageAction(
  pageId: string,
): Promise<Result<null>> {
  const ctxResult = await readPageContext();
  if (!ctxResult.ok) return ctxResult;
  const { userId, tenantId, storeId } = ctxResult.data;

  const result = await deletePage(storeId, pageId, { userId, tenantId });
  if (!result.ok) {
    return { ok: false, code: result.error.code, message: result.error.message };
  }

  revalidatePath("/settings/pages");
  return { ok: true, data: null };
}

// createDraftPageAndRedirect creates an empty draft page then redirects to
// its editor. The redirect() call throws internally (Next.js convention), so
// the function never returns on the happy path — callers that bind this to a
// button should treat any non-error Result as unreachable.
export async function createDraftPageAndRedirect(): Promise<Result<null>> {
  const ctxResult = await readPageContext();
  if (!ctxResult.ok) return ctxResult;
  const { userId, tenantId, storeId } = ctxResult.data;

  const result = await createPage(
    storeId,
    {
      slug: `untitled-${Date.now()}`,
      title: "Untitled page",
      body: "",
      published: false,
    },
    { userId, tenantId },
  );
  if (!result.ok) {
    return { ok: false, code: result.error.code, message: result.error.message };
  }

  revalidatePath("/settings/pages");
  redirect(`/settings/pages/${result.data.id}`);
}
