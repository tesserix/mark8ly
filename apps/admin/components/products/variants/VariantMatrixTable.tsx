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
    // Plain overflow-x-auto. The md:overflow-visible exception existed only
    // so the image picker's absolutely-positioned popover could spill below
    // the row; the picker now lives inside the row's details disclosure, so
    // nothing escapes the wrapper and the exception is gone with it.
    <div className="overflow-x-auto">
      <table className="w-full border-collapse text-left">
        <thead>
          {/* Four visible columns, not eight. Every option collapses into
              one Variant cell, and weight / dimensions / image move behind
              the per-row details disclosure. Eight columns did not fit a
              realistic admin width and pushed the table into a horizontal
              scroll on every product that had options at all. */}
          <tr className="border-b border-[var(--ink-100)] text-xs uppercase tracking-widest text-[var(--ink-500)]">
            <th className="px-3 py-2 font-normal">Variant</th>
            <th className="px-3 py-2 font-normal">{priceHeader}</th>
            <th className="px-3 py-2 font-normal">SKU</th>
            <th className="px-3 py-2 font-normal">Stock</th>
            <th className="w-px px-3 py-2 font-normal">
              <span className="sr-only">Details</span>
            </th>
          </tr>
        </thead>
        <tbody>
          {variants.map((v, index) => (
            <VariantRow
              key={v.key}
              variant={v}
              index={index}
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
