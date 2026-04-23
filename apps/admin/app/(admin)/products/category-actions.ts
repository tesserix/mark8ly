"use server";

import { headers } from "next/headers";
import { revalidatePath } from "next/cache";
import {
  createCategory,
  listCategories,
  updateCategory,
} from "@/lib/api/marketplace-api";

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

  const trimmed = name.trim();
  const slug = trimmed
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/^-+|-+$/g, "")
    .slice(0, 60);

  // Idempotency: if a category with this slug or name already exists in
  // the store, reuse it instead of attempting to create. This lets the
  // product-creation flow continue without forcing the user to pick the
  // existing category manually.
  const existing = await listCategories(storeId, { userId, tenantId });
  const lowerName = trimmed.toLowerCase();
  const dup = existing.find(
    (c) => c.slug === slug || c.name.toLowerCase() === lowerName,
  );
  if (dup) {
    return {
      ok: true,
      category: { id: dup.id, name: dup.name, slug: dup.slug },
    };
  }

  const result = await createCategory(
    storeId,
    { name: trimmed, slug },
    { userId, tenantId },
  );

  if (!result.ok) {
    // Race: another request created the same slug between our list and
    // create. Re-fetch and return the winner instead of surfacing 409.
    if (result.error?.code === "slug_taken") {
      const after = await listCategories(storeId, { userId, tenantId });
      const winner = after.find(
        (c) => c.slug === slug || c.name.toLowerCase() === lowerName,
      );
      if (winner) {
        return {
          ok: true,
          category: {
            id: winner.id,
            name: winner.name,
            slug: winner.slug,
          },
        };
      }
    }
    return {
      ok: false,
      error: result.error?.message ?? "Failed to create category.",
    };
  }
  if (!result.data || !result.data.id) {
    return {
      ok: false,
      error: "Category created but response was malformed.",
    };
  }

  // Intentionally NOT calling revalidatePath here: the ProductCategoriesPicker
  // merges the new category into client state on success. Revalidating the
  // /products path while the user is mid-edit on /products/new forces a
  // server re-render that races with the open form and surfaces an
  // "Unexpected error" boundary to the user.
  return {
    ok: true,
    category: {
      id: result.data.id,
      name: result.data.name,
      slug: result.data.slug,
    },
  };
}

export interface ToggleFeaturedResult {
  ok: boolean;
  error?: string;
}

// Toggle a category's `featured` flag. Surfaces the flag to the
// storefront's /products filter grid. All other category fields are
// untouched.
export async function setCategoryFeatured(
  storeId: string,
  categoryId: string,
  featured: boolean,
): Promise<ToggleFeaturedResult> {
  const h = await headers();
  const userId = h.get("x-session-user-id") ?? "";
  const tenantId = h.get("x-session-tenant-id") ?? "";
  if (!userId || !tenantId || !storeId || !categoryId) {
    return { ok: false, error: "Session expired." };
  }

  const result = await updateCategory(
    storeId,
    categoryId,
    { featured },
    { userId, tenantId },
  );
  if (!result.ok) {
    return {
      ok: false,
      error: result.error?.message ?? "Failed to update category.",
    };
  }

  revalidatePath("/products/categories");
  return { ok: true };
}
