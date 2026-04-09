"use client";

// apps/admin/components/products/ProductForm.tsx
//
// The product detail form. Drives create + simple-update flows. Variants,
// media, rich text, and SEO all deferred to M7c/polish per M7b plan.

import Link from "next/link";
import { useRouter } from "next/navigation";
import { useState, useTransition } from "react";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { ArrowLeft, Trash2, MoreHorizontal } from "lucide-react";

import {
  productFormSchema,
  type ProductFormValues,
} from "@/lib/validation/product-form";
import type { AdminProduct, AdminCategory } from "@/lib/api/marketplace-api";
import {
  createProductAction,
  updateProductAction,
  deleteProductAction,
} from "@/app/products/actions";

import { ProductCategoriesPicker } from "./ProductCategoriesPicker";

export interface ProductFormProps {
  mode: "create" | "edit";
  storeId: string;
  initialProduct?: AdminProduct;
  categories: AdminCategory[];
  currencyCode: string;
  canDelete: boolean;
  canArchive: boolean;
}

export function ProductForm({
  mode,
  storeId,
  initialProduct,
  categories,
  currencyCode,
  canDelete,
}: ProductFormProps) {
  const router = useRouter();
  const [isPending, startTransition] = useTransition();
  const [rootError, setRootError] = useState<string | null>(null);
  const hasMultipleVariants = (initialProduct?.variants.length ?? 0) > 1;
  const firstVariant = initialProduct?.variants[0];

  const defaults: ProductFormValues = {
    title: initialProduct?.title ?? "",
    handle: initialProduct?.handle ?? "",
    description: initialProduct?.description ?? "",
    status: initialProduct?.status ?? "draft",
    price: firstVariant?.price ?? "",
    inventoryQuantity: firstVariant
      ? String(firstVariant.inventory_quantity)
      : "0",
    sku: firstVariant?.sku ?? "",
    categoryIds: initialProduct?.categories.map((c) => c.id) ?? [],
  };

  const form = useForm<ProductFormValues>({
    resolver: zodResolver(productFormSchema),
    defaultValues: defaults,
  });

  const onSubmit = (values: ProductFormValues) => {
    setRootError(null);
    startTransition(async () => {
      if (mode === "create") {
        const result = await createProductAction(storeId, currencyCode, values);
        if (!result.ok && result.error) {
          applyError(result.error);
        }
        // On success, createProductAction calls redirect() and never returns.
      } else if (initialProduct) {
        const result = await updateProductAction(
          storeId,
          initialProduct.id,
          currencyCode,
          values,
        );
        if (!result.ok && result.error) {
          applyError(result.error);
        } else {
          router.refresh();
        }
      }
    });
  };

  const applyError = (err: {
    code: string;
    message: string;
    field?: string;
  }) => {
    if (err.field) {
      form.setError(err.field as keyof ProductFormValues, {
        type: "server",
        message: err.message,
      });
    } else {
      setRootError(err.message);
    }
  };

  const handleDelete = () => {
    if (!initialProduct) return;
    if (!window.confirm(`Delete "${initialProduct.title}"? This cannot be undone.`)) return;
    startTransition(async () => {
      const result = await deleteProductAction(storeId, initialProduct.id);
      if (!result.ok && result.error) {
        setRootError(result.error.message);
      }
    });
  };

  const title = mode === "create" ? "New product" : (initialProduct?.title ?? "Product");

  return (
    <form
      onSubmit={form.handleSubmit(onSubmit)}
      className="flex flex-col gap-8"
      aria-labelledby="product-form-heading"
    >
      {/* Header strip */}
      <div className="flex flex-col gap-2">
        <Link
          href="/products"
          className="inline-flex w-fit items-center gap-1 text-sm text-[color:var(--ink-900)] opacity-70 transition-opacity hover:opacity-100 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)]"
        >
          <ArrowLeft className="h-4 w-4" aria-hidden="true" /> Products
        </Link>
        <div className="flex items-start justify-between gap-4">
          <h1
            id="product-form-heading"
            className="font-[family-name:var(--font-serif,'Source_Serif_4',serif)] text-4xl leading-tight text-[color:var(--ink-900)]"
          >
            {title}
          </h1>
          {mode === "edit" && initialProduct && (
            <span className="text-xs text-[color:var(--ink-900)] opacity-50">
              /{initialProduct.handle}
            </span>
          )}
        </div>
      </div>

      {rootError && (
        <div
          role="alert"
          className="rounded-md border border-[color:var(--signal,#C23B22)] border-opacity-40 bg-[color:var(--signal,#C23B22)] bg-opacity-5 px-4 py-3 text-sm text-[color:var(--signal,#C23B22)]"
        >
          {rootError}
        </div>
      )}

      {/* Title */}
      <Field label="Title" error={form.formState.errors.title?.message}>
        <input
          type="text"
          {...form.register("title")}
          className="w-full rounded-md border border-[color:var(--ink-900)] border-opacity-20 bg-[color:var(--background-elevated,white)] px-3 py-2 text-base text-[color:var(--ink-900)] focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)]"
        />
      </Field>

      {/* Handle */}
      <Field
        label="Handle"
        helper={
          mode === "create"
            ? "Leave empty to auto-generate from the title. mark8ly.com/<handle>"
            : "Changing the handle breaks existing links. mark8ly.com/<handle>"
        }
        error={form.formState.errors.handle?.message}
      >
        <input
          type="text"
          {...form.register("handle")}
          placeholder="linen-shirt"
          className="w-full rounded-md border border-[color:var(--ink-900)] border-opacity-20 bg-[color:var(--background-elevated,white)] px-3 py-2 text-sm text-[color:var(--ink-900)] focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)]"
        />
      </Field>

      {/* Description */}
      <Field
        label="Description"
        helper="Plain text only for now; rich text lands in a follow-up."
        error={form.formState.errors.description?.message}
      >
        <textarea
          {...form.register("description")}
          rows={6}
          className="w-full resize-vertical rounded-md border border-[color:var(--ink-900)] border-opacity-20 bg-[color:var(--background-elevated,white)] px-3 py-2 text-sm text-[color:var(--ink-900)] focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)]"
        />
      </Field>

      {/* Status */}
      <Field label="Status" error={form.formState.errors.status?.message}>
        <select
          {...form.register("status")}
          className="w-full rounded-md border border-[color:var(--ink-900)] border-opacity-20 bg-[color:var(--background-elevated,white)] px-3 py-2 text-sm text-[color:var(--ink-900)] focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)]"
        >
          <option value="draft">Draft</option>
          <option value="active">Active</option>
          <option value="archived">Archived</option>
        </select>
      </Field>

      {/* Categories */}
      <Field label="Categories" error={form.formState.errors.categoryIds?.message}>
        <ProductCategoriesPicker
          allCategories={categories}
          selectedIds={form.watch("categoryIds")}
          onChange={(ids) =>
            form.setValue("categoryIds", ids, { shouldDirty: true })
          }
        />
      </Field>

      {/* Pricing / Inventory — simple product only */}
      {hasMultipleVariants ? (
        <div className="rounded-md border border-[color:var(--ink-900)] border-opacity-10 bg-[color:var(--paper-200)] px-4 py-3 text-sm text-[color:var(--ink-900)] opacity-70">
          This product has variants. Price and stock live in the variant editor
          (coming in M7c).
        </div>
      ) : (
        <div className="grid gap-4 sm:grid-cols-3">
          <Field
            label={`Price (${currencyCode})`}
            error={form.formState.errors.price?.message}
          >
            <input
              type="text"
              inputMode="decimal"
              placeholder="19.99"
              {...form.register("price")}
              className="w-full rounded-md border border-[color:var(--ink-900)] border-opacity-20 bg-[color:var(--background-elevated,white)] px-3 py-2 text-sm text-[color:var(--ink-900)] focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)]"
            />
          </Field>
          <Field
            label="Stock"
            error={form.formState.errors.inventoryQuantity?.message}
          >
            <input
              type="text"
              inputMode="numeric"
              placeholder="0"
              {...form.register("inventoryQuantity")}
              className="w-full rounded-md border border-[color:var(--ink-900)] border-opacity-20 bg-[color:var(--background-elevated,white)] px-3 py-2 text-sm text-[color:var(--ink-900)] focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)]"
            />
          </Field>
          <Field label="SKU" error={form.formState.errors.sku?.message}>
            <input
              type="text"
              placeholder="Auto-generated from handle"
              {...form.register("sku")}
              className="w-full rounded-md border border-[color:var(--ink-900)] border-opacity-20 bg-[color:var(--background-elevated,white)] px-3 py-2 text-sm text-[color:var(--ink-900)] focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)]"
            />
          </Field>
        </div>
      )}

      {mode === "edit" && hasMultipleVariants && (
        <p className="text-xs text-[color:var(--ink-900)] opacity-50">
          M7b updates only basics on edit. Price and stock changes on variant
          products require the variant editor (M7c).
        </p>
      )}

      {/* Actions */}
      <div className="flex items-center justify-between border-t border-[color:var(--ink-900)] border-opacity-10 pt-6">
        <Link
          href="/products"
          className="text-sm text-[color:var(--ink-900)] opacity-70 underline-offset-4 hover:underline focus-visible:underline focus-visible:outline-none"
        >
          Discard
        </Link>
        <div className="flex items-center gap-3">
          {mode === "edit" && canDelete && initialProduct && (
            <button
              type="button"
              onClick={handleDelete}
              disabled={isPending}
              className="inline-flex items-center gap-1 rounded-md border border-[color:var(--ink-900)] border-opacity-20 px-3 py-2 text-sm text-[color:var(--signal,#C23B22)] transition-colors hover:bg-[color:var(--signal,#C23B22)] hover:bg-opacity-5 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)] disabled:opacity-50"
              aria-label="Delete product"
            >
              <Trash2 className="h-4 w-4" aria-hidden="true" /> Delete
            </button>
          )}
          <button
            type="submit"
            disabled={isPending}
            className="inline-flex items-center gap-2 rounded-md bg-[color:var(--moss-700)] px-5 py-2 text-sm text-[color:var(--paper-200)] transition-opacity hover:opacity-90 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)] disabled:opacity-50"
          >
            {isPending
              ? "Saving…"
              : mode === "create"
                ? "Create product"
                : "Save changes"}
          </button>
        </div>
      </div>
    </form>
  );
}

function Field({
  label,
  children,
  error,
  helper,
}: {
  label: string;
  children: React.ReactNode;
  error?: string;
  helper?: string;
}) {
  return (
    <label className="flex flex-col gap-2">
      <span className="text-sm font-medium text-[color:var(--ink-900)]">
        {label}
      </span>
      {children}
      {helper && !error && (
        <span className="text-xs text-[color:var(--ink-900)] opacity-50">
          {helper}
        </span>
      )}
      {error && (
        <span className="text-xs text-[color:var(--signal,#C23B22)]" role="alert">
          {error}
        </span>
      )}
    </label>
  );
}
