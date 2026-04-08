import type { ReactNode } from "react";

/**
 * RoleBadge — editorial pill for tenant/store roles.
 *
 * Single primitive consumed by admin (top bar, Team page, PickTenant,
 * AcceptInvite) and onboarding (welcome page) so every place the word
 * "owner" / "admin" / "staff" / "viewer" appears reads the same and
 * carries the same tier weight.
 *
 * Visual tiers:
 *   - owner   → moss-filled, strongest visual weight. The brand accent.
 *   - admin   → ink-filled, very strong, authoritative.
 *   - manager → ink outline, strong but not ink-filled (store-scoped).
 *   - staff   → paper-100 fill, hairline border. Quiet but present.
 *   - viewer  → paper-100 fill, foreground-tertiary text. Quietest.
 *
 * Size:
 *   - sm → for inline row usage (Team list, sidebar)
 *   - md → for top bar, header contexts
 *
 * Unknown roles (e.g. new DSL additions, custom strings) fall back to
 * the "staff" treatment so nothing ever renders without a pill.
 */
export type RoleBadgeRole =
  | "owner"
  | "admin"
  | "manager"
  | "staff"
  | "viewer"
  | (string & {});

export interface RoleBadgeProps {
  role: RoleBadgeRole;
  size?: "sm" | "md";
  /** Optional label override; defaults to the role string */
  children?: ReactNode;
  className?: string;
}

export function RoleBadge({
  role,
  size = "sm",
  children,
  className = "",
}: RoleBadgeProps) {
  const variant = variantFor(role);
  const dimensions =
    size === "md"
      ? "h-7 px-3 text-[11px]"
      : "h-[22px] px-2.5 text-[10px]";

  return (
    <span
      className={`inline-flex ${dimensions} items-center justify-center whitespace-nowrap rounded-sm border font-semibold uppercase tracking-[0.14em] ${variant} ${className}`}
    >
      {children ?? role}
    </span>
  );
}

function variantFor(role: string): string {
  switch (role.toLowerCase()) {
    case "owner":
      return "bg-moss-700 border-moss-700 text-paper-50";
    case "admin":
      return "bg-ink-900 border-ink-900 text-paper-50";
    case "manager":
      return "bg-background-elevated border-ink-900 text-foreground";
    case "staff":
      return "bg-paper-100 border-border-strong text-foreground";
    case "viewer":
      return "bg-paper-100 border-border text-foreground-tertiary";
    default:
      return "bg-paper-100 border-border text-foreground-tertiary";
  }
}
