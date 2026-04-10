import Link from "next/link";

import type { AdminCustomer } from "@/lib/api/marketplace-api";

import { CustomerStatusBadge } from "./CustomerStatusBadge";

interface CustomersListProps {
  customers: AdminCustomer[];
}

export function CustomersList({ customers }: CustomersListProps) {
  return (
    <div className="flex flex-col">
      <div
        role="row"
        className="grid grid-cols-[minmax(0,2fr)_minmax(0,1.4fr)_minmax(0,0.8fr)_minmax(0,0.8fr)_minmax(0,1fr)_minmax(0,1fr)] items-end gap-6 border-b border-[color:var(--ink-900)] border-opacity-15 pb-3 text-xs uppercase tracking-wider text-[color:var(--ink-900)] opacity-60"
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
            className="border-b border-[color:var(--ink-900)] border-opacity-10"
          >
            <Link
              href={`/customers/${c.id}`}
              className="grid grid-cols-[minmax(0,2fr)_minmax(0,1.4fr)_minmax(0,0.8fr)_minmax(0,0.8fr)_minmax(0,1fr)_minmax(0,1fr)] items-center gap-6 py-4 transition-colors hover:bg-[color:var(--ink-900)] hover:bg-opacity-[0.03] focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)]"
              aria-label={`Customer ${formatName(c)} — ${c.email}`}
            >
              <span className="flex flex-col gap-1">
                <span className="font-[family-name:var(--font-serif,'Source_Serif_4',serif)] text-base text-[color:var(--ink-900)]">
                  {formatName(c)}
                </span>
                {c.tags.length > 0 && (
                  <span className="flex flex-wrap gap-1">
                    {c.tags.slice(0, 3).map((tag) => (
                      <span
                        key={tag}
                        className="rounded-sm bg-[color:var(--ink-900)] bg-opacity-[0.06] px-1.5 py-0.5 text-[10px] text-[color:var(--ink-900)] opacity-70"
                      >
                        {tag}
                      </span>
                    ))}
                    {c.tags.length > 3 && (
                      <span className="text-[10px] text-[color:var(--ink-900)] opacity-50">
                        +{c.tags.length - 3}
                      </span>
                    )}
                  </span>
                )}
              </span>

              <span className="truncate text-sm text-[color:var(--ink-900)] opacity-80">
                {c.email}
              </span>

              <CustomerStatusBadge status={c.status} className="text-sm" />

              <span className="hidden md:block text-right font-[family-name:var(--font-serif,'Source_Serif_4',serif)] text-base text-[color:var(--ink-900)]">
                {c.order_count}
              </span>

              <span className="hidden md:block text-right font-[family-name:var(--font-serif,'Source_Serif_4',serif)] text-base text-[color:var(--ink-900)]">
                {formatMoney(c.total_spent)}
              </span>

              <span className="hidden md:block text-xs text-[color:var(--ink-900)] opacity-60">
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

function formatDate(iso: string): string {
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

function formatMoney(amount: number): string {
  try {
    return new Intl.NumberFormat(undefined, {
      style: "currency",
      currency: "AUD",
      minimumFractionDigits: 2,
    }).format(amount);
  } catch {
    return `$${amount.toFixed(2)}`;
  }
}
