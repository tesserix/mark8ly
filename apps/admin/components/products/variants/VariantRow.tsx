"use client";

import * as React from "react";
import type { AdminMediaResponse } from "@/lib/api/marketplace-api";
import { VariantImagePicker } from "./VariantImagePicker";
import type { VariantDraft } from "./VariantMatrixTable";
import { VariantStockByWarehouse } from "@/components/products/form/VariantStockByWarehouse";
import type { Warehouse } from "@/lib/api/warehouses-api";

export interface VariantRowProps {
  variant: VariantDraft;
  optionNames: string[];
  currencyCode: string;
  media: AdminMediaResponse[];
  onPatch: (patch: Partial<VariantDraft>) => void;
  /**
   * Per-warehouse stock (#177 PR 6). With fewer than two warehouses the
   * Stock cell stays a plain number and this row is byte-for-byte what it
   * was — a store with one location has no split to make.
   */
  warehouses?: Warehouse[];
  storeId?: string;
  productId?: string;
  /** This variant's current breakdown, keyed by warehouse id. */
  stockByLocation?: Record<string, number>;
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
  warehouses = [],
  storeId,
  productId,
  stockByLocation = {},
}: VariantRowProps): React.ReactElement {
  const [splitOpen, setSplitOpen] = React.useState(false);
  const splitTriggerRef = React.useRef<HTMLButtonElement>(null);

  // The split is offered only when there is a choice to make AND a saved
  // variant to write against: per-warehouse stock saves through the
  // variant PATCH, which needs an id. A variant generated but not yet
  // saved keeps the plain total until the product is saved.
  const canSplit =
    warehouses.length > 1 && Boolean(variant.id && storeId && productId);

  // What the collapsed cell says. A count ("in 2") tells the merchant
  // nothing they cannot already see; WHERE the units are is the thing they
  // opened the page for, and it doubles as the at-a-glance flag for a
  // variant stocked in only one place or carrying unassigned units.
  const assigned = warehouses.reduce(
    (sum, w) => sum + (stockByLocation[w.id] ?? 0),
    0,
  );
  const unassigned = Math.max(variant.stock - assigned, 0);
  const stocked = warehouses.filter((w) => (stockByLocation[w.id] ?? 0) > 0);

  let splitSummary: string;
  if (unassigned > 0) {
    splitSummary = `${unassigned} unassigned`;
  } else if (stocked.length === 0) {
    splitSummary = "Not stocked";
  } else if (stocked.length === 1) {
    // "only" carries the signal in words rather than adding a second
    // colour. Stocking one location is often deliberate, so it is not a
    // warning — just worth saying.
    splitSummary = `${stocked[0]!.city || stocked[0]!.name} only`;
  } else if (stocked.length === 2) {
    splitSummary = stocked.map((w) => w.city || w.name).join(", ");
  } else {
    splitSummary = `${stocked[0]!.city || stocked[0]!.name} +${stocked.length - 1}`;
  }

  // Colour is reserved for the one state that is a data-hygiene problem.
  // Vermillion (--signal) stays for destructive/error; unassigned stock is
  // amber-bronze, matching the same warning the single-variant panel uses.
  const summaryClass = unassigned > 0
    ? "text-xs text-[color:var(--warning)]"
    : "text-xs text-[color:var(--ink-900)]/40";

  // WCAG 1.4.1: colour alone never carries the state, so the accessible
  // name says it too.
  const stockLabel =
    `Stock, ${variant.stock} units — ${splitSummary}. Expand to edit by warehouse.`;
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

  const totalColumns = optionNames.length + 6;

  return (
    <>
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
        {canSplit ? (
          // A disclosure control, not a stunted text field. Every sibling
          // cell is an input; styling this one to half-resemble one made it
          // read as broken. Borderless, chevron, serif numeral — closer to
          // a footnote reference than a form control.
          <button
            type="button"
            aria-expanded={splitOpen}
            aria-controls={`stock-panel-${variant.id}`}
            aria-label={stockLabel}
            ref={splitTriggerRef}
            onClick={() => {
              setSplitOpen((open) => {
                // Collapsing returns focus to this button rather than
                // dumping the keyboard user at the top of a long table.
                if (open) {
                  requestAnimationFrame(() => splitTriggerRef.current?.focus());
                }
                return !open;
              });
            }}
            className="group flex flex-col items-start gap-0.5 rounded-md px-2 py-1 text-left hover:bg-[color:var(--ink-900)]/[0.03] focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)]"
          >
            <span className="flex items-center gap-1.5">
              <span
                style={tabularNum}
                className="font-[family-name:var(--font-serif,'Source_Serif_4',serif)] text-sm text-[var(--ink-900)]"
              >
                {variant.stock}
              </span>
              <svg
                width="10"
                height="10"
                viewBox="0 0 10 10"
                aria-hidden="true"
                className={`text-[color:var(--ink-900)]/40 transition-transform group-hover:text-[color:var(--moss-700)] motion-reduce:transition-none ${splitOpen ? "rotate-180" : ""}`}
              >
                <path d="M2 4l3 3 3-3" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round" />
              </svg>
            </span>
            <span className={summaryClass}>{splitSummary}</span>
          </button>
        ) : (
          <input
            aria-label="Stock"
            type="number"
            value={stock}
            onChange={(e) => setStock(e.target.value)}
            onBlur={commitStock}
            style={tabularNum}
            className="w-20 bg-transparent px-2 py-1 text-sm text-[var(--ink-900)] outline-none focus:ring-2 focus:ring-[var(--moss-700)]"
          />
        )}
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
    {canSplit && splitOpen && (
      // A full-width sub-row rather than extra columns: the split is a
      // detail of ONE variant, and widening every row to carry a column per
      // warehouse would make the common single-warehouse table worse to
      // serve the uncommon case.
      <tr className="border-b border-[var(--ink-100)]">
        <td colSpan={totalColumns} className="px-3 py-6">
          {/* Constrained rather than full-bleed, on the page's own paper
              rather than a tinted panel: the design system is hairline
              rules, not cards, and a full-width filled block in a dense
              table reads as a layout break rather than detail attached to
              this row. */}
          <div id={`stock-panel-${variant.id}`} className="max-w-xl">
          <VariantStockByWarehouse
            storeId={storeId!}
            productId={productId!}
            variantId={variant.id!}
            warehouses={warehouses}
            byLocation={stockByLocation}
            autoFocus
          />
          </div>
        </td>
      </tr>
    )}
    </>
  );
}
