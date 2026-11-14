"use server";

import { headers } from "next/headers";
import { revalidatePath } from "next/cache";
import {
  createCategory,
  deleteCategory as deleteCategoryApi,
  listCategories,
  updateCategory,
  type MutationError,
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

// Rename a category. The slug stays as it is, so links to the category
// page keep working.
export async function renameCategory(
  storeId: string,
  categoryId: string,
  name: string,
): Promise<ToggleFeaturedResult> {
  const h = await headers();
  const userId = h.get("x-session-user-id") ?? "";
  const tenantId = h.get("x-session-tenant-id") ?? "";
  if (!userId || !tenantId || !storeId || !categoryId) {
    return { ok: false, error: "Session expired." };
  }
  const trimmed = name.trim();
  if (!trimmed) {
    return { ok: false, error: "Give the category a name." };
  }

  const result = await updateCategory(
    storeId,
    categoryId,
    { name: trimmed },
    { userId, tenantId },
  );
  if (!result.ok) {
    return {
      ok: false,
      error: result.error?.message ?? "Failed to rename category.",
    };
  }

  revalidatePath("/products/categories");
  return { ok: true };
}

// Delete a category. The API refuses while products or sub-categories
// still use it; say which, so the merchant knows what to move first.
export async function deleteCategory(
  storeId: string,
  categoryId: string,
): Promise<ToggleFeaturedResult> {
  const h = await headers();
  const userId = h.get("x-session-user-id") ?? "";
  const tenantId = h.get("x-session-tenant-id") ?? "";
  if (!userId || !tenantId || !storeId || !categoryId) {
    return { ok: false, error: "Session expired." };
  }

  const result = await deleteCategoryApi(storeId, categoryId, { userId, tenantId });
  if (!result.ok) {
    return { ok: false, error: deleteErrorMessage(result.error) };
  }

  revalidatePath("/products/categories");
  return { ok: true };
}

function deleteErrorMessage(err: MutationError): string {
  const count = (key: string) => Number(err.details?.[key] ?? 0);
  if (err.code === "category_not_empty") {
    const n = count("product_count");
    return `${n === 1 ? "1 product is" : `${n} products are`} still in this category. Move them to another category first.`;
  }
  if (err.code === "category_has_children") {
    const n = count("child_count");
    return `${n === 1 ? "1 sub-category sits" : `${n} sub-categories sit`} under this category. Move or delete them first.`;
  }
  return err.message || "Failed to delete category.";
}
