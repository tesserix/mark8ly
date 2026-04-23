// apps/storefront/components/CategoryFilter.tsx
//
// Editorial category filter: small-caps text labels for the merchant-
// curated "featured" subset plus "All" and a "Browse all categories →"
// link to the full index at /categories. Falls back to the top six by
// position when no categories are featured, so a fresh store still has
// a useful filter row instead of nothing.

import Link from "next/link";
import type { StorefrontCategory } from "@/lib/api/marketplace-api";

const FALLBACK_LIMIT = 6;

interface CategoryFilterProps {
  categories: StorefrontCategory[];
  activeCategorySlug?: string;
}

function pickVisible(
  categories: StorefrontCategory[],
  activeCategorySlug?: string,
): StorefrontCategory[] {
  const sorted = [...categories].sort((a, b) => {
    if (a.position !== b.position) return a.position - b.position;
    return a.name.localeCompare(b.name);
  });
  const featured = sorted.filter((c) => c.featured);
  const base = featured.length > 0 ? featured : sorted.slice(0, FALLBACK_LIMIT);
  if (!activeCategorySlug) return base;
  if (base.some((c) => c.slug === activeCategorySlug)) return base;
  // Active category isn't in the featured set — hoist it so the user
  // always sees what they're filtering on.
  const active = sorted.find((c) => c.slug === activeCategorySlug);
  return active ? [active, ...base] : base;
}

export function CategoryFilter({
  categories,
  activeCategorySlug,
}: CategoryFilterProps) {
  if (categories.length === 0) return null;

  const isAll = !activeCategorySlug;
  const visible = pickVisible(categories, activeCategorySlug);
  const hasMore = categories.length > visible.length;

  const labelClass = (active: boolean) =>
    [
      "inline-flex items-center py-2.5 text-sm tracking-wide transition-colors",
      "focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--storefront-accent,var(--moss-700))]",
      active
        ? "text-[color:var(--storefront-text,var(--ink-900))] underline underline-offset-[6px] decoration-1"
        : "text-[color:var(--storefront-text,var(--ink-900))]/55 hover:text-[color:var(--storefront-text,var(--ink-900))]",
    ].join(" ");

  return (
    <nav
      aria-label="Categories"
      className="flex flex-wrap items-baseline gap-x-6 gap-y-1"
    >
      <Link href="/products" className={labelClass(isAll)}>
        All
      </Link>
      {visible.map((cat) => (
        <Link
          key={cat.slug}
          href={`/categories/${encodeURIComponent(cat.slug)}`}
          className={labelClass(cat.slug === activeCategorySlug)}
        >
          {cat.name}
        </Link>
      ))}
      {hasMore && (
        <Link
          href="/categories"
          className="inline-flex items-center py-2.5 text-sm tracking-wide text-[color:var(--storefront-accent,var(--moss-700))] transition-opacity hover:opacity-80 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--storefront-accent,var(--moss-700))]"
        >
          Browse all categories →
        </Link>
      )}
    </nav>
  );
}
