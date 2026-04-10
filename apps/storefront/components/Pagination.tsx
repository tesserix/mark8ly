// apps/storefront/components/Pagination.tsx
//
// Server-friendly pagination with prev/next links. Uses the heuristic
// "data.length === pageSize → there might be a next page" since the
// API meta doesn't include total count.

import Link from "next/link";

interface PaginationProps {
  currentPage: number;
  pageSize: number;
  itemCount: number;
  basePath: string;
  /** Extra query params to preserve (e.g. search, slug) */
  extraParams?: Record<string, string>;
}

function buildHref(
  basePath: string,
  page: number,
  extra: Record<string, string>,
): string {
  const params = new URLSearchParams(extra);
  if (page > 1) params.set("page", String(page));
  const qs = params.toString();
  return qs ? `${basePath}?${qs}` : basePath;
}

export function Pagination({
  currentPage,
  pageSize,
  itemCount,
  basePath,
  extraParams = {},
}: PaginationProps) {
  const hasPrev = currentPage > 1;
  const hasNext = itemCount === pageSize;

  if (!hasPrev && !hasNext) return null;

  return (
    <nav
      aria-label="Pagination"
      className="mt-12 flex items-center justify-between border-t border-[color:var(--ink-900)]/10 pt-6"
    >
      {hasPrev ? (
        <Link
          href={buildHref(basePath, currentPage - 1, extraParams)}
          className="text-sm font-semibold text-[color:var(--ink-900)] opacity-70 transition-opacity hover:opacity-100 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)]"
        >
          ← Previous
        </Link>
      ) : (
        <span />
      )}

      <span
        className="text-xs text-[color:var(--ink-900)] opacity-40"
        style={{ fontFeatureSettings: '"tnum" 1, "lnum" 1' }}
      >
        Page {currentPage}
      </span>

      {hasNext ? (
        <Link
          href={buildHref(basePath, currentPage + 1, extraParams)}
          className="text-sm font-semibold text-[color:var(--ink-900)] opacity-70 transition-opacity hover:opacity-100 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)]"
        >
          Next →
        </Link>
      ) : (
        <span />
      )}
    </nav>
  );
}
