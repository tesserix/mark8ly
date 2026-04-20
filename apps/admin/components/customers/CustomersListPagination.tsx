// Customers list pagination. Numbered + ellipsis, current page rendered
// as a serif tabular-nums number with a moss underline — matches the
// orders + products pagination so the editorial rhythm stays consistent
// across every list surface.

import Link from "next/link";
import type { ReactNode } from "react";
import { ChevronLeft, ChevronRight } from "lucide-react";

export interface CustomersListPaginationProps {
  currentPage: number;
  totalPages: number;
  /** Builds an href for the given target page, preserving other search params. */
  buildHref: (page: number) => string;
}

export function CustomersListPagination({
  currentPage,
  totalPages,
  buildHref,
}: CustomersListPaginationProps) {
  if (totalPages <= 1) return null;
  const pages = buildPageList(currentPage, totalPages);
  const prevDisabled = currentPage <= 1;
  const nextDisabled = currentPage >= totalPages;
  return (
    <nav className="flex items-center gap-1 pt-4" aria-label="Customers pagination">
      <PageButton
        href={prevDisabled ? undefined : buildHref(currentPage - 1)}
        disabled={prevDisabled}
        ariaLabel="Previous page"
      >
        <ChevronLeft className="h-4 w-4" />
      </PageButton>
      {pages.map((p, i) =>
        p === "…" ? (
          <span
            key={`ellipsis-${i}`}
            className="px-2 text-sm text-foreground-tertiary opacity-60"
            aria-hidden="true"
          >
            …
          </span>
        ) : (
          <PageButton
            key={p}
            href={p === currentPage ? undefined : buildHref(p)}
            current={p === currentPage}
            ariaLabel={`Page ${p}`}
          >
            {p}
          </PageButton>
        ),
      )}
      <PageButton
        href={nextDisabled ? undefined : buildHref(currentPage + 1)}
        disabled={nextDisabled}
        ariaLabel="Next page"
      >
        <ChevronRight className="h-4 w-4" />
      </PageButton>
    </nav>
  );
}

interface PageButtonProps {
  href?: string;
  children: ReactNode;
  disabled?: boolean;
  current?: boolean;
  ariaLabel: string;
}

function PageButton({
  href,
  children,
  disabled,
  current,
  ariaLabel,
}: PageButtonProps) {
  const base =
    "inline-flex h-9 min-w-9 items-center justify-center px-2 text-sm tabular-nums transition-colors focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)]";
  const variants = current
    ? "font-serif text-base text-foreground underline underline-offset-[6px] decoration-[color:var(--moss-700)] decoration-[1.5px]"
    : disabled
      ? "text-foreground-tertiary opacity-50"
      : "text-foreground-secondary hover:text-[color:var(--moss-700)]";
  if (href) {
    return (
      <Link
        href={href}
        aria-label={ariaLabel}
        aria-current={current ? "page" : undefined}
        className={`${base} ${variants}`}
      >
        {children}
      </Link>
    );
  }
  return (
    <span
      aria-label={ariaLabel}
      aria-current={current ? "page" : undefined}
      aria-disabled={disabled || undefined}
      className={`${base} ${variants}`}
    >
      {children}
    </span>
  );
}

function buildPageList(current: number, total: number): (number | "…")[] {
  if (total <= 7) return Array.from({ length: total }, (_, i) => i + 1);
  const pages: (number | "…")[] = [1];
  if (current > 3) pages.push("…");
  const start = Math.max(2, current - 1);
  const end = Math.min(total - 1, current + 1);
  for (let i = start; i <= end; i++) pages.push(i);
  if (current < total - 2) pages.push("…");
  pages.push(total);
  return pages;
}
