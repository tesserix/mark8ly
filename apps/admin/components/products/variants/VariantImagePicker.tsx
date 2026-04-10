"use client";

import * as React from "react";
import type { AdminMediaResponse } from "@/lib/api/marketplace-api";

export interface VariantImagePickerProps {
  media: AdminMediaResponse[];
  currentMediaId: string | null;
  onSelect: (mediaId: string | null) => void;
}

export function VariantImagePicker({
  media,
  currentMediaId,
  onSelect,
}: VariantImagePickerProps): React.ReactElement {
  return (
    <div className="flex flex-col gap-3 rounded-[6px] border border-[var(--ink-100)] bg-[var(--background-elevated)] p-4 shadow-[var(--shadow-1)]">
      <div
        role="radiogroup"
        aria-label="Variant image"
        className="flex flex-wrap gap-3"
      >
        {media.map((m) => {
          const selected = m.id === currentMediaId;
          return (
            <button
              key={m.id}
              type="button"
              role="radio"
              aria-checked={selected}
              aria-label={m.alt ?? "Image"}
              onClick={() => onSelect(m.id)}
              className={`relative h-20 w-20 overflow-hidden rounded-[6px] border transition-all focus:outline-none focus:ring-2 focus:ring-[var(--moss-700)] ${
                selected
                  ? "border-[var(--moss-700)] ring-2 ring-[var(--moss-700)]"
                  : "border-[var(--ink-100)]"
              }`}
            >
              {/* eslint-disable-next-line @next/next/no-img-element */}
              <img
                src={m.url}
                alt={m.alt ?? ""}
                className="h-full w-full object-cover"
                draggable={false}
              />
            </button>
          );
        })}
      </div>
      <button
        type="button"
        onClick={() => onSelect(null)}
        className="self-start text-xs uppercase tracking-widest text-[var(--ink-500)] hover:text-[var(--moss-700)] focus:text-[var(--moss-700)] focus:outline-none"
      >
        Remove image
      </button>
    </div>
  );
}
