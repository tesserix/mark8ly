"use client";

import { useFormContext, useWatch } from "react-hook-form";
import {
  VariantMatrixTable,
  type VariantDraft,
} from "@/components/products/variants/VariantMatrixTable";
import { VariantBulkBar } from "@/components/products/variants/VariantBulkBar";
import type { ProductFormValues } from "@/lib/validation/product-form";
import type { AdminMediaResponse } from "@/lib/api/marketplace-api";
import type { Warehouse } from "@/lib/api/warehouses-api";

export interface VariantsTabProps {
  currencyCode: string;
  /** Per-warehouse stock (#177 PR 6); fewer than two changes nothing. */
  warehouses?: Warehouse[];
  storeId?: string;
  productId?: string;
  stockByLocation?: Record<string, Record<string, number>>;
}

type BulkPatch =
  | { field: "price"; value: string }
  | { field: "stock"; value: number }
  | { field: "weight"; value: number };

export function VariantsTab({
  currencyCode,
  warehouses = [],
  storeId,
  productId,
  stockByLocation = {},
}: VariantsTabProps) {
  const { control, setValue, getValues } = useFormContext<ProductFormValues>();
  const variants =
    (useWatch({ control, name: "variants" }) as VariantDraft[] | undefined) ?? [];
  const media =
    (useWatch({ control, name: "media" }) as unknown as AdminMediaResponse[] | undefined) ?? [];

  if (variants.length === 0) {
    return (
      <div className="border-t border-[color:var(--ink-100)] py-12 pl-1">
        <p className="font-[family-name:var(--font-serif,'Source_Serif_4',serif)] text-lg text-[color:var(--ink-900)] opacity-80">
          Add options on the Options tab to generate variants.
        </p>
      </div>
    );
  }

  const handlePatch = (key: string, patch: Partial<VariantDraft>): void => {
    const current = (getValues("variants") as VariantDraft[] | undefined) ?? [];
    const next = current.map((v) => (v.key === key ? { ...v, ...patch } : v));
    setValue("variants", next, { shouldDirty: true, shouldValidate: false });
  };

  const handleBulk = (patch: BulkPatch): void => {
    const current = (getValues("variants") as VariantDraft[] | undefined) ?? [];
    const next = current.map((v) => ({ ...v, [patch.field]: patch.value }));
    setValue("variants", next, { shouldDirty: true, shouldValidate: false });
  };

  return (
    <div className="flex flex-col gap-4">
      <VariantBulkBar
        variantCount={variants.length}
        currencyCode={currencyCode}
        onBulkPatch={handleBulk}
      />
      {warehouses.length > 1 && (
        // Two save models in one table is a real inconsistency, so it is
        // stated rather than disguised: prices and SKUs commit on blur with
        // the product, warehouse stock has its own Save. A merchant who
        // expects one Save for everything otherwise files a bug about
        // stock "not saving".
        <p className="border-b border-[var(--ink-100)] pb-3 text-xs text-[color:var(--ink-900)]/50">
          Prices, SKUs and dimensions save with the product. Warehouse stock
          saves on its own, inside each variant.
        </p>
      )}
      <VariantMatrixTable
        variants={variants}
        currencyCode={currencyCode}
        media={media}
        onPatch={handlePatch}
        warehouses={warehouses}
        storeId={storeId}
        productId={productId}
        stockByLocation={stockByLocation}
      />
    </div>
  );
}
