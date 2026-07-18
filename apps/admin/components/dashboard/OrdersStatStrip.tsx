"use client";

import Link from "next/link";
import type { DashboardStats } from "@/lib/api/marketplace-api";

interface OrdersStatStripProps {
  stats: DashboardStats;
}

/**
 * Orders as an editorial stat strip — big serif numerals with small-caps
 * labels beneath, not a table of rows or an equal-card grid. Pending takes
 * the single moss accent and becomes a link when there's something waiting,
 * since it's the one actionable count on the strip.
 */
export function OrdersStatStrip({ stats }: OrdersStatStripProps) {
  const hasPending = stats.orders_pending > 0;

  return (
    <section
      aria-labelledby="orders-strip-heading"
      className="border-t border-border-subtle pt-8"
    >
      <h2 id="orders-strip-heading" className="eyebrow">
        Orders
      </h2>
      <div className="mt-5 grid grid-cols-2 gap-y-6 sm:grid-cols-4">
        <StatItem label="Today" value={stats.orders_today} />
        <StatItem
          label="Pending"
          value={stats.orders_pending}
          accent={hasPending}
          href={hasPending ? "/orders?status=pending" : undefined}
        />
        <StatItem label="Fulfilled" value={stats.orders_fulfilled} />
        <StatItem label="Cancelled" value={stats.orders_cancelled} />
      </div>
    </section>
  );
}

function StatItem({
  label,
  value,
  accent,
  href,
}: {
  label: string;
  value: number;
  accent?: boolean;
  href?: string;
}) {
  const body = (
    <>
      <span
        className={`font-serif text-4xl font-medium tabular-nums ${
          accent ? "text-[color:var(--moss-700)]" : "text-foreground"
        }`}
      >
        {value}
      </span>
      <span className="mt-1 text-[11px] font-semibold uppercase tracking-[0.16em] text-foreground-tertiary">
        {label}
      </span>
    </>
  );

  if (href) {
    return (
      <Link
        href={href}
        className="group flex flex-col rounded-sm transition-colors focus-visible:outline-2 focus-visible:outline-offset-4 focus-visible:outline-[color:var(--moss-700)]"
      >
        {body}
        <span className="mt-1 text-xs text-[color:var(--moss-700)] opacity-80 transition-opacity group-hover:opacity-100">
          Awaiting fulfillment →
        </span>
      </Link>
    );
  }

  return <div className="flex flex-col">{body}</div>;
}
