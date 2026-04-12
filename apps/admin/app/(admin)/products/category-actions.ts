"use server";

import { headers } from "next/headers";
import { revalidatePath } from "next/cache";
import { createCategory } from "@/lib/api/marketplace-api";

export interface CreateCategoryResult {
  ok: boolean;
  category?: { id: string; name: string; slug: string };
  error?: string;
}

export async function createCategoryInline(
  storeId: string,
  name: string,
): Promise<CreateCategoryResult> {
  const h = await headers();
  const userId = h.get("x-session-user-id") ?? "";
  const tenantId = h.get("x-session-tenant-id") ?? "";
  if (!userId || !tenantId || !storeId) {
    return { ok: false, error: "Session expired." };
  }

  const slug = name
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/^-+|-+$/g, "")
    .slice(0, 60);

  const result = await createCategory(
    storeId,
    { name: name.trim(), slug },
    { userId, tenantId },
  );

  if (!result.ok) {
    return {
      ok: false,
      error: result.error?.message ?? "Failed to create category.",
    };
  }

  revalidatePath("/products");
  return {
    ok: true,
    category: {
      id: result.data.id,
      name: result.data.name,
      slug: result.data.slug,
    },
  };
}
