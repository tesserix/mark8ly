// components/orders/OrdersList.tsx
//
// Editorial-style orders table. Hairline-rule rows (no boxed cards),
// left-aligned, monetary totals right-aligned. Each row links to the
// detail page (`/orders/[id]`). Row component is memoized so scrolling
// + filter changes don't re-render every row.

import { memo } from "react";
import Link from "next/link";

import type { AdminOrder } from "@/lib/api/marketplace-api";
import { formatDate, formatMoney } from "@/lib/format";

import {
  FulfillmentStatusBadge,
  OrderStatusBadge,
  PaymentStatusBadge,
} from "./OrderStatusBadges";

interface OrdersListProps {
  orders: AdminOrder[];
}

const GRID = "grid-cols-[minmax(0,1.4fr)_minmax(0,1.6fr)_minmax(0,1.2fr)_minmax(0,1.2fr)_minmax(0,1fr)_minmax(0,1fr)]";

export function OrdersList({ orders }: OrdersListProps) {
  return (
    <div className="flex flex-col">
      <div
        role="presentation"
        className={`grid ${GRID} items-end gap-6 border-b border-[color:var(--ink-900)]/15 px-4 pb-3 text-xs font-medium uppercase tracking-wider text-foreground-tertiary`}
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
          <OrderRow key={o.id} order={o} />
        ))}
      </ul>
    </div>
  );
}

const OrderRow = memo(function OrderRow({ order: o }: { order: AdminOrder }) {
  return (
    <li className="border-b border-[color:var(--ink-900)]/10">
      <Link
        href={`/orders/${o.id}`}
        className={`grid ${GRID} items-center gap-6 px-4 py-4 transition-colors hover:bg-[color:var(--ink-900)]/[0.03] focus-visible:outline-2 focus-visible:outline-offset-[-2px] focus-visible:outline-[color:var(--moss-700)]`}
        aria-label={`Order ${o.order_number} for ${o.customer_email}`}
      >
        <span className="flex min-w-0 flex-col gap-1">
          <span className="truncate font-serif text-base text-foreground">
            {o.order_number}
          </span>
          <span className="truncate text-xs text-foreground-tertiary">
            {formatDate(o.placed_at)}
          </span>
        </span>

        <span className="flex min-w-0 flex-col gap-1 text-sm text-foreground">
          <span className="truncate">{o.customer_name ?? o.customer_email}</span>
          {o.customer_name && (
            <span className="truncate text-xs text-foreground-tertiary">{o.customer_email}</span>
          )}
        </span>

        <OrderStatusBadge status={o.status} className="text-sm" />
        <PaymentStatusBadge status={o.payment_status} className="text-sm" />
        <FulfillmentStatusBadge
          status={o.fulfillment_status}
          className="text-sm"
        />

        <span className="text-right">
          <span className="font-serif text-base tabular-nums text-foreground">
            {formatMoney(o.grand_total, o.currency_code)}
          </span>
        </span>
      </Link>
    </li>
  );
});
