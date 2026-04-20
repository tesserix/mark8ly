"use client";

// Editorial filter bar for /customers. Hairline search + underline
// status chips, matching the pattern used on /orders and /products.
// URL is the source of truth — changes navigate with preserved state.

import { useRouter, useSearchParams, usePathname } from "next/navigation";
import { useEffect, useState } from "react";
import Link from "next/link";

type StatusValue = "active" | "blocked" | undefined;

const STATUS_OPTIONS: { value: StatusValue; label: string }[] = [
  { value: undefined, label: "All" },
  { value: "active", label: "Active" },
  { value: "blocked", label: "Blocked" },
];

export function CustomersListFilters() {
  const router = useRouter();
  const pathname = usePathname();
  const searchParams = useSearchParams();

  const [searchDraft, setSearchDraft] = useState(
    searchParams.get("search") ?? "",
  );
  const status = (searchParams.get("status") ?? undefined) as StatusValue;
  const tag = searchParams.get("tag") ?? "";
  const [tagDraft, setTagDraft] = useState(tag);
  const hasFilters = !!status || !!searchDraft || !!tagDraft;

  // Debounce search → URL navigation. The URL is the source of truth.
  useEffect(() => {
    const handler = setTimeout(() => {
      const params = new URLSearchParams(searchParams.toString());
      if (searchDraft) {
        params.set("search", searchDraft);
      } else {
        params.delete("search");
      }
      params.delete("page");
      const qs = params.toString();
      router.push(qs ? `${pathname}?${qs}` : pathname);
    }, 300);
    return () => clearTimeout(handler);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [searchDraft]);

  // Debounce tag input similarly.
  useEffect(() => {
    const handler = setTimeout(() => {
      const params = new URLSearchParams(searchParams.toString());
      if (tagDraft) {
        params.set("tag", tagDraft);
      } else {
        params.delete("tag");
      }
      params.delete("page");
      const qs = params.toString();
      router.push(qs ? `${pathname}?${qs}` : pathname);
    }, 300);
    return () => clearTimeout(handler);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [tagDraft]);

  const buildStatusHref = (next: StatusValue): string => {
    const params = new URLSearchParams();
    if (next) params.set("status", next);
    if (searchDraft) params.set("search", searchDraft);
    if (tagDraft) params.set("tag", tagDraft);
    const qs = params.toString();
    return qs ? `${pathname}?${qs}` : pathname;
  };

  return (
    <div className="flex flex-col gap-5">
      <label className="flex items-center gap-3 border-b border-border-subtle pb-2">
        <span className="sr-only">Search customers</span>
        <input
          type="search"
          value={searchDraft}
          onChange={(e) => setSearchDraft(e.target.value)}
          placeholder="Search customers by name or email…"
          aria-label="Search customers"
          className="w-full border-none bg-transparent py-2 text-sm text-foreground placeholder:text-foreground-tertiary focus:outline-none"
        />
        {searchDraft && (
          <button
            type="button"
            onClick={() => setSearchDraft("")}
            className="text-xs text-foreground-tertiary underline-offset-4 hover:text-foreground hover:underline"
          >
            Clear
          </button>
        )}
      </label>

      <div className="flex flex-wrap items-baseline gap-x-4 gap-y-1">
        <span className="shrink-0 text-[11px] font-semibold uppercase tracking-[0.16em] text-foreground-tertiary">
          Status
        </span>
        <div className="flex flex-wrap items-baseline gap-x-4 gap-y-1">
          {STATUS_OPTIONS.map((opt) => {
            const isCurrent = status === opt.value;
            return (
              <Link
                key={opt.label}
                href={buildStatusHref(opt.value)}
                aria-current={isCurrent ? "true" : undefined}
                className={
                  "text-xs transition-colors focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)] " +
                  (isCurrent
                    ? "text-foreground underline underline-offset-[6px] decoration-[1.5px]"
                    : "text-foreground-tertiary hover:text-foreground")
                }
              >
                {opt.label}
              </Link>
            );
          })}
        </div>

        <span className="mx-2 h-4 w-px shrink-0 bg-border-subtle" aria-hidden="true" />

        <label className="flex items-baseline gap-2">
          <span className="shrink-0 text-[11px] font-semibold uppercase tracking-[0.16em] text-foreground-tertiary">
            Tag
          </span>
          <input
            type="text"
            value={tagDraft}
            onChange={(e) => setTagDraft(e.target.value)}
            placeholder="any"
            className="w-32 border-b border-border-subtle bg-transparent py-0.5 text-xs text-foreground placeholder:text-foreground-tertiary focus:border-[color:var(--moss-700)] focus:outline-none"
          />
        </label>

        {hasFilters && (
          <Link
            href={pathname}
            className="ml-auto text-xs text-[color:var(--moss-700)] underline-offset-4 hover:underline"
          >
            Clear all
          </Link>
        )}
      </div>
    </div>
  );
}
