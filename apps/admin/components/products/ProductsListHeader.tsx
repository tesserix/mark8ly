// apps/admin/components/products/ProductsListHeader.tsx
//
// Editorial page header: small-caps eyebrow, serif "Products" title,
// and a + New product primary CTA on the right. The CTA is hidden for
// staff (read-only) per §7.8 — enforcement is still on the backend.

import Link from "next/link";
import { Plus } from "lucide-react";

export interface ProductsListHeaderProps {
  canCreate: boolean;
}

export function ProductsListHeader({ canCreate }: ProductsListHeaderProps) {
  return (
    <header className="flex flex-wrap items-start justify-between gap-4 border-b border-[color:var(--ink-900)] border-opacity-10 pb-6">
      <div className="flex flex-col gap-1">
        <span className="text-xs font-medium uppercase tracking-widest text-[color:var(--ink-900)] opacity-50">
          Catalogue
        </span>
        <h1 className="font-[family-name:var(--font-serif,'Source_Serif_4',serif)] text-4xl leading-tight text-[color:var(--ink-900)]">
          Products
        </h1>
      </div>
      {canCreate && (
        <Link
          href="/products/new"
          className="inline-flex items-center gap-2 rounded-md bg-[color:var(--ink-900)] px-4 py-2 text-sm text-[color:var(--paper-200)] transition-opacity hover:opacity-90 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)]"
        >
          <Plus className="h-4 w-4" aria-hidden="true" />
          New product
        </Link>
      )}
    </header>
  );
}
