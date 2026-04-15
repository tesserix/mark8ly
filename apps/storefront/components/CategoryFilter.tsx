// apps/storefront/components/CategoryFilter.tsx
//
// Horizontal row of category pill links for the product list page.
// "All" is always first; the active category gets the ink fill.

import Link from "next/link";
import type { StorefrontCategory } from "@/lib/api/marketplace-api";

interface CategoryFilterProps {
  categories: StorefrontCategory[];
  activeCategorySlug?: string;
}

export function CategoryFilter({
  categories,
  activeCategorySlug,
}: CategoryFilterProps) {
  if (categories.length === 0) return null;

  const isAll = !activeCategorySlug;

  return (
    <nav aria-label="Categories" className="flex flex-wrap gap-2">
      <Link
        href="/products"
        className={[
          "rounded-full border px-4 py-1.5 text-sm transition-all",
          "focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--storefront-accent,var(--moss-700))]",
          isAll
            ? "border-[color:var(--storefront-text,var(--ink-900))] bg-[color:var(--storefront-accent,var(--ink-900))] text-[color:var(--storefront-on-accent,var(--paper-200))]"
            : "border-[color:var(--storefront-text,var(--ink-900))]/15 text-[color:var(--storefront-text,var(--ink-900))] opacity-70 hover:opacity-100",
        ].join(" ")}
      >
        All
      </Link>
      {categories.map((cat) => {
        const active = cat.slug === activeCategorySlug;
        return (
          <Link
            key={cat.slug}
            href={`/categories/${encodeURIComponent(cat.slug)}`}
            className={[
              "rounded-full border px-4 py-1.5 text-sm transition-all",
              "focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--storefront-accent,var(--moss-700))]",
              active
                ? "border-[color:var(--storefront-text,var(--ink-900))] bg-[color:var(--storefront-accent,var(--ink-900))] text-[color:var(--storefront-on-accent,var(--paper-200))]"
                : "border-[color:var(--storefront-text,var(--ink-900))]/15 text-[color:var(--storefront-text,var(--ink-900))] opacity-70 hover:opacity-100",
            ].join(" ")}
          >
            {cat.name}
          </Link>
        );
      })}
    </nav>
  );
}
