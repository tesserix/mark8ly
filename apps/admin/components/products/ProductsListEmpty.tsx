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
      <div className="flex flex-col items-start gap-3 border-t border-border-subtle py-12">
        <h2 className="font-serif text-xl font-medium text-foreground">
          No products yet
        </h2>
        <p className="max-w-prose text-sm text-foreground-secondary">
          Your catalogue is empty. Add your first product to start selling —
          photos, variants, and pricing all in one place.
        </p>
        <Link
          href="/products/new"
          className="mt-1 inline-flex items-center gap-2 rounded-md bg-[color:var(--ink-900)] px-4 py-2 text-sm text-[color:var(--primary-foreground)] transition-colors hover:bg-[color:var(--moss-700)] focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)]"
        >
          + New product
        </Link>
      </div>
    );
  }
  return (
    <div className="flex flex-col items-start gap-3 border-t border-border-subtle py-12">
      <h2 className="font-serif text-xl font-medium text-foreground">
        No products match your filters
      </h2>
      <p className="max-w-prose text-sm text-foreground-secondary">
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
