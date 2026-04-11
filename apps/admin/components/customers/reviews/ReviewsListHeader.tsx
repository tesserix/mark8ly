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
        className="flex flex-wrap items-center gap-1 border-b border-border-subtle"
      >
        {tabs.map((tab) => {
          const active = tab.status === activeStatus;
          return (
            <Link
              key={tab.label}
              href={tab.href}
              aria-current={active ? "page" : undefined}
              className={`inline-flex min-h-[2.75rem] items-center gap-2 border-b-2 px-4 py-2 text-sm transition-colors ${
                active
                  ? "border-[color:var(--moss-700)] text-foreground"
                  : "border-transparent text-foreground-secondary hover:text-foreground"
              }`}
            >
              <span>{tab.label}</span>
              {typeof tab.count === "number" && tab.count > 0 && (
                <span className="rounded-full bg-paper-100 px-2 py-0.5 text-[11px] font-semibold text-foreground-secondary">
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
