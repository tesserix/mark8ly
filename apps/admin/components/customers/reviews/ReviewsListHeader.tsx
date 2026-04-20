import Link from "next/link";

import type { ReviewStatus } from "@/lib/api/marketplace-api";

interface ReviewsListHeaderProps {
  activeStatus?: ReviewStatus;
  counts?: {
    pending?: number;
    approved?: number;
    rejected?: number;
  };
}

interface FilterTab {
  label: string;
  status?: ReviewStatus;
  href: string;
  count?: number;
}

export function ReviewsListHeader({
  activeStatus,
  counts,
}: ReviewsListHeaderProps) {
  const tabs: FilterTab[] = [
    { label: "All", href: "/customers/reviews" },
    {
      label: "Pending",
      status: "pending",
      href: "/customers/reviews?status=pending",
      count: counts?.pending,
    },
    {
      label: "Approved",
      status: "approved",
      href: "/customers/reviews?status=approved",
    },
    {
      label: "Rejected",
      status: "rejected",
      href: "/customers/reviews?status=rejected",
    },
  ];

  return (
    <header className="flex flex-col gap-4">
      <div className="flex flex-col gap-2">
        <p className="eyebrow">Customers</p>
        <h1
          id="reviews-heading"
          className="font-serif text-3xl font-medium tracking-tight text-foreground"
        >
          Reviews
        </h1>
        <p className="max-w-xl text-sm text-foreground-secondary">
          Moderate product reviews submitted by customers. Approved reviews
          appear on the storefront; rejected reviews are hidden.
        </p>
      </div>

      <nav
        aria-label="Filter reviews by status"
        className="flex flex-wrap items-center gap-0 overflow-x-auto border-b border-border-subtle px-1 scrollbar-hide"
      >
        {tabs.map((tab) => {
          const active = tab.status === activeStatus;
          return (
            <Link
              key={tab.label}
              href={tab.href}
              aria-current={active ? "page" : undefined}
              className={
                "-mb-px flex shrink-0 items-baseline gap-1.5 border-b-2 px-3 py-3 text-[11px] font-semibold uppercase tracking-[0.12em] transition-colors focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)] " +
                (active
                  ? "border-[color:var(--ink-900)] text-foreground"
                  : "border-transparent text-foreground-tertiary hover:text-foreground")
              }
            >
              <span>{tab.label}</span>
              {typeof tab.count === "number" && tab.count > 0 && (
                <span
                  className={
                    "text-[11px] tabular-nums " +
                    (active ? "text-foreground-secondary" : "text-foreground-tertiary")
                  }
                >
                  {tab.count}
                </span>
              )}
            </Link>
          );
        })}
      </nav>
    </header>
  );
}
