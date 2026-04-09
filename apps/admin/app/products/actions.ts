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
  type CreateProductRequest,
  type UpdateProductRequest,
} from "@/lib/api/marketplace-api";
import { productFormSchema, type ProductFormValues } from "@/lib/validation/product-form";

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
  storeId: string;
  currencyCode: string;
}

async function readContext(storeId: string, currencyCode: string): Promise<ActionContext | null> {
  const h = await headers();
  const userId = h.get("x-session-user-id") ?? "";
  const tenantId = h.get("x-session-tenant-id") ?? "";
  if (!userId || !tenantId || !storeId) return null;
  return { userId, tenantId, storeId, currencyCode };
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

  const result = await createProduct(ctx.storeId, body, { userId: ctx.userId, tenantId: ctx.tenantId });
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

  // M7b only updates basics. The simple-product price/stock update path
  // goes through the variant quick-PATCH endpoint, NOT through the product
  // PATCH. For M7b, we only PATCH the basics (title, handle, description,
  // status, category_ids); price/stock updates on the simple product form
  // are deferred to a follow-up that wires the variant quick-PATCH action.
  const body: UpdateProductRequest = {
    handle: parsed.data.handle && parsed.data.handle.length > 0 ? parsed.data.handle : undefined,
    title: parsed.data.title,
    description: parsed.data.description && parsed.data.description.length > 0 ? parsed.data.description : undefined,
    status: parsed.data.status,
    category_ids: parsed.data.categoryIds,
  };

  const result = await updateProduct(ctx.storeId, productId, body, { userId: ctx.userId, tenantId: ctx.tenantId });
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

  const result = await deleteProduct(ctx.storeId, productId, { userId: ctx.userId, tenantId: ctx.tenantId });
  if (!result.ok) {
    return { ok: false, error: result.error };
  }

  revalidatePath("/products");
  redirect("/products");
}
