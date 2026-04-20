// components/orders/OrdersListEmpty.tsx
//
// Empty state for the orders list. Two variants:
//   - "no-orders": zero total orders in this store; encouraging copy with
//     a hint that orders flow in from the storefront.
//   - "no-matches": filters are active but no rows; offers a clear-filters link.

import Link from "next/link";

export interface OrdersListEmptyProps {
  variant: "no-orders" | "no-matches";
  clearFiltersHref?: string;
}

export function OrdersListEmpty({
  variant,
  clearFiltersHref,
}: OrdersListEmptyProps) {
  if (variant === "no-orders") {
    return (
      <div className="flex flex-col items-start gap-3 border-t border-border-subtle py-12">
        <h2 className="font-serif text-xl font-medium text-foreground">
          No orders yet
        </h2>
        <p className="max-w-prose text-sm text-foreground-secondary">
          When customers buy from your storefront, every order will land here
          with status, items, shipping, and refund controls.
        </p>
      </div>
    );
  }
  return (
    <div className="flex flex-col items-start gap-3 border-t border-border-subtle py-12">
      <h2 className="font-serif text-xl font-medium text-foreground">
        No orders match your filters
      </h2>
      <p className="max-w-prose text-sm text-foreground-secondary">
        Try adjusting the status or payment filters, or clear them to start over.
      </p>
      {clearFiltersHref && (
        <Link
          href={clearFiltersHref}
          className="text-sm text-[color:var(--moss-700)] underline-offset-4 hover:underline focus-visible:underline"
        >
          Clear filters
        </Link>
      )}
    </div>
  );
}
