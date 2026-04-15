/**
 * Per-layout style mappings for the editorial block types
 * (marquee, pull_quote, letter). Keyed off `theme.layout` so each
 * of the 8 storefront layouts gets its own visual treatment for
 * the merchant-authored editorial blocks without forking the
 * renderer components.
 *
 * All class strings use Tailwind utilities + the storefront token
 * set — no hard-coded colors. Layouts that omit a case fall back
 * to the `default` treatment, which is a conservative Minimal-ish
 * baseline.
 */

import type { StorefrontLayout } from "@repo/ui/storefront-theme";

export interface MarqueeStyles {
  section: string;
  track: string;
  item: string;
}

export interface PullQuoteStyles {
  container: string;
  text: string;
  attribution: string;
}

export interface LetterStyles {
  container: string;
  eyebrow: string;
  title: string;
  body: string;
  cta: string;
}

export function marqueeStylesFor(layout: StorefrontLayout): MarqueeStyles {
  switch (layout) {
    case "editorial":
    case "story-led":
      return {
        section: "overflow-hidden border-y border-foreground/10 bg-background/60 py-5",
        track: "flex animate-marquee gap-12 whitespace-nowrap text-[11px] font-semibold uppercase tracking-[0.28em] text-foreground/80",
        item: "shrink-0",
      };
    case "bold-promo":
      return {
        section: "overflow-hidden border-y-2 border-foreground bg-foreground py-3",
        track: "flex animate-marquee gap-10 whitespace-nowrap text-sm font-bold uppercase tracking-[0.2em] text-background",
        item: "shrink-0",
      };
    case "classic-shop":
    case "catalog-first":
      return {
        section: "overflow-hidden border-y border-foreground/15 py-4",
        track: "flex animate-marquee gap-8 whitespace-nowrap text-xs font-semibold uppercase tracking-[0.22em] text-foreground-secondary",
        item: "shrink-0",
      };
    case "split-hero":
      return {
        section: "overflow-hidden bg-foreground/5 py-4",
        track: "flex animate-marquee gap-12 whitespace-nowrap text-[11px] font-semibold uppercase tracking-[0.3em] text-foreground/70",
        item: "shrink-0",
      };
    case "minimal":
    case "compact":
    default:
      return {
        section: "overflow-hidden border-y border-foreground/10 py-3",
        track: "flex animate-marquee gap-8 whitespace-nowrap text-xs uppercase tracking-[0.18em] text-foreground-secondary",
        item: "shrink-0",
      };
  }
}

export function pullQuoteStylesFor(layout: StorefrontLayout): PullQuoteStyles {
  switch (layout) {
    case "editorial":
      return {
        container: "mx-auto max-w-3xl border-l-2 border-moss-700 px-6 py-4 sm:px-8",
        text: "font-serif text-3xl font-medium leading-[1.25] tracking-tight text-foreground sm:text-4xl",
        attribution: "mt-4 text-[11px] font-semibold uppercase tracking-[0.22em] text-foreground-secondary",
      };
    case "story-led":
      return {
        container: "mx-auto max-w-2xl text-center px-6",
        text: "font-serif text-2xl italic leading-snug text-foreground sm:text-3xl",
        attribution: "mt-5 text-xs uppercase tracking-[0.2em] text-foreground-secondary",
      };
    case "bold-promo":
      return {
        container: "mx-auto max-w-4xl px-6",
        text: "font-serif text-4xl font-semibold leading-[1.1] tracking-tight text-foreground sm:text-5xl",
        attribution: "mt-5 text-sm font-semibold uppercase tracking-[0.18em] text-accent",
      };
    case "split-hero":
      return {
        container: "mx-auto max-w-3xl border-l border-foreground/20 pl-6",
        text: "font-serif text-2xl font-medium leading-[1.25] text-foreground sm:text-3xl",
        attribution: "mt-3 text-[11px] uppercase tracking-[0.22em] text-foreground-secondary",
      };
    case "classic-shop":
    case "catalog-first":
      return {
        container: "mx-auto max-w-3xl px-6 text-center",
        text: "font-serif text-2xl italic leading-snug text-foreground",
        attribution: "mt-4 text-sm uppercase tracking-wide text-foreground-secondary",
      };
    case "minimal":
    case "compact":
    default:
      return {
        container: "mx-auto max-w-2xl px-6 text-center",
        text: "font-serif text-xl italic leading-snug text-foreground sm:text-2xl",
        attribution: "mt-3 text-xs uppercase tracking-[0.18em] text-foreground-secondary",
      };
  }
}

export function letterStylesFor(layout: StorefrontLayout): LetterStyles {
  switch (layout) {
    case "editorial":
      return {
        container: "mx-auto max-w-2xl px-6",
        eyebrow: "text-[11px] font-semibold uppercase tracking-[0.24em] text-accent",
        title: "mt-3 font-serif text-4xl font-medium leading-[1.1] tracking-tight text-foreground sm:text-5xl",
        body: "mt-6 space-y-4 font-serif text-lg leading-relaxed text-foreground/85",
        cta: "mt-8 inline-flex items-center gap-2 border-b border-accent pb-1 text-sm font-semibold uppercase tracking-[0.14em] text-accent transition-opacity hover:opacity-80",
      };
    case "story-led":
      return {
        container: "mx-auto max-w-2xl px-6 text-center",
        eyebrow: "text-[11px] font-semibold uppercase tracking-[0.26em] text-foreground-secondary",
        title: "mt-4 font-serif text-3xl font-medium leading-tight text-foreground sm:text-4xl",
        body: "mt-6 space-y-4 text-base leading-relaxed text-foreground/80",
        cta: "mt-8 inline-flex h-12 items-center justify-center border border-foreground px-6 text-xs font-semibold uppercase tracking-[0.18em] text-foreground transition-colors hover:bg-foreground hover:text-background",
      };
    case "bold-promo":
      return {
        container: "mx-auto max-w-3xl px-6",
        eyebrow: "text-xs font-bold uppercase tracking-[0.22em] text-accent",
        title: "mt-3 font-serif text-5xl font-semibold leading-[1.02] tracking-tight text-foreground sm:text-6xl",
        body: "mt-6 space-y-4 text-lg leading-relaxed text-foreground/85",
        cta: "mt-8 inline-flex h-14 items-center justify-center bg-foreground px-8 text-sm font-bold uppercase tracking-[0.16em] text-background transition-opacity hover:opacity-90",
      };
    case "split-hero":
      return {
        container: "mx-auto max-w-5xl grid grid-cols-1 gap-10 px-6 sm:grid-cols-5",
        eyebrow: "sm:col-span-2 text-[11px] font-semibold uppercase tracking-[0.24em] text-accent",
        title: "sm:col-span-5 font-serif text-3xl font-medium leading-tight text-foreground sm:text-4xl",
        body: "sm:col-start-3 sm:col-span-3 space-y-4 text-base leading-relaxed text-foreground/80",
        cta: "sm:col-start-3 sm:col-span-3 mt-2 inline-flex items-center gap-2 border-b border-accent pb-1 text-sm font-semibold uppercase tracking-[0.14em] text-accent w-fit",
      };
    case "classic-shop":
    case "catalog-first":
      return {
        container: "mx-auto max-w-3xl px-6",
        eyebrow: "text-xs font-semibold uppercase tracking-[0.2em] text-foreground-secondary",
        title: "mt-3 font-serif text-3xl font-medium leading-tight text-foreground",
        body: "mt-5 space-y-4 text-base leading-relaxed text-foreground/85",
        cta: "mt-6 inline-flex items-center gap-2 text-sm font-semibold text-accent hover:underline",
      };
    case "minimal":
    case "compact":
    default:
      return {
        container: "mx-auto max-w-xl px-6",
        eyebrow: "text-[11px] uppercase tracking-[0.2em] text-foreground-secondary",
        title: "mt-2 font-serif text-2xl font-medium leading-tight text-foreground sm:text-3xl",
        body: "mt-4 space-y-3 text-sm leading-relaxed text-foreground/80",
        cta: "mt-5 inline-flex items-center gap-2 text-sm font-medium text-accent hover:underline",
      };
  }
}
