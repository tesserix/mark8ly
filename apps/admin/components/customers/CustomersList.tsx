import Link from "next/link";

import type { AdminCustomer } from "@/lib/api/marketplace-api";
import { formatDate, formatMoney } from "@/lib/format";

import { CustomerStatusBadge } from "./CustomerStatusBadge";

interface CustomersListProps {
  customers: AdminCustomer[];
  currency?: string;
}

const GRID = "grid-cols-[minmax(0,2fr)_minmax(0,1.4fr)_minmax(0,0.8fr)_minmax(0,0.8fr)_minmax(0,1fr)_minmax(0,1fr)]";

export function CustomersList({ customers, currency = "USD" }: CustomersListProps) {
  return (
    <div className="flex flex-col">
      <div
        role="row"
        className={`grid ${GRID} items-end gap-6 border-b border-[color:var(--ink-900)]/15 px-4 pb-3 text-xs font-medium uppercase tracking-wider text-foreground-tertiary`}
      >
        <span>Customer</span>
        <span>Email</span>
        <span>Status</span>
        <span className="hidden md:block text-right">Orders</span>
        <span className="hidden md:block text-right">Total spent</span>
        <span className="hidden md:block">Joined</span>
      </div>

      <ul role="list" className="flex flex-col">
        {customers.map((c) => (
          <li
            key={c.id}
            className="border-b border-[color:var(--ink-900)]/10"
          >
            <Link
              href={`/customers/${c.id}`}
              className={`grid ${GRID} items-center gap-6 px-4 py-4 transition-colors hover:bg-[color:var(--ink-900)]/[0.03] focus-visible:outline-2 focus-visible:outline-offset-[-2px] focus-visible:outline-[color:var(--moss-700)]`}
              aria-label={`Customer ${formatName(c)} — ${c.email}`}
            >
              <span className="flex flex-col gap-1">
                <span className="font-serif text-base text-foreground">
                  {formatName(c)}
                </span>
                {c.tags.length > 0 && (
                  <span className="flex flex-wrap gap-1">
                    {c.tags.slice(0, 3).map((tag) => (
                      <span
                        key={tag}
                        className="rounded-sm bg-[color:var(--ink-900)]/[0.06] px-1.5 py-0.5 text-[10px] text-foreground-secondary"
                      >
                        {tag}
                      </span>
                    ))}
                    {c.tags.length > 3 && (
                      <span className="text-[10px] text-foreground-tertiary">
                        +{c.tags.length - 3}
                      </span>
                    )}
                  </span>
                )}
              </span>

              <span className="truncate text-sm text-foreground-secondary">
                {c.email}
              </span>

              <CustomerStatusBadge status={c.status} className="text-sm" />

              <span className="hidden md:block text-right font-serif text-base tabular-nums text-foreground">
                {c.order_count}
              </span>

              <span className="hidden md:block text-right font-serif text-base tabular-nums text-foreground">
                {formatMoney(c.total_spent, currency)}
              </span>

              <span className="hidden md:block text-xs text-foreground-tertiary">
                {formatDate(c.created_at)}
              </span>
            </Link>
          </li>
        ))}
      </ul>
    </div>
  );
}

function formatName(c: AdminCustomer): string {
  const parts = [c.first_name, c.last_name].filter(Boolean);
  return parts.length > 0 ? parts.join(" ") : c.email;
}
