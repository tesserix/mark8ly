"use client";

import type { ReactNode } from "react";
import { useFormContext } from "react-hook-form";
import type { AdminCategory } from "@/lib/api/marketplace-api";
import type { ProductFormValues } from "@/lib/validation/product-form";
import { ProductCategoriesPicker } from "../ProductCategoriesPicker";

export interface GeneralTabProps {
  mode: "create" | "edit";
  categories: AdminCategory[];
  currencyCode: string;
  hasMultipleVariants: boolean;
}

export function GeneralTab({
  mode,
  categories,
  currencyCode,
  hasMultipleVariants,
}: GeneralTabProps) {
  const form = useFormContext<ProductFormValues>();
  const { register, formState, watch, setValue } = form;

  return (
    <div className="flex flex-col gap-8">
      <Field label="Title" error={formState.errors.title?.message}>
        <input
          type="text"
          {...register("title")}
          className="w-full rounded-md border border-[color:var(--ink-900)] border-opacity-20 bg-[color:var(--background-elevated,white)] px-3 py-2 text-base text-[color:var(--ink-900)] focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)]"
        />
      </Field>

      <Field
        label="Handle"
        helper={
          mode === "create"
            ? "Leave empty to auto-generate from the title. mark8ly.com/<handle>"
            : "Changing the handle breaks existing links. mark8ly.com/<handle>"
        }
        error={formState.errors.handle?.message}
      >
        <input
          type="text"
          {...register("handle")}
          placeholder="linen-shirt"
          className="w-full rounded-md border border-[color:var(--ink-900)] border-opacity-20 bg-[color:var(--background-elevated,white)] px-3 py-2 text-sm text-[color:var(--ink-900)] focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)]"
        />
      </Field>

      <Field
        label="Description"
        helper="Plain text only for now; rich text lands in a follow-up."
        error={formState.errors.description?.message}
      >
        <textarea
          {...register("description")}
          rows={6}
          className="w-full resize-vertical rounded-md border border-[color:var(--ink-900)] border-opacity-20 bg-[color:var(--background-elevated,white)] px-3 py-2 text-sm text-[color:var(--ink-900)] focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)]"
        />
      </Field>

      <Field label="Status" error={formState.errors.status?.message}>
        <select
          {...register("status")}
          className="w-full rounded-md border border-[color:var(--ink-900)] border-opacity-20 bg-[color:var(--background-elevated,white)] px-3 py-2 text-sm text-[color:var(--ink-900)] focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)]"
        >
          <option value="draft">Draft</option>
          <option value="active">Active</option>
          <option value="archived">Archived</option>
        </select>
      </Field>

      <Field label="Categories" error={formState.errors.categoryIds?.message}>
        <ProductCategoriesPicker
          allCategories={categories}
          selectedIds={watch("categoryIds")}
          onChange={(ids) =>
            setValue("categoryIds", ids, { shouldDirty: true })
          }
        />
      </Field>

      {hasMultipleVariants ? (
        <div className="rounded-md border border-[color:var(--ink-900)] border-opacity-10 bg-[color:var(--paper-200)] px-4 py-3 text-sm text-[color:var(--ink-900)] opacity-70">
          This product has variants. Price and stock live in the Variants tab.
        </div>
      ) : (
        <div className="grid gap-4 sm:grid-cols-3">
          <Field
            label={`Price (${currencyCode})`}
            error={formState.errors.price?.message}
          >
            <input
              type="text"
              inputMode="decimal"
              placeholder="19.99"
              {...register("price")}
              className="w-full rounded-md border border-[color:var(--ink-900)] border-opacity-20 bg-[color:var(--background-elevated,white)] px-3 py-2 text-sm text-[color:var(--ink-900)] focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)]"
            />
          </Field>
          <Field
            label="Stock"
            error={formState.errors.inventoryQuantity?.message}
          >
            <input
              type="text"
              inputMode="numeric"
              placeholder="0"
              {...register("inventoryQuantity")}
              className="w-full rounded-md border border-[color:var(--ink-900)] border-opacity-20 bg-[color:var(--background-elevated,white)] px-3 py-2 text-sm text-[color:var(--ink-900)] focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)]"
            />
          </Field>
          <Field label="SKU" error={formState.errors.sku?.message}>
            <input
              type="text"
              placeholder="Auto-generated from handle"
              {...register("sku")}
              className="w-full rounded-md border border-[color:var(--ink-900)] border-opacity-20 bg-[color:var(--background-elevated,white)] px-3 py-2 text-sm text-[color:var(--ink-900)] focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)]"
            />
          </Field>
        </div>
      )}
    </div>
  );
}

function Field({
  label,
  children,
  error,
  helper,
}: {
  label: string;
  children: ReactNode;
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
