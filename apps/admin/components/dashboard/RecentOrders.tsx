"use client";

import Link from "next/link";

import type { RecentOrder, OrderStatus } from "@/lib/api/marketplace-api";

interface RecentOrdersProps {
  orders: RecentOrder[];
  currencyCode: string;
}

function formatMoney(amount: number, currencyCode: string): string {
  try {
    return new Intl.NumberFormat(undefined, {
      style: "currency",
      currency: currencyCode,
    }).format(amount);
  } catch {
    return `${currencyCode} ${amount.toFixed(2)}`;
  }
}

function statusBadge(status: OrderStatus): string {
  switch (status) {
    case "pending":
      return "bg-[color:var(--ink-900)]/10 text-[color:var(--ink-900)]/60";
    case "confirmed":
      return "bg-[color:var(--moss-700)]/10 text-[color:var(--moss-700)]";
    case "fulfilled":
      return "bg-[color:var(--moss-700)] text-white";
    case "cancelled":
      return "bg-[color:var(--signal)]/10 text-[color:var(--signal)]";
    default:
      return "bg-[color:var(--ink-900)]/10 text-[color:var(--ink-900)]/60";
  }
}

function timeAgo(dateStr: string): string {
  const diff = Date.now() - new Date(dateStr).getTime();
  const mins = Math.floor(diff / 60_000);
  if (mins < 1) return "just now";
  if (mins < 60) return `${mins}m ago`;
  const hrs = Math.floor(mins / 60);
  if (hrs < 24) return `${hrs}h ago`;
  const days = Math.floor(hrs / 24);
  return `${days}d ago`;
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
      <h3 className="font-[family-name:var(--font-source-serif),'Source_Serif_4',serif] text-lg font-medium text-foreground">
        Recent orders
      </h3>
      <div className="overflow-x-auto">
      <ul className="mt-4 divide-y divide-border-subtle" role="list">
        {orders.map((order) => (
          <li key={order.id}>
            <Link
              href={`/orders/${order.id}`}
              className="flex min-h-[44px] items-center justify-between gap-4 py-3 transition-colors hover:bg-paper-100"
            >
              <div className="min-w-0 flex-1">
                <div className="flex items-center gap-3">
                  <span
                    className="font-mono text-sm font-medium text-foreground"
                    style={{ fontFeatureSettings: '"tnum" 1' }}
                  >
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
                <p
                  className="font-[family-name:var(--font-source-serif),'Source_Serif_4',serif] text-sm font-medium text-foreground"
                  style={{ fontFeatureSettings: '"tnum" 1' }}
                >
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
