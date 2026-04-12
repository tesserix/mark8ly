// apps/admin/components/products/ProductsListEmpty.tsx
//
// Two empty-state variants per spec §7.2:
//   - "no-products": zero total products in the store; big + New product CTA
//   - "no-matches": filters are active but no results; offers Clear filters link

import Link from "next/link";

export interface ProductsListEmptyProps {
  variant: "no-products" | "no-matches";
  clearFiltersHref?: string;
}

export function ProductsListEmpty({
  variant,
  clearFiltersHref,
}: ProductsListEmptyProps) {
  if (variant === "no-products") {
    return (
      <div className="flex flex-col items-start gap-3 border-t border-[color:var(--ink-900)]/10 py-12">
        <h2 className="text-lg font-medium text-[color:var(--ink-900)]">
          No products yet
        </h2>
        <p className="max-w-prose text-sm text-[color:var(--ink-900)] opacity-70">
          Your catalogue is empty. Add your first product to start selling —
          photos, variants, and pricing all in one place.
        </p>
        <Link
          href="/products/new"
          className="mt-1 inline-flex items-center gap-2 rounded-md bg-[color:var(--ink-900)] px-4 py-2 text-sm text-[color:var(--paper-200)] transition-opacity hover:opacity-90 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)]"
        >
          + New product
        </Link>
      </div>
    );
  }
  return (
    <div className="flex flex-col items-start gap-3 border-t border-[color:var(--ink-900)]/10 py-12">
      <h2 className="text-lg font-medium text-[color:var(--ink-900)]">
        No products match your filters
      </h2>
      <p className="max-w-prose text-[color:var(--ink-900)] opacity-70">
        Try adjusting your search or clearing the filters.
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
