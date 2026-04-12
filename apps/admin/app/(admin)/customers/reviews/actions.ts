"use server";

import { revalidatePath } from "next/cache";
import { headers } from "next/headers";

import { resolveStoreId } from "@/lib/auth/serverSession";
import {
  approveReview,
  rejectReview,
  replyToReview,
  setReviewFeatured,
} from "@/lib/api/marketplace-api";

interface ActionResult {
  ok: boolean;
  error?: string;
}

interface ActionContext {
  userId: string;
  tenantId: string;
  storeId: string;
}

async function readContext(): Promise<ActionContext | null> {
  const h = await headers();
  const userId = h.get("x-session-user-id") ?? "";
  const tenantId = h.get("x-session-tenant-id") ?? "";
  const storeId = await resolveStoreId();
  if (!userId || !tenantId || !storeId) return null;
  return { userId, tenantId, storeId };
}

export async function approveReviewAction(
  reviewId: string,
): Promise<ActionResult> {
  const ctx = await readContext();
  if (!ctx) {
    return { ok: false, error: "Session expired. Please sign in again." };
  }

  const result = await approveReview(ctx.storeId, reviewId, {
    userId: ctx.userId,
    tenantId: ctx.tenantId,
  });
  if (!result.ok) {
    return { ok: false, error: result.error.message };
  }
  revalidatePath("/customers/reviews");
  revalidatePath(`/customers/reviews/${reviewId}`);
  return { ok: true };
}

export async function rejectReviewAction(
  reviewId: string,
): Promise<ActionResult> {
  const ctx = await readContext();
  if (!ctx) {
    return { ok: false, error: "Session expired. Please sign in again." };
  }

  const result = await rejectReview(ctx.storeId, reviewId, {
    userId: ctx.userId,
    tenantId: ctx.tenantId,
  });
  if (!result.ok) {
    return { ok: false, error: result.error.message };
  }
  revalidatePath("/customers/reviews");
  revalidatePath(`/customers/reviews/${reviewId}`);
  return { ok: true };
}

export async function toggleFeaturedAction(
  reviewId: string,
  featured: boolean,
): Promise<ActionResult> {
  const ctx = await readContext();
  if (!ctx) {
    return { ok: false, error: "Session expired. Please sign in again." };
  }

  const result = await setReviewFeatured(ctx.storeId, reviewId, featured, {
    userId: ctx.userId,
    tenantId: ctx.tenantId,
  });
  if (!result.ok) {
    return { ok: false, error: result.error.message };
  }
  revalidatePath("/customers/reviews");
  revalidatePath(`/customers/reviews/${reviewId}`);
  return { ok: true };
}

export async function replyToReviewAction(
  reviewId: string,
  content: string,
): Promise<ActionResult> {
  const ctx = await readContext();
  if (!ctx) {
    return { ok: false, error: "Session expired. Please sign in again." };
  }

  const trimmed = content.trim();
  if (!trimmed) {
    return { ok: false, error: "Reply content is required." };
  }
  if (trimmed.length > 5000) {
    return { ok: false, error: "Reply is too long (max 5000 characters)." };
  }

  const result = await replyToReview(ctx.storeId, reviewId, trimmed, {
    userId: ctx.userId,
    tenantId: ctx.tenantId,
  });
  if (!result.ok) {
    return { ok: false, error: result.error.message };
  }
  revalidatePath("/customers/reviews");
  revalidatePath(`/customers/reviews/${reviewId}`);
  return { ok: true };
}
