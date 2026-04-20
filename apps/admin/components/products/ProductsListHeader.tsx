// apps/admin/components/products/ProductsListHeader.tsx
//
// Editorial list page header: small-caps eyebrow, serif "Products" title,
// muted description, and a + New product primary CTA on the right. The
// CTA is hidden for staff (read-only) per §7.8 — enforcement is still on
// the backend. Matches the same header rhythm as `AdminPage` so list and
// settings pages read consistently.

import Link from "next/link";
import { Plus } from "lucide-react";

export interface ProductsListHeaderProps {
  canCreate: boolean;
}

export function ProductsListHeader({ canCreate }: ProductsListHeaderProps) {
  return (
    <header className="flex flex-wrap items-end justify-between gap-x-6 gap-y-4">
      <div className="min-w-0 flex-1 space-y-3">
        <p className="eyebrow">Catalogue</p>
        <h1
          id="products-heading"
          className="font-serif text-4xl font-medium tracking-tight text-foreground text-balance sm:text-5xl"
        >
          Products
        </h1>
        <p className="max-w-2xl text-base leading-7 text-foreground-secondary">
          Every item you sell across this store — inventory, pricing,
          variants, and visibility.
        </p>
      </div>
      {canCreate && (
        <Link
          href="/products/new"
          className="inline-flex items-center gap-2 rounded-md bg-[color:var(--ink-900)] px-4 py-2 text-sm font-medium text-white transition-colors hover:bg-[color:var(--moss-700)] focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)]"
        >
          <Plus className="h-4 w-4" aria-hidden="true" />
          New product
        </Link>
      )}
    </header>
  );
}
