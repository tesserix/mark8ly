// components/orders/OrderStatusBadges.tsx
//
// Three small at-a-glance status indicators for the orders surface.
// Mirrors the StatusDot visual primitive from @repo/ui — same 8px circle
// + label pattern — but with the orders-specific Paper · Ink · Moss
// palette. Status semantics are different from product status (orders
// have three orthogonal axes), so these live local to the admin app
// rather than extending the shared design system.
//
// Palette:
//   pending / unfulfilled / authorized → outlined ink (in progress)
//   confirmed / paid / fulfilled       → solid moss (good state)
//   partially_refunded / partial       → outlined moss (in-between)
//   cancelled / refunded               → muted solid ink (terminal/closed)
//   failed                             → solid signal (vermillion, attention)

import type {
  FulfillmentStatus,
  OrderStatus,
  PaymentStatus,
} from "@/lib/api/marketplace-api";

type Variant = "in-progress" | "good" | "between" | "closed" | "warn";

const VARIANT_CLASS: Record<Variant, string> = {
  "in-progress":
    "border border-[color:var(--ink-900)] border-opacity-60 bg-transparent",
  good: "bg-[color:var(--moss-700)]",
  between:
    "border border-[color:var(--moss-700)] bg-transparent",
  closed: "bg-[color:var(--ink-900)] opacity-40",
  warn: "bg-[color:var(--signal)]",
};

interface BadgeProps {
  variant: Variant;
  label: string;
  className?: string;
}

function Badge({ variant, label, className = "" }: BadgeProps) {
  return (
    <span className={`inline-flex items-center gap-2 ${className}`}>
      <span
        role="presentation"
        aria-hidden="true"
        className={`inline-block h-2 w-2 rounded-full ${VARIANT_CLASS[variant]}`}
      />
      <span className="text-[color:var(--ink-900)]">{label}</span>
    </span>
  );
}

// ─────────────────────────────────────────────────────────────────────────
// OrderStatusBadge — orders.status (operational lifecycle)
// ─────────────────────────────────────────────────────────────────────────

const ORDER_STATUS_MAP: Record<
  OrderStatus,
  { variant: Variant; label: string }
> = {
  pending: { variant: "in-progress", label: "Pending" },
  confirmed: { variant: "good", label: "Confirmed" },
  fulfilled: { variant: "good", label: "Fulfilled" },
  cancelled: { variant: "closed", label: "Cancelled" },
};

interface OrderStatusBadgeProps {
  status: OrderStatus;
  className?: string;
}

export function OrderStatusBadge({ status, className }: OrderStatusBadgeProps) {
  const { variant, label } = ORDER_STATUS_MAP[status];
  return <Badge variant={variant} label={label} className={className} />;
}

// ─────────────────────────────────────────────────────────────────────────
// PaymentStatusBadge — orders.payment_status (money state)
// ─────────────────────────────────────────────────────────────────────────

const PAYMENT_STATUS_MAP: Record<
  PaymentStatus,
  { variant: Variant; label: string }
> = {
  pending: { variant: "in-progress", label: "Pending" },
  authorized: { variant: "in-progress", label: "Authorized" },
  paid: { variant: "good", label: "Paid" },
  failed: { variant: "warn", label: "Failed" },
  refunded: { variant: "closed", label: "Refunded" },
  partially_refunded: { variant: "between", label: "Partially refunded" },
};

interface PaymentStatusBadgeProps {
  status: PaymentStatus;
  className?: string;
}

export function PaymentStatusBadge({
  status,
  className,
}: PaymentStatusBadgeProps) {
  const { variant, label } = PAYMENT_STATUS_MAP[status];
  return <Badge variant={variant} label={label} className={className} />;
}

// ─────────────────────────────────────────────────────────────────────────
// FulfillmentStatusBadge — orders.fulfillment_status (shipping state)
// ─────────────────────────────────────────────────────────────────────────

const FULFILLMENT_STATUS_MAP: Record<
  FulfillmentStatus,
  { variant: Variant; label: string }
> = {
  unfulfilled: { variant: "in-progress", label: "Unfulfilled" },
  partial: { variant: "between", label: "Partial" },
  fulfilled: { variant: "good", label: "Fulfilled" },
};

interface FulfillmentStatusBadgeProps {
  status: FulfillmentStatus;
  className?: string;
}

export function FulfillmentStatusBadge({
  status,
  className,
}: FulfillmentStatusBadgeProps) {
  const { variant, label } = FULFILLMENT_STATUS_MAP[status];
  return <Badge variant={variant} label={label} className={className} />;
}
