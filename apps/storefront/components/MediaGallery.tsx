"use client";

// apps/storefront/components/MediaGallery.tsx
//
// Product media gallery with a large main image and a horizontal
// thumbnail strip. Click a thumbnail to switch the main view.
// Falls back to a placeholder when the product has no media.

import { useState } from "react";
import type { StorefrontMedia } from "@/lib/api/marketplace-api";

interface MediaGalleryProps {
  media: StorefrontMedia[];
  productTitle: string;
}

export function MediaGallery({ media, productTitle }: MediaGalleryProps) {
  const [activeIndex, setActiveIndex] = useState(0);

  if (media.length === 0) {
    return (
      <div className="aspect-square rounded-md bg-[color:var(--paper-200)] flex items-center justify-center">
        <span className="text-xs uppercase tracking-widest text-[color:var(--ink-900)] opacity-30">
          No image
        </span>
      </div>
    );
  }

  const active = media[activeIndex] ?? media[0]!;

  return (
    <div className="flex flex-col gap-3">
      {/* Main image */}
      <div className="relative aspect-square overflow-hidden rounded-md bg-[color:var(--paper-200)]">
        {/* eslint-disable-next-line @next/next/no-img-element */}
        <img
          src={active.url}
          alt={active.alt ?? productTitle}
          className="h-full w-full object-cover"
        />
        {media.length > 1 && (
          <p className="absolute bottom-3 right-3 rounded-full bg-[color:var(--ink-900)]/60 px-2.5 py-1 text-xs text-[color:var(--paper-200)]">
            {activeIndex + 1} / {media.length}
          </p>
        )}
      </div>

      {/* Thumbnail strip */}
      {media.length > 1 && (
        <div
          className="flex gap-2 overflow-x-auto pb-1"
          role="listbox"
          aria-label="Product images"
        >
          {media.map((m, i) => (
            <button
              key={`${m.url}-${i}`}
              type="button"
              role="option"
              aria-selected={i === activeIndex}
              onClick={() => setActiveIndex(i)}
              className={[
                "relative h-16 w-16 shrink-0 overflow-hidden rounded-md transition-all",
                "focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)]",
                i === activeIndex
                  ? "ring-2 ring-[color:var(--ink-900)]"
                  : "opacity-60 hover:opacity-100",
              ].join(" ")}
            >
              {/* eslint-disable-next-line @next/next/no-img-element */}
              <img
                src={m.url}
                alt={m.alt ?? `${productTitle} ${i + 1}`}
                className="h-full w-full object-cover"
              />
            </button>
          ))}
        </div>
      )}
    </div>
  );
}
