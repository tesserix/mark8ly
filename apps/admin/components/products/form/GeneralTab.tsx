"use client";

import {
  cloneElement,
  isValidElement,
  useId,
  type ReactElement,
  type ReactNode,
} from "react";
import { useFormContext } from "react-hook-form";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@tesserix/web";
import type { AdminCategory } from "@/lib/api/marketplace-api";
import type { ProductFormValues } from "@/lib/validation/product-form";
import { ProductCategoriesPicker } from "../ProductCategoriesPicker";

export interface GeneralTabProps {
  mode: "create" | "edit";
  categories: AdminCategory[];
  currencyCode: string;
  hasMultipleVariants: boolean;
  storeId?: string;
}

export function GeneralTab({
  mode,
  categories,
  currencyCode,
  hasMultipleVariants,
  storeId,
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
            ? "Leave empty to auto-generate from the title. Lives at your-store.mark8ly.com/products/<handle>."
            : "Changing the handle breaks existing links. Lives at your-store.mark8ly.com/products/<handle>."
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
        helper="Shown on the product page below the title."
        error={formState.errors.description?.message}
      >
        <textarea
          {...register("description")}
          rows={6}
          className="w-full resize-vertical rounded-md border border-[color:var(--ink-900)] border-opacity-20 bg-[color:var(--background-elevated,white)] px-3 py-2 text-sm text-[color:var(--ink-900)] focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)]"
        />
      </Field>

      <Field label="Status" error={formState.errors.status?.message}>
        <Select
          value={watch("status")}
          onValueChange={(value) =>
            setValue("status", value as ProductFormValues["status"], {
              shouldDirty: true,
            })
          }
        >
          <SelectTrigger className="w-full">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="draft">Draft</SelectItem>
            <SelectItem value="active">Active</SelectItem>
            <SelectItem value="archived">Archived</SelectItem>
          </SelectContent>
        </Select>
      </Field>

      <Field label="Categories" error={formState.errors.categoryIds?.message}>
        <ProductCategoriesPicker
          allCategories={categories}
          selectedIds={watch("categoryIds")}
          onChange={(ids) =>
            setValue("categoryIds", ids, { shouldDirty: true })
          }
          storeId={storeId}
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

interface FieldChildProps {
  id?: string;
  "aria-invalid"?: boolean;
  "aria-describedby"?: string;
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
  const generatedId = useId();
  const errorId = `${generatedId}-error`;
  const helperId = `${generatedId}-helper`;

  // Inject a11y props so screen readers announce validation errors and
  // associate helper text with the input. Safe for both native inputs and
  // wrapper components (extra props are ignored if unused).
  const enhanced =
    isValidElement<FieldChildProps>(children)
      ? cloneElement(children as ReactElement<FieldChildProps>, {
          id: children.props.id ?? generatedId,
          "aria-invalid": error ? true : undefined,
          "aria-describedby": error ? errorId : helper ? helperId : undefined,
        })
      : children;

  return (
    <label htmlFor={generatedId} className="flex flex-col gap-2">
      <span className="text-sm font-medium text-[color:var(--ink-900)]">
        {label}
      </span>
      {enhanced}
      {helper && !error && (
        <span
          id={helperId}
          className="text-xs text-[color:var(--ink-900)] opacity-50"
        >
          {helper}
        </span>
      )}
      {error && (
        <span
          id={errorId}
          className="text-xs text-[color:var(--signal,#C23B22)]"
          role="alert"
        >
          {error}
        </span>
      )}
    </label>
  );
}
