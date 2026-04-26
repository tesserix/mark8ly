"use client";

import * as React from "react";
import type { AdminMediaResponse } from "@/lib/api/marketplace-api";
import { VariantImagePicker } from "./VariantImagePicker";
import type { VariantDraft } from "./VariantMatrixTable";

export interface VariantRowProps {
  variant: VariantDraft;
  optionNames: string[];
  currencyCode: string;
  media: AdminMediaResponse[];
  onPatch: (patch: Partial<VariantDraft>) => void;
}

function findValue(variant: VariantDraft, optionName: string): string {
  return variant.optionValues.find((ov) => ov.optionName === optionName)?.value ?? "";
}

const tabularNum: React.CSSProperties = { fontVariantNumeric: "tabular-nums" };

export function VariantRow({
  variant,
  optionNames,
  media,
  onPatch,
}: VariantRowProps): React.ReactElement {
  const [price, setPrice] = React.useState(variant.price);
  const [sku, setSku] = React.useState(variant.sku);
  const [stock, setStock] = React.useState(String(variant.stock));
  const [weight, setWeight] = React.useState(String(variant.weight));
  const [lengthCm, setLengthCm] = React.useState(
    variant.lengthCm ? String(variant.lengthCm) : "",
  );
  const [widthCm, setWidthCm] = React.useState(
    variant.widthCm ? String(variant.widthCm) : "",
  );
  const [heightCm, setHeightCm] = React.useState(
    variant.heightCm ? String(variant.heightCm) : "",
  );
  const [pickerOpen, setPickerOpen] = React.useState(false);

  React.useEffect(() => setPrice(variant.price), [variant.price]);
  React.useEffect(() => setSku(variant.sku), [variant.sku]);
  React.useEffect(() => setStock(String(variant.stock)), [variant.stock]);
  React.useEffect(() => setWeight(String(variant.weight)), [variant.weight]);
  React.useEffect(
    () => setLengthCm(variant.lengthCm ? String(variant.lengthCm) : ""),
    [variant.lengthCm],
  );
  React.useEffect(
    () => setWidthCm(variant.widthCm ? String(variant.widthCm) : ""),
    [variant.widthCm],
  );
  React.useEffect(
    () => setHeightCm(variant.heightCm ? String(variant.heightCm) : ""),
    [variant.heightCm],
  );

  const currentMedia = media.find((m) => m.id === variant.variantImageId) ?? null;

  const commitPrice = (): void => {
    if (price !== variant.price) onPatch({ price });
  };
  const commitSku = (): void => {
    if (sku !== variant.sku) onPatch({ sku });
  };
  const commitStock = (): void => {
    const n = Number.parseInt(stock, 10);
    if (!Number.isNaN(n) && n !== variant.stock) onPatch({ stock: n });
  };
  const commitWeight = (): void => {
    const n = Number.parseFloat(weight);
    if (!Number.isNaN(n) && n !== variant.weight) onPatch({ weight: n });
  };
  const commitDim = (
    raw: string,
    field: "lengthCm" | "widthCm" | "heightCm",
  ): void => {
    if (raw.trim() === "") {
      if (variant[field] !== undefined) onPatch({ [field]: undefined });
      return;
    }
    const n = Number.parseFloat(raw);
    if (!Number.isNaN(n) && n !== variant[field]) onPatch({ [field]: n });
  };

  return (
    <tr className="border-b border-[var(--ink-100)]">
      {optionNames.map((name) => (
        <td key={name} className="px-3 py-2 text-sm text-[var(--ink-900)]">
          {findValue(variant, name)}
        </td>
      ))}
      <td className="px-3 py-2">
        <input
          aria-label="Price"
          type="text"
          value={price}
          onChange={(e) => setPrice(e.target.value)}
          onBlur={commitPrice}
          style={tabularNum}
          className="w-24 bg-transparent px-2 py-1 text-sm text-[var(--ink-900)] outline-none focus:ring-2 focus:ring-[var(--moss-700)]"
        />
      </td>
      <td className="px-3 py-2">
        <input
          aria-label="SKU"
          type="text"
          value={sku}
          onChange={(e) => setSku(e.target.value)}
          onBlur={commitSku}
          className="w-32 bg-transparent px-2 py-1 text-sm text-[var(--ink-900)] outline-none focus:ring-2 focus:ring-[var(--moss-700)]"
        />
      </td>
      <td className="px-3 py-2">
        <input
          aria-label="Stock"
          type="number"
          value={stock}
          onChange={(e) => setStock(e.target.value)}
          onBlur={commitStock}
          style={tabularNum}
          className="w-20 bg-transparent px-2 py-1 text-sm text-[var(--ink-900)] outline-none focus:ring-2 focus:ring-[var(--moss-700)]"
        />
      </td>
      <td className="px-3 py-2">
        <input
          aria-label="Weight"
          type="number"
          step="0.01"
          value={weight}
          onChange={(e) => setWeight(e.target.value)}
          onBlur={commitWeight}
          style={tabularNum}
          className="w-20 bg-transparent px-2 py-1 text-sm text-[var(--ink-900)] outline-none focus:ring-2 focus:ring-[var(--moss-700)]"
        />
      </td>
      <td className="px-3 py-2">
        <div className="flex items-center gap-1" style={tabularNum}>
          <input
            aria-label="Length (cm)"
            type="number"
            step="0.01"
            placeholder="L"
            value={lengthCm}
            onChange={(e) => setLengthCm(e.target.value)}
            onBlur={() => commitDim(lengthCm, "lengthCm")}
            className="w-14 bg-transparent px-2 py-1 text-sm text-[var(--ink-900)] outline-none focus:ring-2 focus:ring-[var(--moss-700)]"
          />
          <span className="text-xs text-[var(--ink-500)]">×</span>
          <input
            aria-label="Width (cm)"
            type="number"
            step="0.01"
            placeholder="W"
            value={widthCm}
            onChange={(e) => setWidthCm(e.target.value)}
            onBlur={() => commitDim(widthCm, "widthCm")}
            className="w-14 bg-transparent px-2 py-1 text-sm text-[var(--ink-900)] outline-none focus:ring-2 focus:ring-[var(--moss-700)]"
          />
          <span className="text-xs text-[var(--ink-500)]">×</span>
          <input
            aria-label="Height (cm)"
            type="number"
            step="0.01"
            placeholder="H"
            value={heightCm}
            onChange={(e) => setHeightCm(e.target.value)}
            onBlur={() => commitDim(heightCm, "heightCm")}
            className="w-14 bg-transparent px-2 py-1 text-sm text-[var(--ink-900)] outline-none focus:ring-2 focus:ring-[var(--moss-700)]"
          />
        </div>
      </td>
      <td className="relative px-3 py-2">
        <button
          type="button"
          aria-label="Variant image"
          onClick={() => setPickerOpen((o) => !o)}
          className="flex h-10 w-10 items-center justify-center overflow-hidden rounded-md border border-[var(--ink-100)] bg-[var(--background-elevated)] text-[var(--ink-500)] focus:outline-none focus:ring-2 focus:ring-[var(--moss-700)]"
        >
          {currentMedia ? (
            // eslint-disable-next-line @next/next/no-img-element
            <img
              src={currentMedia.url}
              alt={currentMedia.alt ?? ""}
              className="h-full w-full object-cover"
            />
          ) : (
            <span className="text-lg">+</span>
          )}
        </button>
        {pickerOpen ? (
          <div className="absolute right-0 top-12 z-10">
            <VariantImagePicker
              media={media}
              currentMediaId={variant.variantImageId ?? null}
              onSelect={(id) => {
                onPatch({ variantImageId: id });
                setPickerOpen(false);
              }}
            />
          </div>
        ) : null}
      </td>
    </tr>
  );
}
