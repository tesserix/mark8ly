// components/orders/OrdersList.tsx
//
// Editorial-style orders table. Hairline-rule rows (no boxed cards),
// left-aligned, monetary totals right-aligned. Each row links to the
// detail page (`/orders/[id]`). The detail route is registered in
// Phase 2 — for now the link 404s on click, which is acceptable while
// the list is the only Phase 1 deliverable.

import Link from "next/link";

import type { AdminOrder } from "@/lib/api/marketplace-api";

import {
  FulfillmentStatusBadge,
  OrderStatusBadge,
  PaymentStatusBadge,
} from "./OrderStatusBadges";

interface OrdersListProps {
  orders: AdminOrder[];
}

export function OrdersList({ orders }: OrdersListProps) {
  return (
    <div className="flex flex-col">
      {/* Column header rule */}
      <div
        role="presentation"
        className="grid grid-cols-[minmax(0,1.4fr)_minmax(0,1.6fr)_minmax(0,1.2fr)_minmax(0,1.2fr)_minmax(0,1fr)_minmax(0,1fr)] items-end gap-6 border-b border-[color:var(--ink-900)] border-opacity-15 pb-3 text-xs uppercase tracking-wider text-[color:var(--ink-900)] opacity-60"
      >
        <span>Order</span>
        <span>Customer</span>
        <span title="Operational lifecycle — pending, confirmed, fulfilled, or cancelled">Status</span>
        <span title="Money state — pending, authorized, paid, failed, or refunded">Payment</span>
        <span title="Shipping state — unfulfilled, partial, or fulfilled">Fulfillment</span>
        <span className="text-right">Total</span>
      </div>

      <ul role="list" className="flex flex-col">
        {orders.map((o) => (
          <li
            key={o.id}
            className="border-b border-[color:var(--ink-900)] border-opacity-10"
          >
            <Link
              href={`/orders/${o.id}`}
              className="grid grid-cols-[minmax(0,1.4fr)_minmax(0,1.6fr)_minmax(0,1.2fr)_minmax(0,1.2fr)_minmax(0,1fr)_minmax(0,1fr)] items-center gap-6 py-4 transition-colors hover:bg-[color:var(--ink-900)]/[0.03] focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)]"
              aria-label={`Order ${o.order_number} for ${o.customer_email}`}
            >
              <span className="flex flex-col gap-1">
                <span className="font-[family-name:var(--font-serif,'Source_Serif_4',serif)] text-base text-[color:var(--ink-900)]">
                  {o.order_number}
                </span>
                <span className="text-xs text-[color:var(--ink-900)] opacity-60">
                  {formatPlacedAt(o.placed_at)}
                </span>
              </span>

              <span className="flex flex-col gap-1 text-sm text-[color:var(--ink-900)]">
                <span>{o.customer_name ?? o.customer_email}</span>
                {o.customer_name && (
                  <span className="text-xs opacity-60">{o.customer_email}</span>
                )}
              </span>

              <OrderStatusBadge status={o.status} className="text-sm" />
              <PaymentStatusBadge status={o.payment_status} className="text-sm" />
              <FulfillmentStatusBadge
                status={o.fulfillment_status}
                className="text-sm"
              />

              <span className="text-right">
                <span className="font-[family-name:var(--font-serif,'Source_Serif_4',serif)] text-base text-[color:var(--ink-900)]">
                  {formatMoney(o.grand_total, o.currency_code)}
                </span>
              </span>
            </Link>
          </li>
        ))}
      </ul>
    </div>
  );
}

function formatPlacedAt(iso: string): string {
  try {
    const d = new Date(iso);
    return d.toLocaleDateString(undefined, {
      year: "numeric",
      month: "short",
      day: "numeric",
    });
  } catch {
    return iso;
  }
}

function formatMoney(amount: string, currency: string): string {
  const n = Number.parseFloat(amount);
  if (Number.isNaN(n)) return `${currency} ${amount}`;
  try {
    return new Intl.NumberFormat(undefined, {
      style: "currency",
      currency,
    }).format(n);
  } catch {
    return `${currency} ${amount}`;
  }
}
