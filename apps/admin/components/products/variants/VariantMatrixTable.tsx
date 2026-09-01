"use client";

import * as React from "react";
import type { AdminMediaResponse } from "@/lib/api/marketplace-api";
import type { Warehouse } from "@/lib/api/warehouses-api";
import { VariantRow } from "./VariantRow";

export interface VariantDraft {
  id?: string;
  key: string;
  price: string;
  sku: string;
  stock: number;
  weight: number;
  /** Optional package dimensions in centimetres. */
  lengthCm?: number;
  widthCm?: number;
  heightCm?: number;
  variantImageId?: string | null;
  optionValues: Array<{ optionName: string; value: string }>;
}

export interface VariantMatrixTableProps {
  variants: VariantDraft[];
  currencyCode: string;
  media: AdminMediaResponse[];
  onPatch: (key: string, patch: Partial<VariantDraft>) => void;
  /** Per-warehouse stock (#177 PR 6); fewer than two changes nothing. */
  warehouses?: Warehouse[];
  storeId?: string;
  productId?: string;
  /** variantId -> warehouseId -> quantity. */
  stockByLocation?: Record<string, Record<string, number>>;
}

export function VariantMatrixTable({
  variants,
  currencyCode,
  media,
  onPatch,
  warehouses = [],
  storeId,
  productId,
  stockByLocation = {},
}: VariantMatrixTableProps): React.ReactElement {
  const optionNames = React.useMemo<string[]>(() => {
    const first = variants[0];
    if (!first) return [];
    return first.optionValues.map((ov) => ov.optionName);
  }, [variants]);

  const priceHeader = React.useMemo(() => {
    try {
      const parts = new Intl.NumberFormat(undefined, {
        style: "currency",
        currency: currencyCode,
      }).formatToParts(0);
      const symbol = parts.find((p) => p.type === "currency")?.value ?? currencyCode;
      return `Price (${symbol})`;
    } catch {
      return `Price (${currencyCode})`;
    }
  }, [currencyCode]);

  return (
    // overflow-x-auto handles narrow viewports (mobile / small split panes),
    // but on md+ we need overflow-visible so the VariantImagePicker popover
    // can spill below the row. Otherwise CSS coerces overflow-y to auto and
    // the absolute popover triggers a vertical scrollbar on the wrapper.
    <div className="overflow-x-auto md:overflow-visible">
      <table className="w-full border-collapse text-left">
        <thead>
          <tr className="border-b border-[var(--ink-100)] text-xs uppercase tracking-widest text-[var(--ink-500)]">
            {optionNames.map((name) => (
              <th key={name} className="px-3 py-2 font-normal">
                {name}
              </th>
            ))}
            <th className="px-3 py-2 font-normal">{priceHeader}</th>
            <th className="px-3 py-2 font-normal">SKU</th>
            <th className="px-3 py-2 font-normal">Stock</th>
            <th className="px-3 py-2 font-normal">Weight (kg)</th>
            <th className="px-3 py-2 font-normal" title="Length × Width × Height in centimetres">
              L × W × H (cm)
            </th>
            <th className="px-3 py-2 font-normal">Image</th>
          </tr>
        </thead>
        <tbody>
          {variants.map((v) => (
            <VariantRow
              key={v.key}
              variant={v}
              optionNames={optionNames}
              currencyCode={currencyCode}
              media={media}
              onPatch={(patch) => onPatch(v.key, patch)}
              warehouses={warehouses}
              storeId={storeId}
              productId={productId}
              stockByLocation={v.id ? stockByLocation[v.id] : undefined}
            />
          ))}
        </tbody>
      </table>
    </div>
  );
}
