"use client";

import Link from "next/link";

import type { RecentOrder, OrderStatus } from "@/lib/api/marketplace-api";
import { formatMoney, timeAgo } from "@/lib/format";

interface RecentOrdersProps {
  orders: RecentOrder[];
  currencyCode: string;
}

function statusBadge(status: OrderStatus): string {
  switch (status) {
    case "pending":
      return "bg-[color:var(--ink-900)]/10 text-foreground-secondary";
    case "confirmed":
      return "bg-[color:var(--accent-tint)] text-[color:var(--moss-700)]";
    case "fulfilled":
      return "bg-[color:var(--moss-700)] text-[color:var(--primary-foreground)]";
    case "cancelled":
      return "bg-[color:var(--signal)]/10 text-[color:var(--signal)]";
    default:
      return "bg-[color:var(--ink-900)]/10 text-foreground-secondary";
  }
}

export function RecentOrders({ orders, currencyCode }: RecentOrdersProps) {
  if (orders.length === 0) {
    return (
      <div className="py-8 text-center">
        <p className="text-sm text-foreground-secondary">
          No orders yet. They will appear here once customers start purchasing.
        </p>
      </div>
    );
  }

  return (
    <div>
      <h3 className="font-serif text-lg font-medium text-foreground">
        Recent orders
      </h3>
      <div className="overflow-x-auto">
        <ul className="mt-4 divide-y divide-border-subtle" role="list">
          {orders.map((order) => (
            <li key={order.id}>
              <Link
                href={`/orders/${order.id}`}
                className="flex min-h-[44px] items-center justify-between gap-4 px-4 py-3 transition-colors hover:bg-[color:var(--ink-900)]/[0.03] focus-visible:outline-2 focus-visible:outline-offset-[-2px] focus-visible:outline-[color:var(--moss-700)]"
              >
                <div className="min-w-0 flex-1">
                  <div className="flex items-center gap-3">
                    <span className="font-mono text-sm font-medium tabular-nums text-foreground">
                      {order.order_number}
                    </span>
                    <span
                      className={`inline-flex items-center rounded-full px-2 py-0.5 text-[11px] font-medium ${statusBadge(order.status)}`}
                    >
                      {order.status}
                    </span>
                  </div>
                  <p className="mt-0.5 truncate text-sm text-foreground-secondary">
                    {order.customer_email}
                  </p>
                </div>
                <div className="text-right">
                  <p className="font-serif text-sm font-medium tabular-nums text-foreground">
                    {formatMoney(order.grand_total, currencyCode)}
                  </p>
                  <p className="text-xs text-foreground-tertiary">
                    {timeAgo(order.created_at)}
                  </p>
                </div>
              </Link>
            </li>
          ))}
        </ul>
      </div>
    </div>
  );
}
