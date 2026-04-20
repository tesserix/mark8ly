import Link from "next/link";

export interface CustomersListEmptyProps {
  variant: "no-customers" | "no-matches";
  clearFiltersHref?: string;
}

export function CustomersListEmpty({
  variant,
  clearFiltersHref,
}: CustomersListEmptyProps) {
  if (variant === "no-customers") {
    return (
      <div className="flex flex-col items-start gap-4 py-16">
        <h2 className="font-serif text-3xl font-medium text-foreground">
          No customers yet
        </h2>
        <p className="max-w-prose text-sm text-foreground-secondary">
          When customers create accounts or place orders on your storefront,
          their profiles will appear here.
        </p>
      </div>
    );
  }
  return (
    <div className="flex flex-col items-start gap-3 py-12">
      <h2 className="font-serif text-2xl font-medium text-foreground">
        No customers match your filters
      </h2>
      <p className="max-w-prose text-sm text-foreground-secondary">
        Try adjusting the search or filters, or clear them to start over.
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
