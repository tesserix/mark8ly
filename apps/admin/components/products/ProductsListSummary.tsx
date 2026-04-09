// apps/admin/components/products/ProductsListSummary.tsx
//
// Muted "N products total" line under the page header.
//
// Future: once the backend exposes status-breakdown counts, render the
// editorial "42 products · 3 drafts · 2 archived" line from spec §7.2.
// M7a only has total from the admin list endpoint's meta envelope.

import type { ListProductsMeta } from "@/lib/api/marketplace-api";

export interface ProductsListSummaryProps {
  meta: ListProductsMeta;
  statusFilter?: string;
}

export function ProductsListSummary({
  meta,
  statusFilter,
}: ProductsListSummaryProps) {
  const noun = meta.total === 1 ? "product" : "products";
  const filterLabel = statusFilter ? ` · filtered by ${statusFilter}` : "";
  return (
    <p className="text-sm text-[color:var(--ink-900)] opacity-60">
      {meta.total.toLocaleString()} {noun} total
      {filterLabel}
    </p>
  );
}
