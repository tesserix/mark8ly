// packages/ui/src/price-display.tsx
//
// PriceDisplay formats a money amount in Source Serif 4 tabular figures
// with locale-aware currency formatting. Admin list, storefront cards,
// order lines, and invoices all use this component so the rendering is
// consistent across surfaces.
//
// Takes amount as a string (the raw decimal shape the API returns, e.g.
// "19.99" or "89.00") to avoid JS float loss. For display-only use cases
// Intl.NumberFormat + parseFloat is fine — we don't do arithmetic here.
//
// For variant-priced products, callers render a "from" prefix outside
// this component (e.g. `<span>from <PriceDisplay ... /></span>`).

import type { CSSProperties } from "react";

export interface PriceDisplayProps {
  amount: string;
  currencyCode: string;
  /** BCP 47 locale. Omit to use the browser's default. */
  locale?: string;
  className?: string;
  /** Render as a `<span>` (default) or `<div>` / other block element. */
  as?: "span" | "div";
}

const TABULAR_STYLE: CSSProperties = {
  fontFeatureSettings: '"tnum" 1, "lnum" 1',
};

export function PriceDisplay({
  amount,
  currencyCode,
  locale,
  className = "",
  as = "span",
}: PriceDisplayProps) {
  const numeric = Number.parseFloat(amount);
  const formatted = Number.isFinite(numeric)
    ? new Intl.NumberFormat(locale, {
        style: "currency",
        currency: currencyCode,
      }).format(numeric)
    : `${currencyCode} ${amount}`;
  const Tag = as;
  return (
    <Tag
      className={`font-[family-name:var(--font-serif,'Source_Serif_4',serif)] ${className}`}
      style={TABULAR_STYLE}
    >
      {formatted}
    </Tag>
  );
}
