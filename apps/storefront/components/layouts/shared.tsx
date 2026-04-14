import type { CSSProperties, ReactNode } from "react";

import type { PublicStore } from "@/lib/api/platform-api";
import type { StorefrontTheme } from "@repo/ui/storefront-theme";

/* ============================================================
   Shared layout primitives
   ------------------------------------------------------------
   All storefront layouts consume the same theme tokens and
   the same primitive set. A layout switch + a preset swap should
   produce a wildly different look without any layout touching
   brand colors directly.
   ============================================================ */

export interface LayoutProps {
  store: PublicStore;
  theme: StorefrontTheme;
}

/* ------------------------------------------------------------
   Structural primitives
   ------------------------------------------------------------ */

/** Reserved-space product slot. Honest placeholder — never fakes
 *  product photography. The aspect ratio matches what real merchant
 *  catalog photography will fill. */
export function ProductSlot({
  theme,
  label,
  ratio = "aspect-[4/5]",
  size = "md",
}: {
  theme: StorefrontTheme;
  label: string;
  ratio?: string;
  size?: "sm" | "md" | "lg";
}) {
  const tone = theme.preset === "midnight" ? "light" : "dark";
  const labelSize =
    size === "lg" ? "text-sm" : size === "sm" ? "text-[10px]" : "text-xs";
  return (
    <figure
      className={`group relative overflow-hidden ${ratio}`}
      style={{
        background: `${theme.colors.primary}08`,
        border: `1px solid ${theme.colors.primary}18`,
        borderRadius: "var(--storefront-radius)",
      }}
    >
      <div
        className="absolute inset-0 flex items-end p-4"
        style={{
          background: `linear-gradient(180deg, ${theme.colors.primary}00 55%, ${theme.colors.primary}12)`,
        }}
      />
      <figcaption
        className={`absolute left-4 top-4 font-semibold uppercase tracking-[0.18em] ${labelSize}`}
        style={{
          color: tone === "light" ? `${theme.colors.text}CC` : `${theme.colors.primary}99`,
        }}
      >
        {label}
      </figcaption>
      <div
        className="absolute bottom-4 left-4 right-4 text-[11px] uppercase tracking-[0.16em]"
        style={{ color: `${theme.colors.primary}55` }}
      >
        Reserved for product photography
      </div>
    </figure>
  );
}

/** Editorial pull quote. Large serif over paper, moss rule. */
export function PullQuote({
  theme,
  quote,
  attribution,
}: {
  theme: StorefrontTheme;
  quote: string;
  attribution?: string;
}) {
  return (
    <blockquote
      className="border-l-2 pl-6 py-2"
      style={{ borderColor: theme.colors.accent }}
    >
      <p
        className="text-2xl font-medium leading-[1.3] tracking-tight sm:text-3xl"
        style={{ ...headingStyle(), color: theme.colors.text }}
      >
        &ldquo;{quote}&rdquo;
      </p>
      {attribution && (
        <footer
          className="mt-3 text-[11px] font-semibold uppercase tracking-[0.2em]"
          style={{ color: `${theme.colors.primary}99` }}
        >
          — {attribution}
        </footer>
      )}
    </blockquote>
  );
}

/** Horizontal marquee strip — brand loop, uppercase. Pure CSS. */
export function Marquee({
  theme,
  items,
}: {
  theme: StorefrontTheme;
  items: string[];
}) {
  const line = items.join("   ·   ");
  return (
    <div
      className="overflow-hidden border-y py-4"
      style={{
        borderColor: `${theme.colors.primary}22`,
        background: `${theme.colors.surface}88`,
      }}
    >
      <div
        className="whitespace-nowrap text-xs font-semibold uppercase tracking-[0.28em]"
        style={{ color: `${theme.colors.primary}CC` }}
      >
        {line}   ·   {line}   ·   {line}
      </div>
    </div>
  );
}

/** Section divider with editorial label. */
export function SectionLabel({
  theme,
  label,
  title,
  trailing,
}: {
  theme: StorefrontTheme;
  label: string;
  title?: string;
  trailing?: ReactNode;
}) {
  return (
    <div
      className="flex items-end justify-between gap-4 border-b pb-4"
      style={{ borderColor: `${theme.colors.primary}22` }}
    >
      <div>
        <p
          className="text-[11px] font-semibold uppercase tracking-[0.24em]"
          style={{ color: `${theme.colors.accent}D9` }}
        >
          {label}
        </p>
        {title && (
          <h2
            className="mt-1 text-2xl font-medium tracking-tight sm:text-3xl"
            style={headingStyle()}
          >
            {title}
          </h2>
        )}
      </div>
      {trailing}
    </div>
  );
}

/* ------------------------------------------------------------
   Typographic primitives
   ------------------------------------------------------------ */

export function HeroTitle({
  store,
  theme,
  align = "left",
  size = "lg",
}: {
  store: PublicStore;
  theme: StorefrontTheme;
  align?: "left" | "center";
  size?: "md" | "lg" | "xl";
}) {
  const sizeClass =
    size === "xl"
      ? "text-6xl sm:text-7xl lg:text-[7.5rem]"
      : size === "md"
        ? "text-4xl sm:text-5xl"
        : "text-5xl sm:text-6xl lg:text-7xl";
  return (
    <div className={align === "center" ? "text-center" : ""}>
      <h1
        className={`${sizeClass} font-medium leading-[0.95] tracking-[-0.03em]`}
        style={headingStyle()}
      >
        {store.name}
      </h1>
    </div>
  );
}

export function Eyebrow({
  theme,
  children,
}: {
  theme: StorefrontTheme;
  children: ReactNode;
}) {
  return (
    <p
      className="text-[11px] font-semibold uppercase tracking-[0.24em]"
      style={{ color: `${theme.colors.accent}D9` }}
    >
      {children}
    </p>
  );
}

/* ------------------------------------------------------------
   Buttons
   ------------------------------------------------------------ */

export function PrimaryButton({
  theme,
  children,
  size = "md",
}: {
  theme: StorefrontTheme;
  children: ReactNode;
  size?: "md" | "lg";
}) {
  const h = size === "lg" ? "h-14 px-8 text-base" : "h-12 px-6 text-sm";
  return (
    <button
      type="button"
      className={`inline-flex ${h} items-center justify-center font-semibold uppercase tracking-[0.14em] text-white transition-opacity hover:opacity-90`}
      style={{
        background: theme.colors.primary,
        borderRadius: "var(--storefront-radius)",
      }}
    >
      {children}
    </button>
  );
}

export function SecondaryButton({
  theme,
  children,
}: {
  theme: StorefrontTheme;
  children: ReactNode;
}) {
  return (
    <button
      type="button"
      className="inline-flex h-12 items-center justify-center border px-6 text-sm font-semibold uppercase tracking-[0.14em] transition-colors"
      style={{
        borderColor: `${theme.colors.primary}44`,
        background: "transparent",
        color: theme.colors.text,
        borderRadius: "var(--storefront-radius)",
      }}
    >
      {children}
    </button>
  );
}

export function LinkCta({
  theme,
  children,
}: {
  theme: StorefrontTheme;
  children: ReactNode;
}) {
  return (
    <span
      className="inline-flex items-center gap-2 border-b pb-1 text-sm font-semibold uppercase tracking-[0.14em]"
      style={{
        color: theme.colors.accent,
        borderColor: theme.colors.accent,
      }}
    >
      {children}
      <span aria-hidden="true">→</span>
    </span>
  );
}

/* ------------------------------------------------------------
   Style helpers
   ------------------------------------------------------------ */

export function headingStyle(): CSSProperties {
  return { fontFamily: "var(--storefront-heading-font)" };
}

export function surfaceStyle(theme: StorefrontTheme): CSSProperties {
  return {
    background: theme.colors.surface,
    border: `1px solid ${theme.colors.primary}22`,
    borderRadius: "var(--storefront-radius)",
  };
}
