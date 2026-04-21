"use client";

// apps/storefront/components/MediaGallery.tsx
//
// Product media gallery with a large main image and a horizontal
// thumbnail strip. Click a thumbnail to switch the main view.
// Falls back to a placeholder when the product has no media.
//
// Accessibility:
//   - auto-cycle honours `prefers-reduced-motion` (no rotation)
//   - visible Pause/Play toggle for touch + keyboard (WCAG 2.2.2)
//   - left/right arrow-key navigation across the listbox
//   - thumbnail strip is a listbox with a descriptive aria-label
//   - the image counter is announced via aria-live="polite" when it
//     changes, so screen readers follow the slideshow

import { useCallback, useState, type KeyboardEvent } from "react";
import Image from "next/image";
import { Pause, Play } from "lucide-react";
import type { StorefrontMedia } from "@/lib/api/marketplace-api";
import { useCarouselRotation } from "@/lib/useCarouselRotation";

interface MediaGalleryProps {
  media: StorefrontMedia[];
  productTitle: string;
}

const AUTO_CYCLE_MS = 4000;

export function MediaGallery({ media, productTitle }: MediaGalleryProps) {
  const [hovered, setHovered] = useState(false);
  const [focused, setFocused] = useState(false);
  const {
    index: activeIndex,
    setIndex,
    isPaused,
    togglePaused,
    next,
    prev,
    prefersReducedMotion,
  } = useCarouselRotation({
    count: media.length,
    intervalMs: AUTO_CYCLE_MS,
    paused: hovered || focused,
  });

  const handleThumbsKeyDown = useCallback(
    (e: KeyboardEvent<HTMLDivElement>) => {
      if (e.key === "ArrowRight") {
        e.preventDefault();
        next();
      } else if (e.key === "ArrowLeft") {
        e.preventDefault();
        prev();
      } else if (e.key === "Home") {
        e.preventDefault();
        setIndex(0);
      } else if (e.key === "End") {
        e.preventDefault();
        setIndex(media.length - 1);
      }
    },
    [next, prev, setIndex, media.length],
  );

  if (media.length === 0) {
    return (
      <div className="aspect-square rounded-md bg-[color:var(--storefront-background,var(--paper-200))] flex items-center justify-center">
        <span className="text-xs uppercase tracking-widest text-[color:var(--storefront-text,var(--ink-900))] opacity-30">
          No image
        </span>
      </div>
    );
  }

  const canCycle = media.length > 1 && !prefersReducedMotion;

  return (
    <div className="flex flex-col gap-3">
      {/* Main image — cross-fades between all images on an auto cycle */}
      <div
        className="relative aspect-square overflow-hidden rounded-md bg-[color:var(--storefront-background,var(--paper-200))]"
        onMouseEnter={() => setHovered(true)}
        onMouseLeave={() => setHovered(false)}
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
          <>
            <p
              className="absolute bottom-3 right-3 rounded-full bg-[color:var(--storefront-accent,var(--ink-900))]/60 px-2.5 py-1 text-xs text-[color:var(--storefront-on-accent,var(--paper-200))]"
              style={{ fontFeatureSettings: '"tnum" 1, "lnum" 1' }}
              aria-live="polite"
              aria-atomic="true"
            >
              <span className="sr-only">Image </span>
              {activeIndex + 1} / {media.length}
            </p>
            {canCycle ? (
              <button
                type="button"
                onClick={togglePaused}
                aria-pressed={isPaused}
                aria-label={isPaused ? "Play image slideshow" : "Pause image slideshow"}
                className="absolute bottom-3 left-3 inline-flex h-11 w-11 items-center justify-center rounded-full bg-[color:var(--storefront-accent,var(--ink-900))]/60 text-[color:var(--storefront-on-accent,var(--paper-200))] transition-opacity hover:opacity-90 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--storefront-accent,var(--moss-700))]"
              >
                {isPaused ? (
                  <Play className="size-4" aria-hidden="true" />
                ) : (
                  <Pause className="size-4" aria-hidden="true" />
                )}
              </button>
            ) : null}
          </>
        )}
      </div>

      {/* Thumbnail strip */}
      {media.length > 1 && (
        <div
          className="flex gap-2 overflow-x-auto pb-1"
          role="listbox"
          aria-label={`${productTitle} — select image (use arrow keys to navigate)`}
          tabIndex={0}
          onKeyDown={handleThumbsKeyDown}
          onFocus={() => setFocused(true)}
          onBlur={() => setFocused(false)}
        >
          {media.map((m, i) => (
            <button
              key={`${m.url}-${i}`}
              type="button"
              role="option"
              aria-selected={i === activeIndex}
              aria-label={`Image ${i + 1} of ${media.length}`}
              onClick={() => setIndex(i)}
              className={[
                // Only opacity animates here — ring-2 is a box-shadow
                // state change that snaps on selection (editorial
                // "decisive state" over mushy transitions).
                "relative h-16 w-16 shrink-0 overflow-hidden rounded-md transition-opacity duration-150",
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
