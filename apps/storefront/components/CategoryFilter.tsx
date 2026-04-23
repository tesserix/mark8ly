// apps/storefront/components/CategoryFilter.tsx
//
// Horizontal row of category pill links for the product list page.
// "All" is always first; the active category gets the ink fill.
// When the store has many categories we collapse the overflow behind a
// "Show all N" toggle so the page doesn't open with a wall of chips.

"use client";

import Link from "next/link";
import { useState } from "react";
import type { StorefrontCategory } from "@/lib/api/marketplace-api";

const COLLAPSED_LIMIT = 10;

interface CategoryFilterProps {
  categories: StorefrontCategory[];
  activeCategorySlug?: string;
}

export function CategoryFilter({
  categories,
  activeCategorySlug,
}: CategoryFilterProps) {
  const [expanded, setExpanded] = useState(false);

  if (categories.length === 0) return null;

  const isAll = !activeCategorySlug;
  const hasOverflow = categories.length > COLLAPSED_LIMIT;

  // Keep the active category in the visible set even when collapsed —
  // if it's past the limit, hoist it to the front so the user always
  // sees what they're filtering on.
  const activeIndex = activeCategorySlug
    ? categories.findIndex((c) => c.slug === activeCategorySlug)
    : -1;
  const ordered =
    activeIndex >= COLLAPSED_LIMIT
      ? [
          categories[activeIndex]!,
          ...categories.filter((_, i) => i !== activeIndex),
        ]
      : categories;

  const visible =
    expanded || !hasOverflow ? ordered : ordered.slice(0, COLLAPSED_LIMIT);

  const chipClass = (active: boolean) =>
    [
      "inline-flex min-h-[44px] items-center rounded-full border px-4 py-2 text-sm transition-colors",
      "focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--storefront-accent,var(--moss-700))]",
      active
        ? "border-[color:var(--storefront-text,var(--ink-900))] bg-[color:var(--storefront-accent,var(--ink-900))] text-[color:var(--storefront-on-accent,var(--paper-200))]"
        : "border-[color:var(--storefront-text,var(--ink-900))]/15 text-[color:var(--storefront-text,var(--ink-900))] opacity-70 hover:opacity-100",
    ].join(" ");

  return (
    <div className="flex flex-col gap-3">
      <nav aria-label="Categories" className="flex flex-wrap gap-2">
        <Link href="/products" className={chipClass(isAll)}>
          All
        </Link>
        {visible.map((cat) => (
          <Link
            key={cat.slug}
            href={`/categories/${encodeURIComponent(cat.slug)}`}
            className={chipClass(cat.slug === activeCategorySlug)}
          >
            {cat.name}
          </Link>
        ))}
      </nav>
      {hasOverflow && (
        <button
          type="button"
          onClick={() => setExpanded((e) => !e)}
          aria-expanded={expanded}
          className="self-start text-xs uppercase tracking-widest text-[color:var(--storefront-text,var(--ink-900))]/60 transition-colors hover:text-[color:var(--storefront-accent,var(--moss-700))] focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--storefront-accent,var(--moss-700))]"
        >
          {expanded
            ? "Show fewer"
            : `Show all ${categories.length} categories`}
        </button>
      )}
    </div>
  );
}
