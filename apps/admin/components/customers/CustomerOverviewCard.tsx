import type { AdminCustomerDetail } from "@/lib/api/marketplace-api";

interface CustomerOverviewCardProps {
  customer: AdminCustomerDetail;
}

export function CustomerOverviewCard({ customer }: CustomerOverviewCardProps) {
  return (
    <section aria-labelledby="overview-heading" className="flex flex-col gap-6">
      <h2
        id="overview-heading"
        className="font-[family-name:var(--font-serif,'Source_Serif_4',serif)] text-2xl text-[color:var(--ink-900)]"
      >
        Overview
      </h2>

      <div className="grid grid-cols-1 gap-6 sm:grid-cols-3 sm:gap-8">
        <StatCard
          label="Orders"
          value={String(customer.order_count ?? 0)}
        />
        <StatCard
          label="Total spent"
          value={formatMoney(customer.total_spent ?? 0)}
        />
        <StatCard
          label="Last order"
          value={customer.last_order_at ? formatDate(customer.last_order_at) : "Never"}
        />
      </div>

      <div className="border-t border-[color:var(--ink-900)] border-opacity-10 pt-6">
        <dl className="grid grid-cols-1 gap-y-4 sm:grid-cols-2 sm:gap-x-8 text-sm">
          <div>
            <dt className="text-[color:var(--ink-900)] opacity-60">Email</dt>
            <dd className="mt-1 text-[color:var(--ink-900)]">{customer.email}</dd>
          </div>
          <div>
            <dt className="text-[color:var(--ink-900)] opacity-60">Phone</dt>
            <dd className="mt-1 text-[color:var(--ink-900)]">
              {customer.phone || "Not provided"}
            </dd>
          </div>
          <div>
            <dt className="text-[color:var(--ink-900)] opacity-60">Marketing opt-in</dt>
            <dd className="mt-1 text-[color:var(--ink-900)]">
              {customer.marketing_opt_in ? "Yes" : "No"}
            </dd>
          </div>
          <div>
            <dt className="text-[color:var(--ink-900)] opacity-60">Joined</dt>
            <dd className="mt-1 text-[color:var(--ink-900)]">
              {formatDate(customer.created_at)}
            </dd>
          </div>
        </dl>
      </div>
    </section>
  );
}

function StatCard({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex flex-col gap-1">
      <span className="text-xs uppercase tracking-wider text-[color:var(--ink-900)] opacity-60">
        {label}
      </span>
      <span className="font-[family-name:var(--font-serif,'Source_Serif_4',serif)] text-2xl text-[color:var(--ink-900)]">
        {value}
      </span>
    </div>
  );
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
