import Link from "next/link";

import type { AdminCustomerDetail } from "@/lib/api/marketplace-api";

import { CustomerStatusBadge } from "./CustomerStatusBadge";

interface CustomerDetailHeaderProps {
  customer: AdminCustomerDetail;
}

export function CustomerDetailHeader({ customer }: CustomerDetailHeaderProps) {
  const name = formatName(customer);

  return (
    <header className="flex flex-col gap-4">
      <Link
        href="/customers"
        className="text-sm text-[color:var(--moss-700)] underline-offset-4 hover:underline focus-visible:underline"
      >
        Back to customers
      </Link>

      <div className="flex items-start justify-between gap-4">
        <div className="flex flex-col gap-2">
          <h1
            id="customer-heading"
            className="font-[family-name:var(--font-serif,'Source_Serif_4',serif)] text-4xl text-[color:var(--ink-900)]"
          >
            {name}
          </h1>
          <p className="text-sm text-[color:var(--ink-900)] opacity-70">
            {customer.email}
            {customer.phone ? ` \u00b7 ${customer.phone}` : ""}
          </p>
        </div>

        <CustomerStatusBadge status={customer.status} className="text-base" />
      </div>

      {customer.status === "blocked" && customer.block_reason && (
        <p className="text-sm text-[color:var(--signal,#C4391D)]">
          Blocked: {customer.block_reason}
        </p>
      )}
    </header>
  );
}

function formatName(c: { first_name?: string; last_name?: string; email: string }): string {
  const parts = [c.first_name, c.last_name].filter(Boolean);
  return parts.length > 0 ? parts.join(" ") : c.email;
}
