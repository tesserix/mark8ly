// apps/storefront/components/ui/StatusChip.tsx
//
// Single shell-level primitive for status visualization across the
// storefront. Each tone has one source colour (configurable by the
// merchant in admin theme settings) — the bg/border variants are
// derived via CSS color-mix in globals.css, so every chip stays
// legible on whatever surface and theme it lands on.
//
// Used for: order status, ticket status, return status, stock
// availability, gift-card state, loyalty tier signals, etc.

import type { HTMLAttributes, ReactNode } from "react";

export type StatusTone = "success" | "warning" | "danger" | "info" | "neutral";
export type StatusVariant = "soft" | "outline" | "dot";
export type StatusSize = "sm" | "md";

export interface StatusChipProps
  extends Omit<HTMLAttributes<HTMLSpanElement>, "children"> {
  tone?: StatusTone;
  variant?: StatusVariant;
  size?: StatusSize;
  /** Show a coloured dot on the leading edge (redundant signal for colour-blind users). */
  withDot?: boolean;
  children: ReactNode;
}

// Both fg AND bg pinned to fixed hex values. Previously fg was
// `var(--storefront-success/warning/info)` so the merchant could re-
// theme it — but on a dark merchant theme they override those source
// colours to LIGHT shades for visibility, which then rendered light
// fg on the light paper-tinted bg my earlier fix introduced. Result:
// "Confirmed" and "Pending" pills became unreadable smudges.
// Hex values are intentionally darker than the AA contrast threshold
// against the 22% paper bgs in globals.css.
const TONE_VARS: Record<StatusTone, { fg: string; bg: string; border: string }> = {
  success: {
    fg: "#0E3F22",
    bg: "var(--storefront-success-bg)",
    border: "var(--storefront-success-border)",
  },
  warning: {
    fg: "#5A3D00",
    bg: "var(--storefront-warning-bg)",
    border: "var(--storefront-warning-border)",
  },
  danger: {
    fg: "#7A2615",
    bg: "var(--storefront-danger-bg)",
    border: "var(--storefront-danger-border)",
  },
  info: {
    fg: "#1F356E",
    bg: "var(--storefront-info-bg)",
    border: "var(--storefront-info-border)",
  },
  neutral: {
    fg: "#0E0E0C",
    bg: "var(--storefront-neutral-bg)",
    border: "var(--storefront-neutral-border)",
  },
};

const SIZE_CLASSES: Record<StatusSize, string> = {
  sm: "px-2 py-0.5 text-[10px]",
  md: "px-2.5 py-1 text-xs",
};

export function StatusChip({
  tone = "neutral",
  variant = "soft",
  size = "sm",
  withDot = false,
  className = "",
  children,
  style,
  ...rest
}: StatusChipProps) {
  const vars = TONE_VARS[tone];
  const baseStyle =
    variant === "outline"
      ? { color: vars.fg, borderColor: vars.border, backgroundColor: "transparent" }
      : variant === "dot"
        ? { color: vars.fg, backgroundColor: "transparent", borderColor: "transparent" }
        : { color: vars.fg, backgroundColor: vars.bg, borderColor: vars.border };

  return (
    <span
      {...rest}
      className={`inline-flex items-center gap-1.5 border font-medium ${SIZE_CLASSES[size]} ${className}`}
      style={{
        borderRadius: "var(--storefront-radius)",
        ...baseStyle,
        ...style,
      }}
    >
      {withDot ? (
        <span
          aria-hidden
          className="inline-block h-1.5 w-1.5 shrink-0 rounded-full"
          style={{ backgroundColor: vars.fg }}
        />
      ) : null}
      {children}
    </span>
  );
}
