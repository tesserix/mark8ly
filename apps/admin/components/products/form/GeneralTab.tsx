"use client";

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
import { Field } from "./Field";
import { isUsableWeight } from "@/lib/products/usable-weight";
import type { Warehouse } from "@/lib/api/warehouses-api";
import { VariantStockByWarehouse } from "./VariantStockByWarehouse";

export interface GeneralTabProps {
  mode: "create" | "edit";
  categories: AdminCategory[];
  currencyCode: string;
  hasMultipleVariants: boolean;
  storeId?: string;
  /**
   * The store's live warehouses (#177 PR 5e). With fewer than two, the
   * single Stock field below stays exactly as it was — a store with one
   * warehouse must see no change at all.
   */
  warehouses?: Warehouse[];
  /** Set on edit only — the per-warehouse editor saves against a real variant. */
  productId?: string;
  variantId?: string;
  /** Current per-warehouse breakdown for that variant. */
  stockByLocation?: Record<string, number>;
}

export function GeneralTab({
  mode,
  categories,
  currencyCode,
  hasMultipleVariants,
  storeId,
  warehouses = [],
  productId,
  variantId,
  stockByLocation = {},
}: GeneralTabProps) {
  const form = useFormContext<ProductFormValues>();
  const { register, formState, watch, setValue } = form;

  // Blank, whitespace, unparseable or non-positive all mean "no usable
  // weight" — each ends up as the fallback at checkout.
  const weightKgValue = watch("weightKg");
  const isWeightMissing = !isUsableWeight(weightKgValue);

  // Per-warehouse stock replaces the single field only when there is an
  // actual choice to make AND a saved variant to write against. On create
  // there is no variant id yet, so the single field stays and the merchant
  // splits the stock after the product exists.
  const perWarehouseStock =
    warehouses.length > 1 &&
    mode === "edit" &&
    !hasMultipleVariants &&
    Boolean(storeId && productId && variantId);

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
        <div className="rounded-md border border-[color:var(--ink-900)] border-opacity-10 bg-[color:var(--paper-200)] px-4 py-3 text-sm text-foreground-secondary">
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
          {!perWarehouseStock && (
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
          )}
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

      {perWarehouseStock && storeId && productId && variantId && (
        <VariantStockByWarehouse
          storeId={storeId}
          productId={productId}
          variantId={variantId}
          warehouses={warehouses}
          byLocation={stockByLocation}
        />
      )}

      {!hasMultipleVariants && (
        <label className="flex items-start gap-2 text-sm text-[color:var(--ink-900)]">
          <input
            type="checkbox"
            {...register("alwaysInStock")}
            className="mt-[3px] h-4 w-4 rounded border border-[color:var(--ink-900)] border-opacity-30"
          />
          <span>
            <span className="font-medium">Always in stock</span>
            <span className="ml-2 text-[color:var(--ink-900)] opacity-60">
              Continue selling this product when stock reaches zero
              (sets <code>inventory_policy=continue</code> on the variant).
            </span>
          </span>
        </label>
      )}

      {!hasMultipleVariants && (
        <fieldset className="rounded-md border border-[color:var(--ink-900)] border-opacity-10 bg-[color:var(--paper-200)] px-4 py-3">
          <legend className="px-2 text-xs font-semibold uppercase tracking-[0.12em] text-[color:var(--ink-900)] opacity-60">
            Shipping
          </legend>
          <p className="mb-3 mt-1 text-xs text-[color:var(--ink-900)] opacity-60">
            Carriers (Australia Post / ShipEngine / etc.) need package
            dimensions and weight to quote real rates. Blank dimensions fall
            back to a 30 × 20 × 10 cm envelope. Weight is different — see below.
          </p>
          <div className="grid gap-4 sm:grid-cols-4">
            <Field
              label="Weight (kg)"
              error={formState.errors.weightKg?.message}
            >
              <input
                type="text"
                inputMode="decimal"
                placeholder="1.20"
                {...register("weightKg")}
                className="w-full rounded-md border border-[color:var(--ink-900)] border-opacity-20 bg-[color:var(--background-elevated,white)] px-3 py-2 text-sm text-[color:var(--ink-900)] focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)]"
              />
              {/* A blank weight is not a neutral default the way blank
                  dimensions are: checkout substitutes the store's fallback
                  and the shopper is charged carrier rates derived from it.
                  Silent until the field is actually empty, so it reads as
                  guidance rather than an error. */}
              {isWeightMissing && (
                <p className="mt-1 text-xs text-[color:var(--warning)]">
                  No weight set — shipping is quoted using your store&rsquo;s
                  default parcel weight, so rates for this product may be
                  wrong. Set a weight for accurate pricing.
                </p>
              )}
            </Field>
            <Field
              label="Length (cm)"
              error={formState.errors.lengthCm?.message}
            >
              <input
                type="text"
                inputMode="decimal"
                placeholder="30"
                {...register("lengthCm")}
                className="w-full rounded-md border border-[color:var(--ink-900)] border-opacity-20 bg-[color:var(--background-elevated,white)] px-3 py-2 text-sm text-[color:var(--ink-900)] focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)]"
              />
            </Field>
            <Field
              label="Width (cm)"
              error={formState.errors.widthCm?.message}
            >
              <input
                type="text"
                inputMode="decimal"
                placeholder="20"
                {...register("widthCm")}
                className="w-full rounded-md border border-[color:var(--ink-900)] border-opacity-20 bg-[color:var(--background-elevated,white)] px-3 py-2 text-sm text-[color:var(--ink-900)] focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)]"
              />
            </Field>
            <Field
              label="Height (cm)"
              error={formState.errors.heightCm?.message}
            >
              <input
                type="text"
                inputMode="decimal"
                placeholder="10"
                {...register("heightCm")}
                className="w-full rounded-md border border-[color:var(--ink-900)] border-opacity-20 bg-[color:var(--background-elevated,white)] px-3 py-2 text-sm text-[color:var(--ink-900)] focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)]"
              />
            </Field>
          </div>
        </fieldset>
      )}
    </div>
  );
}
