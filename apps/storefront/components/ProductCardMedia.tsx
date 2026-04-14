"use client";

// Product card image — auto-cycles through all product media
// every 3s with a soft cross-fade. Pauses on hover so the shopper
// can inspect a specific shot. Falls back to a "No image" tile.

import Image from "next/image";
import { useEffect, useRef, useState } from "react";
import type { StorefrontMedia } from "@/lib/api/marketplace-api";

interface Props {
  media: StorefrontMedia[];
  alt: string;
}

const INTERVAL_MS = 3000;

export function ProductCardMedia({ media, alt }: Props) {
  const [index, setIndex] = useState(0);
  const pausedRef = useRef(false);

  useEffect(() => {
    if (media.length <= 1) return;
    const id = window.setInterval(() => {
      if (pausedRef.current) return;
      setIndex((i) => (i + 1) % media.length);
    }, INTERVAL_MS);
    return () => window.clearInterval(id);
  }, [media.length]);

  if (media.length === 0) {
    return (
      <div className="relative aspect-square overflow-hidden rounded-md bg-[color:var(--paper-200)]">
        <div className="flex h-full w-full items-center justify-center text-xs uppercase tracking-widest text-[color:var(--ink-900)] opacity-30">
          No image
        </div>
      </div>
    );
  }

  return (
    <div
      className="relative aspect-square overflow-hidden rounded-md bg-[color:var(--paper-200)]"
      onMouseEnter={() => (pausedRef.current = true)}
      onMouseLeave={() => (pausedRef.current = false)}
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
        <div className="pointer-events-none absolute bottom-2 left-1/2 flex -translate-x-1/2 gap-1.5">
          {media.map((_, i) => (
            <span
              key={i}
              className={[
                "h-1.5 rounded-full transition-all duration-300",
                i === index
                  ? "w-5 bg-[color:var(--paper-200)]"
                  : "w-1.5 bg-[color:var(--paper-200)]/60",
              ].join(" ")}
            />
          ))}
        </div>
      )}
    </div>
  );
}
