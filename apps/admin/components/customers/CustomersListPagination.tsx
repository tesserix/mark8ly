import Link from "next/link";

interface CustomersListPaginationProps {
  currentPage: number;
  totalPages: number;
  buildHref: (page: number) => string;
}

export function CustomersListPagination({
  currentPage,
  totalPages,
  buildHref,
}: CustomersListPaginationProps) {
  if (totalPages <= 1) return null;

  return (
    <nav
      aria-label="Customer list pagination"
      className="flex items-center justify-between border-t border-[color:var(--ink-900)] border-opacity-10 pt-4"
    >
      <span className="text-sm text-[color:var(--ink-900)] opacity-60">
        Page {currentPage} of {totalPages}
      </span>

      <div className="flex gap-2">
        {currentPage > 1 && (
          <Link
            href={buildHref(currentPage - 1)}
            className="rounded-md border border-[color:var(--ink-900)] border-opacity-20 px-3 py-2.5 text-sm text-[color:var(--ink-900)] transition-colors hover:bg-[color:var(--ink-900)]/[0.04] focus-visible:outline-2 focus-visible:outline-[color:var(--moss-700)]"
          >
            Previous
          </Link>
        )}
        {currentPage < totalPages && (
          <Link
            href={buildHref(currentPage + 1)}
            className="rounded-md border border-[color:var(--ink-900)] border-opacity-20 px-3 py-2.5 text-sm text-[color:var(--ink-900)] transition-colors hover:bg-[color:var(--ink-900)]/[0.04] focus-visible:outline-2 focus-visible:outline-[color:var(--moss-700)]"
          >
            Next
          </Link>
        )}
      </div>
    </nav>
  );
}
