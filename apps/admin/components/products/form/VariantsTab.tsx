"use client";

import { useFormContext, useWatch } from "react-hook-form";
import {
  VariantMatrixTable,
  type VariantDraft,
} from "@/components/products/variants/VariantMatrixTable";
import { VariantBulkBar } from "@/components/products/variants/VariantBulkBar";
import type { ProductFormValues } from "@/lib/validation/product-form";
import type { AdminMediaResponse } from "@/lib/api/marketplace-api";

export interface VariantsTabProps {
  currencyCode: string;
}

type BulkPatch =
  | { field: "price"; value: string }
  | { field: "stock"; value: number }
  | { field: "weight"; value: number };

export function VariantsTab({ currencyCode }: VariantsTabProps) {
  const { control, setValue, getValues } = useFormContext<ProductFormValues>();
  const variants =
    (useWatch({ control, name: "variants" }) as VariantDraft[] | undefined) ?? [];
  const media =
    (useWatch({ control, name: "media" }) as unknown as AdminMediaResponse[] | undefined) ?? [];

  if (variants.length === 0) {
    return (
      <div className="rounded-md border border-dashed border-[color:var(--ink-900)] border-opacity-20 p-12 text-center">
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
      <VariantMatrixTable
        variants={variants}
        currencyCode={currencyCode}
        media={media}
        onPatch={handlePatch}
      />
    </div>
  );
}
