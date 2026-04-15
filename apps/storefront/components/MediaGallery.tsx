"use client";

// apps/storefront/components/MediaGallery.tsx
//
// Product media gallery with a large main image and a horizontal
// thumbnail strip. Click a thumbnail to switch the main view.
// Falls back to a placeholder when the product has no media.

import { useEffect, useRef, useState } from "react";
import Image from "next/image";
import type { StorefrontMedia } from "@/lib/api/marketplace-api";

interface MediaGalleryProps {
  media: StorefrontMedia[];
  productTitle: string;
}

const AUTO_CYCLE_MS = 4000;

export function MediaGallery({ media, productTitle }: MediaGalleryProps) {
  const [activeIndex, setActiveIndex] = useState(0);
  const pausedRef = useRef(false);

  useEffect(() => {
    if (media.length <= 1) return;
    const id = window.setInterval(() => {
      if (pausedRef.current) return;
      setActiveIndex((i) => (i + 1) % media.length);
    }, AUTO_CYCLE_MS);
    return () => window.clearInterval(id);
  }, [media.length]);

  if (media.length === 0) {
    return (
      <div className="aspect-square rounded-md bg-[color:var(--storefront-background,var(--paper-200))] flex items-center justify-center">
        <span className="text-xs uppercase tracking-widest text-[color:var(--storefront-text,var(--ink-900))] opacity-30">
          No image
        </span>
      </div>
    );
  }

  return (
    <div className="flex flex-col gap-3">
      {/* Main image — cross-fades between all images on an auto cycle */}
      <div
        className="relative aspect-square overflow-hidden rounded-md bg-[color:var(--storefront-background,var(--paper-200))]"
        onMouseEnter={() => (pausedRef.current = true)}
        onMouseLeave={() => (pausedRef.current = false)}
      >
        {media.map((m, i) => (
          <Image
            key={`${m.url}-${i}`}
            src={m.url}
            alt={m.alt ?? productTitle}
            fill
            sizes="(min-width: 1024px) 50vw, 100vw"
            priority={i === 0}
            className={[
              "object-cover transition-opacity duration-700 ease-in-out",
              i === activeIndex ? "opacity-100" : "opacity-0",
            ].join(" ")}
          />
        ))}
        {media.length > 1 && (
          <p
            className="absolute bottom-3 right-3 rounded-full bg-[color:var(--storefront-accent,var(--ink-900))]/60 px-2.5 py-1 text-xs text-[color:var(--storefront-on-accent,var(--paper-200))]"
            style={{ fontFeatureSettings: '"tnum" 1, "lnum" 1' }}
          >
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
                "relative h-16 w-16 shrink-0 overflow-hidden rounded-md transition-all duration-150",
                "focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--storefront-accent,var(--moss-700))]",
                i === activeIndex
                  ? "ring-2 ring-[color:var(--storefront-text,var(--ink-900))]"
                  : "opacity-50 hover:opacity-100",
              ].join(" ")}
            >
              <Image
                src={m.url}
                alt={m.alt ?? `${productTitle} — thumbnail ${i + 1}`}
                fill
                sizes="64px"
                className="object-cover"
              />
            </button>
          ))}
        </div>
      )}
    </div>
  );
}
