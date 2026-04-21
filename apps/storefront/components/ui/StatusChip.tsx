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

const TONE_VARS: Record<StatusTone, { fg: string; bg: string; border: string }> = {
  success: {
    fg: "var(--storefront-success)",
    bg: "var(--storefront-success-bg)",
    border: "var(--storefront-success-border)",
  },
  warning: {
    fg: "var(--storefront-warning)",
    bg: "var(--storefront-warning-bg)",
    border: "var(--storefront-warning-border)",
  },
  danger: {
    fg: "var(--storefront-danger)",
    bg: "var(--storefront-danger-bg)",
    border: "var(--storefront-danger-border)",
  },
  info: {
    fg: "var(--storefront-info)",
    bg: "var(--storefront-info-bg)",
    border: "var(--storefront-info-border)",
  },
  neutral: {
    fg: "var(--storefront-text)",
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
      className={`inline-flex items-center gap-1.5 rounded-full border font-medium ${SIZE_CLASSES[size]} ${className}`}
      style={{ ...baseStyle, ...style }}
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
