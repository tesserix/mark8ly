// packages/ui/src/status-dot.tsx
//
// StatusDot renders an 8px circle in one of the three product-status
// variants, optionally followed by a label. Used on the products list,
// orders list, invitations list, and anywhere else a small at-a-glance
// state indicator fits.
//
// Palette (Paper · Ink · Moss):
//   active   — solid moss
//   draft    — outlined ink (no fill)
//   archived — muted solid ink (reduced opacity)

import type { ReactNode } from "react";

export type StatusDotVariant = "active" | "draft" | "archived";

export interface StatusDotProps {
  status: StatusDotVariant;
  withLabel?: boolean;
  className?: string;
  /** Override the rendered label text. Defaults to the capitalized status. */
  label?: ReactNode;
}

const VARIANT_CLASS: Record<StatusDotVariant, string> = {
  active: "bg-[color:var(--moss-700)]",
  draft: "border border-[color:var(--ink-900)] bg-transparent",
  archived: "bg-[color:var(--ink-900)] opacity-40",
};

const DEFAULT_LABEL: Record<StatusDotVariant, string> = {
  active: "Active",
  draft: "Draft",
  archived: "Archived",
};

export function StatusDot({
  status,
  withLabel = true,
  className = "",
  label,
}: StatusDotProps) {
  const dot = (
    <span
      role="presentation"
      aria-hidden="true"
      className={`inline-block h-2 w-2 rounded-full ${VARIANT_CLASS[status]}`}
    />
  );
  if (!withLabel) {
    return <span className={className}>{dot}</span>;
  }
  return (
    <span className={`inline-flex items-center gap-2 ${className}`}>
      {dot}
      <span className="text-[color:var(--ink-900)]">
        {label ?? DEFAULT_LABEL[status]}
      </span>
    </span>
  );
}
