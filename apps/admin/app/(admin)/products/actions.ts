"use server";

// apps/admin/app/products/actions.ts
//
// Server actions for the product detail form. Each action:
// 1. Reads session from middleware-forwarded headers
// 2. Runs the Zod schema (defense-in-depth; client also validates)
// 3. Builds the marketplace-api request body, synthesizing a single
//    simple-product variant from the form's price/stock/sku fields
// 4. Calls the appropriate marketplace-api client function
// 5. Returns a typed result the form can surface inline, OR redirects
//    on success

import { headers } from "next/headers";
import { redirect } from "next/navigation";
import { revalidatePath } from "next/cache";

import {
  createProduct,
  updateProduct,
  deleteProduct,
  copyProduct,
  bulkProductAction,
  getProduct,
  type CreateProductRequest,
  type UpdateProductRequest,
} from "@/lib/api/marketplace-api";
import { productFormSchema, type ProductFormValues } from "@/lib/validation/product-form";
import { bulkActionSchema } from "@/lib/validation/bulk-action";

// listStoresByTenant is from platform-api — we need the store id + currency
// from the session, which is already resolved in the page caller. Instead of
// re-fetching here, every action accepts the storeId + currencyCode from the
// caller (passed as a bound form field). The middleware-forwarded tenant id
// is still the tenant scope — platform-api owns the tenant row, marketplace-
// api trusts the x-tenant-id header.

export interface ActionResult {
  ok: boolean;
  error?: {
    code: string;
    message: string;
    field?: string;
  };
}

interface ActionContext {
  userId: string;
  tenantId: string;
  email: string;
  storeId: string;
  currencyCode: string;
}

async function readContext(storeId: string, currencyCode: string): Promise<ActionContext | null> {
  const h = await headers();
  const userId = h.get("x-session-user-id") ?? "";
  const tenantId = h.get("x-session-tenant-id") ?? "";
  const email = h.get("x-session-email") ?? "";
  if (!userId || !tenantId || !storeId) return null;
  return { userId, tenantId, email, storeId, currencyCode };
}

function slugFromTitle(title: string): string {
  return title
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/^-+|-+$/g, "")
    .slice(0, 60) || "product";
}

function buildVariants(
  values: ProductFormValues,
  currencyCode: string,
): CreateProductRequest["variants"] {
  const qty = Number.parseInt(values.inventoryQuantity, 10);
  return [
    {
      sku: (values.sku && values.sku.length > 0 ? values.sku : `${slugFromTitle(values.title)}-default`),
      price: values.price,
      currency_code: currencyCode,
      inventory_quantity: Number.isFinite(qty) ? qty : 0,
      inventory_policy: "deny",
      position: 0,
    },
  ];
}

export async function createProductAction(
  storeId: string,
  currencyCode: string,
  input: ProductFormValues,
): Promise<ActionResult> {
  const ctx = await readContext(storeId, currencyCode);
  if (!ctx) return { ok: false, error: { code: "no_session", message: "Your session has expired. Please sign in again." } };

  const parsed = productFormSchema.safeParse(input);
  if (!parsed.success) {
    const first = parsed.error.issues[0];
    return { ok: false, error: { code: "validation_failed", message: first?.message ?? "Invalid input", field: String(first?.path?.[0] ?? "") } };
  }

  const body: CreateProductRequest = {
    handle: parsed.data.handle && parsed.data.handle.length > 0 ? parsed.data.handle : undefined,
    title: parsed.data.title,
    description: parsed.data.description && parsed.data.description.length > 0 ? parsed.data.description : undefined,
    status: parsed.data.status,
    options: [],
    variants: buildVariants(parsed.data, ctx.currencyCode),
    category_ids: parsed.data.categoryIds.length > 0 ? parsed.data.categoryIds : undefined,
  };

  const result = await createProduct(ctx.storeId, body, { userId: ctx.userId, tenantId: ctx.tenantId, email: ctx.email });
  if (!result.ok) {
    return { ok: false, error: result.error };
  }

  revalidatePath("/products");
  redirect(`/products/${result.data.id}`);
}

export async function updateProductAction(
  storeId: string,
  productId: string,
  currencyCode: string,
  input: ProductFormValues,
): Promise<ActionResult> {
  const ctx = await readContext(storeId, currencyCode);
  if (!ctx) return { ok: false, error: { code: "no_session", message: "Your session has expired. Please sign in again." } };

  const parsed = productFormSchema.safeParse(input);
  if (!parsed.success) {
    const first = parsed.error.issues[0];
    return { ok: false, error: { code: "validation_failed", message: first?.message ?? "Invalid input", field: String(first?.path?.[0] ?? "") } };
  }

  // M7c: full aggregate PATCH. Options, variants, media, and
  // removed_variant_ids are forwarded as-is when the caller supplies
  // them. M7b callers that don't populate the new arrays still work
  // because Zod `.default([])` fills them in.
  //
  // Simple-product quirk: the RHF form only tracks variants when the
  // user opens the Variants tab, so for title/price/status-only edits on
  // a simple product, `parsed.data.variants` is empty. The Go aggregate
  // service requires exactly one variant when there are no options, so
  // an empty array is rejected with VariantMatrixMismatch. Synthesize a
  // single variant from the scalar price/sku/stock fields, preserving
  // the existing variant's id so the server replaces in-place rather
  // than creating a duplicate.
  let variants = (parsed.data.variants ?? []).map((v) => ({
    id: v.id,
    sku: v.sku,
    price: v.price,
    inventory_quantity: v.stock,
    weight_grams: Math.round(v.weight * 1000),
    currency_code: ctx.currencyCode,
    option_values: v.optionValues.map((ov) => ({
      option_name: ov.optionName,
      value: ov.value,
    })),
  }));

  const hasOptions = (parsed.data.options ?? []).length > 0;
  if (!hasOptions && variants.length === 0) {
    const existing = await getProduct(ctx.storeId, productId, {
      userId: ctx.userId,
      tenantId: ctx.tenantId,
      email: ctx.email,
    });
    const firstVariant = existing?.variants[0];
    const qty = Number.parseInt(parsed.data.inventoryQuantity, 10);
    variants = [
      {
        id: firstVariant?.id,
        sku: parsed.data.sku && parsed.data.sku.length > 0
          ? parsed.data.sku
          : firstVariant?.sku ?? `${slugFromTitle(parsed.data.title)}-default`,
        price: parsed.data.price,
        inventory_quantity: Number.isFinite(qty) ? qty : 0,
        weight_grams: firstVariant?.weight_grams ?? 0,
        currency_code: ctx.currencyCode,
        option_values: [],
      },
    ];
  }

  const body: UpdateProductRequest = {
    handle: parsed.data.handle && parsed.data.handle.length > 0 ? parsed.data.handle : undefined,
    title: parsed.data.title,
    description: parsed.data.description && parsed.data.description.length > 0 ? parsed.data.description : undefined,
    status: parsed.data.status,
    category_ids: parsed.data.categoryIds,
    options: (parsed.data.options ?? []).map((o) => ({
      id: o.id,
      name: o.name,
      values: o.values.map((v) => ({ id: v.id, value: v.value })),
    })),
    variants,
    media: (parsed.data.media ?? []).map((m) => ({
      id: m.id,
      storage_key: m.storage_key,
      url: m.url,
      alt: m.alt,
      position: m.position,
      variant_id: m.variant_id,
    })),
    removed_variant_ids: parsed.data.removed_variant_ids ?? [],
  };

  const result = await updateProduct(ctx.storeId, productId, body, { userId: ctx.userId, tenantId: ctx.tenantId, email: ctx.email });
  if (!result.ok) {
    return { ok: false, error: result.error };
  }

  revalidatePath("/products");
  revalidatePath(`/products/${productId}`);
  return { ok: true };
}

export async function deleteProductAction(
  storeId: string,
  productId: string,
): Promise<ActionResult> {
  const ctx = await readContext(storeId, "USD");
  if (!ctx) return { ok: false, error: { code: "no_session", message: "Your session has expired. Please sign in again." } };

  const result = await deleteProduct(ctx.storeId, productId, { userId: ctx.userId, tenantId: ctx.tenantId, email: ctx.email });
  if (!result.ok) {
    return { ok: false, error: result.error };
  }

  revalidatePath("/products");
  redirect("/products");
}

// ─────────────────────────────────────────────────────────────────────────
// M7d: copy-to-store + bulk actions
// ─────────────────────────────────────────────────────────────────────────

export interface CopyActionInput {
  targetStoreId: string;
  copyMedia: boolean;
}

export async function copyProductAction(
  storeId: string,
  productId: string,
  input: CopyActionInput,
): Promise<ActionResult> {
  const ctx = await readContext(storeId, "USD");
  if (!ctx) return { ok: false, error: { code: "no_session", message: "Your session has expired. Please sign in again." } };

  const result = await copyProduct(ctx.storeId, productId, {
    target_store_id: input.targetStoreId,
    copy_media: input.copyMedia,
  }, { userId: ctx.userId, tenantId: ctx.tenantId, email: ctx.email });

  if (!result.ok) {
    return { ok: false, error: result.error };
  }

  revalidatePath("/products");
  return { ok: true };
}

export interface BulkActionResult {
  ok: boolean;
  results?: Array<{ id: string; status: "ok" | "error"; error?: string }>;
  error?: { code: string; message: string };
}

async function executeBulkAction(
  storeId: string,
  action: string,
  productIds: string[],
  params?: Record<string, unknown>,
): Promise<BulkActionResult> {
  const ctx = await readContext(storeId, "USD");
  if (!ctx) return { ok: false, error: { code: "no_session", message: "Your session has expired. Please sign in again." } };

  const parsed = bulkActionSchema.safeParse({ product_ids: productIds });
  if (!parsed.success) {
    const first = parsed.error.issues[0];
    return { ok: false, error: { code: "validation_failed", message: first?.message ?? "Invalid input" } };
  }

  const result = await bulkProductAction(ctx.storeId, {
    action,
    product_ids: productIds,
    params,
  }, { userId: ctx.userId, tenantId: ctx.tenantId, email: ctx.email });

  if (!result.ok) {
    return { ok: false, error: result.error };
  }

  revalidatePath("/products");
  return { ok: true, results: result.data.results };
}

export async function bulkArchiveAction(
  storeId: string,
  productIds: string[],
): Promise<BulkActionResult> {
  return executeBulkAction(storeId, "archive", productIds);
}

export async function bulkUnarchiveAction(
  storeId: string,
  productIds: string[],
): Promise<BulkActionResult> {
  return executeBulkAction(storeId, "unarchive", productIds);
}

export async function bulkPublishAction(
  storeId: string,
  productIds: string[],
): Promise<BulkActionResult> {
  return executeBulkAction(storeId, "publish", productIds);
}

export async function bulkUnpublishAction(
  storeId: string,
  productIds: string[],
): Promise<BulkActionResult> {
  return executeBulkAction(storeId, "unpublish", productIds);
}

export async function bulkDeleteAction(
  storeId: string,
  productIds: string[],
): Promise<BulkActionResult> {
  return executeBulkAction(storeId, "delete", productIds);
}

export async function bulkAssignCategoryAction(
  storeId: string,
  productIds: string[],
  categoryIds: string[],
): Promise<BulkActionResult> {
  return executeBulkAction(storeId, "assign_category", productIds, { category_ids: categoryIds });
}
