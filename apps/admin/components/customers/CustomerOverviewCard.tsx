import type { AdminCustomerDetail } from "@/lib/api/marketplace-api";
import { formatDate, formatMoney } from "@/lib/format";

interface CustomerOverviewCardProps {
  customer: AdminCustomerDetail;
  currency?: string;
}

export function CustomerOverviewCard({
  customer,
  currency = "USD",
}: CustomerOverviewCardProps) {
  return (
    <section aria-labelledby="overview-heading" className="flex flex-col gap-6">
      <h2
        id="overview-heading"
        className="font-serif text-2xl font-medium text-foreground"
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
          value={formatMoney(customer.total_spent ?? 0, currency)}
        />
        <StatCard
          label="Last order"
          value={customer.last_order_at ? formatDate(customer.last_order_at) : "Never"}
        />
      </div>

      <div className="border-t border-border-subtle pt-6">
        <dl className="grid grid-cols-1 gap-y-4 text-sm sm:grid-cols-2 sm:gap-x-8">
          <div>
            <dt className="text-foreground-tertiary">Email</dt>
            <dd className="mt-1 text-foreground">{customer.email}</dd>
          </div>
          <div>
            <dt className="text-foreground-tertiary">Phone</dt>
            <dd className="mt-1 text-foreground">
              {customer.phone || "Not provided"}
            </dd>
          </div>
          <div>
            <dt className="text-foreground-tertiary">Marketing opt-in</dt>
            <dd className="mt-1 text-foreground">
              {customer.marketing_opt_in ? "Yes" : "No"}
            </dd>
          </div>
          <div>
            <dt className="text-foreground-tertiary">Joined</dt>
            <dd className="mt-1 text-foreground">
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
      <span className="text-xs font-medium uppercase tracking-wider text-foreground-tertiary">
        {label}
      </span>
      <span className="font-serif text-2xl tabular-nums text-foreground">
        {value}
      </span>
    </div>
  );
}
