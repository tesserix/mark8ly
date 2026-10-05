"use client";

// Where the buyer's artwork sits on the product (#966).
//
// The storefront composites a shopper's photo into the merchant's mockup
// so they see "my photo on the actual shirt" before buying. That needs
// two things from the merchant: WHICH image is the mockup, and WHERE on
// it the print goes. Until this existed the database, the API and the
// types all supported both and there was no way to set either — the
// composite shipped and could never be reached.
//
// The rectangle is PERCENTAGES of the mockup's own dimensions, not
// pixels, so re-exporting the mockup at a different size does not
// invalidate it. That is also what lets the preview here and the
// storefront's composite be the same two lines of CSS.

import * as React from "react";
import { useState } from "react";
import Image from "next/image";

export interface MockupMedia {
  id: string;
  url: string;
  storage_key: string;
  alt?: string;
}

export interface PrintAreaValue {
  x: number;
  y: number;
  w: number;
  h: number;
}

export interface MockupConfigProps {
  media: readonly MockupMedia[];
  mockupStorageKey?: string;
  printArea?: PrintAreaValue;
  busy?: boolean;
  onSave: (patch: {
    mockup_storage_key: string | null;
    print_area: PrintAreaValue | null;
  }) => void;
}

const DEFAULT_AREA: PrintAreaValue = { x: 25, y: 25, w: 50, h: 50 };

/** Keeps a percentage inside the mockup. */
function clamp(n: number): number {
  if (!Number.isFinite(n)) return 0;
  return Math.min(100, Math.max(0, n));
}

export function MockupConfig({
  media,
  mockupStorageKey,
  printArea,
  busy = false,
  onSave,
}: MockupConfigProps): React.ReactElement {
  const [selectedKey, setSelectedKey] = useState<string | undefined>(mockupStorageKey);
  const [area, setArea] = useState<PrintAreaValue>(printArea ?? DEFAULT_AREA);

  const selected = media.find((m) => m.storage_key === selectedKey);

  const setField = (k: keyof PrintAreaValue, raw: string) => {
    const n = Number.parseFloat(raw);
    setArea((a) => ({ ...a, [k]: Number.isFinite(n) ? n : 0 }));
  };

  if (media.length === 0) {
    return (
      <p className="text-xs text-[var(--ink-500)]">
        Add a product image first — the mockup is one of your own photos,
        with the print area marked on it.
      </p>
    );
  }

  return (
    <div className="flex flex-col gap-3">
      <div className="flex flex-col gap-1">
        <span className="text-xs font-medium text-[var(--ink-700)]">
          Mockup image
        </span>
        <p className="text-xs text-[var(--ink-500)]">
          The photo the buyer sees their artwork on. Pick one of this
          product&apos;s images.
        </p>
      </div>

      <div className="flex flex-wrap gap-2" role="group" aria-label="Mockup image">
        {media.map((m) => {
          const isSelected = m.storage_key === selectedKey;
          return (
            <button
              key={m.id}
              type="button"
              aria-pressed={isSelected}
              onClick={() => setSelectedKey(isSelected ? undefined : m.storage_key)}
              className={`relative h-14 w-14 overflow-hidden rounded border-2 focus:outline-none focus:ring-2 focus:ring-[var(--moss-700)] ${
                isSelected ? "border-[var(--moss-700)]" : "border-[var(--ink-200)]"
              }`}
            >
              <Image src={m.url} alt={m.alt ?? ""} fill sizes="56px" className="object-cover" />
            </button>
          );
        })}
      </div>

      {selected ? (
        <>
          <div className="flex flex-col gap-1">
            <span className="text-xs font-medium text-[var(--ink-700)]">
              Print area
            </span>
            <p className="text-xs text-[var(--ink-500)]">
              Percentages of the image. The dashed box is what the buyer&apos;s
              photo will fill.
            </p>
          </div>

          {/* Live preview. Identical positioning to the storefront's
              composite, so what the merchant marks here is what the
              shopper sees. */}
          <div className="relative w-full max-w-[220px] overflow-hidden rounded border border-[var(--ink-200)]">
            <Image
              src={selected.url}
              alt=""
              width={220}
              height={220}
              className="h-auto w-full object-contain"
            />
            <span
              aria-hidden="true"
              className="absolute border-2 border-dashed border-[var(--moss-700)] bg-[var(--moss-700)]/10"
              style={{
                left: `${clamp(area.x)}%`,
                top: `${clamp(area.y)}%`,
                width: `${clamp(area.w)}%`,
                height: `${clamp(area.h)}%`,
              }}
            />
          </div>

          <div className="flex flex-wrap gap-3">
            {(["x", "y", "w", "h"] as const).map((k) => (
              <label key={k} className="flex flex-col gap-1 text-xs">
                <span className="uppercase tracking-wide text-[var(--ink-500)]">
                  {{ x: "Left %", y: "Top %", w: "Width %", h: "Height %" }[k]}
                </span>
                <input
                  type="number"
                  min={0}
                  max={100}
                  step={1}
                  value={area[k]}
                  onChange={(e) => setField(k, e.target.value)}
                  className="w-20 rounded-md border border-[var(--ink-200)] px-2 py-1"
                />
              </label>
            ))}
          </div>
        </>
      ) : null}

      <div className="flex items-center gap-3">
        <button
          type="button"
          disabled={busy || (!!selected && (area.w <= 0 || area.h <= 0))}
          onClick={() =>
            onSave(
              selected
                ? {
                    mockup_storage_key: selected.storage_key,
                    print_area: {
                      x: clamp(area.x),
                      y: clamp(area.y),
                      w: clamp(area.w),
                      h: clamp(area.h),
                    },
                  }
                : // Clearing is an explicit pair of nulls: a mockup with
                  // no rectangle, or a rectangle with no mockup, is half
                  // a feature and the storefront drops it anyway.
                  { mockup_storage_key: null, print_area: null },
            )
          }
          className="rounded-md bg-[var(--ink-900)] px-3 py-1.5 text-xs text-[var(--paper-200)] disabled:opacity-40"
        >
          {busy ? "Saving…" : selected ? "Save mockup" : "Remove mockup"}
        </button>
        {!!selected && (area.w <= 0 || area.h <= 0) ? (
          <span role="alert" className="text-xs text-[var(--danger)]">
            Width and height must be above zero.
          </span>
        ) : null}
      </div>
    </div>
  );
}
