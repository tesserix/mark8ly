"use client";

// PricingSection — what this product costs and how many there are.
//
// THE point of this component: there is exactly ONE home for price, stock
// and SKU, whatever shape the product is. A single-variant product gets
// three plain inputs. A product with options gets the variant table. Same
// section, same scroll position, different rendering of the same idea.
//
// What it replaces: General held Price/Stock/SKU and HID them when
// variants existed, swapping in a sentence — "This product has variants.
// Price and stock live in the Variants tab." That sentence existed only
// to paper over a seam, and it made the 81% of products with one variant
// read about mechanics that do not apply to them. A merchant should never
// be told where their data went; it should be where they are looking.

import { useFormContext } from "react-hook-form";

import type { ProductFormValues } from "@/lib/validation/product-form";
import type { Warehouse } from "@/lib/api/warehouses-api";
import { Field } from "./Field";
import { VariantsTab } from "./VariantsTab";
import { VariantStockByWarehouse } from "./VariantStockByWarehouse";

// RHF's register() returns only { name, onChange, onBlur, ref } — no value
// and no defaultValue — so a server-rendered form emits EMPTY inputs and the
// values appear only once the client has hydrated and RHF has written them
// through its refs. On the product page that showed as a real flash: the
// heading and breadcrumb (plain JSX) were correct immediately while Title sat
// empty and Handle showed its placeholder, which reads exactly like a product
// with no data — or a failed load.
//
// Passing defaultValue puts the value in the server HTML. It affects nothing
// after hydration: RHF owns the input imperatively from then on, and a later
// reset() sets .value directly rather than consulting the attribute.
export interface PricingSectionProps {
  mode: "create" | "edit";
  currencyCode: string;
  hasMultipleVariants: boolean;
  storeId?: string;
  productId?: string;
  /** The single variant's id, when there is exactly one. */
  variantId?: string;
  warehouses?: Warehouse[];
  /** This variant's per-warehouse breakdown, for the single-variant case. */
  stockByLocation?: Record<string, number>;
  /** variantId -> warehouseId -> quantity, for the table case. */
  stockByVariant?: Record<string, Record<string, number>>;
}

const inputClass =
  "w-full rounded-md border border-[color:var(--ink-900)] border-opacity-20 bg-[color:var(--background-elevated,white)] px-3 py-2 text-sm text-[color:var(--ink-900)] focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)]";

export function PricingSection({
  mode,
  currencyCode,
  hasMultipleVariants,
  storeId,
  productId,
  variantId,
  warehouses = [],
  stockByLocation = {},
  stockByVariant = {},
}: PricingSectionProps) {
  const { register, formState, watch, getValues } = useFormContext<ProductFormValues>();

  // With options defined, the variants themselves carry price and stock —
  // the table IS this section.
  if (hasMultipleVariants) {
    return (
      <VariantsTab
        currencyCode={currencyCode}
        warehouses={warehouses}
        storeId={storeId}
        productId={productId}
        stockByLocation={stockByVariant}
      />
    );
  }

  // Per-warehouse stock replaces the single number only when there is a
  // choice to make AND a saved variant to write against.
  const perWarehouseStock =
    warehouses.length > 1 &&
    mode === "edit" &&
    Boolean(storeId && productId && variantId);

  return (
    <div className="space-y-6">
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
            defaultValue={getValues("price")}
            className={inputClass}
          />
        </Field>
        {!perWarehouseStock && (
          <Field label="Stock" error={formState.errors.inventoryQuantity?.message}>
            <input
              type="text"
              inputMode="numeric"
              placeholder="0"
              {...register("inventoryQuantity")}
              defaultValue={getValues("inventoryQuantity")}
              className={inputClass}
            />
          </Field>
        )}
        <Field label="SKU" error={formState.errors.sku?.message}>
          <input
            type="text"
            placeholder="Auto-generated from handle"
            {...register("sku")}
            defaultValue={getValues("sku")}
            className={inputClass}
          />
        </Field>
      </div>

      {perWarehouseStock && storeId && productId && variantId && (
        <VariantStockByWarehouse
          storeId={storeId}
          productId={productId}
          variantId={variantId}
          warehouses={warehouses}
          byLocation={stockByLocation}
        />
      )}

      <label className="flex items-start gap-2 text-sm text-[color:var(--ink-900)]">
        <input
          type="checkbox"
          {...register("alwaysInStock")}
          className="mt-[3px] h-4 w-4 rounded border border-[color:var(--ink-900)] border-opacity-30"
        />
        <span>
          <span className="font-medium">Always in stock</span>
          <span className="ml-2 text-[color:var(--ink-900)] opacity-60">
            Keep selling when stock reaches zero
          </span>
        </span>
      </label>
      {watch("alwaysInStock") && (
        <p className="text-xs text-[color:var(--ink-900)]/50">
          Stock still counts down and is recorded on each order — it just
          never blocks a sale.
        </p>
      )}
    </div>
  );
}
