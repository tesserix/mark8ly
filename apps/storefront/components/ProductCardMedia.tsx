"use client";

// Product card image — auto-cycles through all product media
// every 3s with a soft cross-fade. Falls back to a "No image" tile.
//
// Accessibility: the rotation honours `prefers-reduced-motion` (no
// cycling), pauses on pointer + focus, and exposes each indicator as
// a real button so keyboard and touch users can navigate directly
// (WCAG 2.2.2). All timers share a module-level ticker so a grid of
// 8 product cards runs one interval, not eight.

import Image from "next/image";
import { memo, useState } from "react";
import type { StorefrontMedia } from "@/lib/api/marketplace-api";
import { useCarouselRotation } from "@/lib/useCarouselRotation";

interface Props {
  media: StorefrontMedia[];
  alt: string;
}

const INTERVAL_MS = 3000;

function ProductCardMediaImpl({ media, alt }: Props) {
  const [hovered, setHovered] = useState(false);
  const [focused, setFocused] = useState(false);
  const { index, setIndex, prefersReducedMotion } = useCarouselRotation({
    count: media.length,
    intervalMs: INTERVAL_MS,
    paused: hovered || focused,
  });

  if (media.length === 0) {
    return (
      <div className="relative aspect-square overflow-hidden rounded-md bg-[color:var(--storefront-background,var(--paper-200))]">
        <div className="flex h-full w-full items-center justify-center text-xs uppercase tracking-widest text-[color:var(--storefront-text,var(--ink-900))] opacity-30">
          No image
        </div>
      </div>
    );
  }

  return (
    <div
      className="relative aspect-square overflow-hidden rounded-md bg-[color:var(--storefront-background,var(--paper-200))]"
      onMouseEnter={() => setHovered(true)}
      onMouseLeave={() => setHovered(false)}
      onFocus={() => setFocused(true)}
      onBlur={() => setFocused(false)}
    >
      {media.map((m, i) => (
        <Image
          key={`${m.url}-${i}`}
          src={m.url}
          alt={m.alt ?? alt}
          fill
          sizes="(min-width: 1024px) 33vw, (min-width: 640px) 50vw, 100vw"
          priority={i === 0}
          className={[
            "object-cover transition-opacity duration-700 ease-in-out",
            i === index ? "opacity-100" : "opacity-0",
          ].join(" ")}
        />
      ))}

      {media.length > 1 && (
        <div
          className="absolute bottom-2 left-1/2 flex -translate-x-1/2 gap-1.5"
          role="group"
          aria-label={prefersReducedMotion ? "Product images" : "Product images — select to view"}
        >
          {media.map((_, i) => (
            <button
              key={i}
              type="button"
              aria-label={`Show image ${i + 1} of ${media.length}`}
              aria-current={i === index ? "true" : undefined}
              onClick={(e) => {
                // Don't trigger the parent product link.
                e.preventDefault();
                e.stopPropagation();
                setIndex(i);
              }}
              className={[
                // Active/inactive width snaps rather than animating —
                // width is a layout property, and a grid of 12 cards ×
                // ~5 dots × 4fps ticker would otherwise force 240+
                // layout recalcs per second during cycle. The bg
                // opacity still transitions for hover feedback.
                "h-1.5 rounded-full transition-colors duration-200",
                "focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--storefront-accent,var(--moss-700))]",
                i === index
                  ? "w-5 bg-[color:var(--storefront-background,var(--paper-200))]"
                  : "w-1.5 bg-[color:var(--storefront-background,var(--paper-200))]/60 hover:bg-[color:var(--storefront-background,var(--paper-200))]/90",
              ].join(" ")}
            />
          ))}
        </div>
      )}
    </div>
  );
}

// Memoize so that parent re-renders (from the shared ticker at 250ms
// cadence in sibling cards) don't force every card in the grid to
// reconcile.
export const ProductCardMedia = memo(ProductCardMediaImpl);
