"use client";

// Editorial filter bar for /products. URL is the source of truth —
// search is a debounced hairline input, status is a row of underline
// chips that compose with search. Navigating replaces the URL so the
// view is deep-linkable and survives hard reload.

import { useRouter, useSearchParams, usePathname } from "next/navigation";
import { useEffect, useState } from "react";
import Link from "next/link";

type StatusValue = "draft" | "active" | "archived" | undefined;

const STATUS_OPTIONS: { value: StatusValue; label: string }[] = [
  { value: undefined, label: "All" },
  { value: "active", label: "Active" },
  { value: "draft", label: "Draft" },
  { value: "archived", label: "Archived" },
];

export function ProductsListFilters() {
  const router = useRouter();
  const pathname = usePathname();
  const searchParams = useSearchParams();

  const [searchDraft, setSearchDraft] = useState(
    searchParams.get("search") ?? "",
  );
  const status = (searchParams.get("status") ?? undefined) as StatusValue;
  const hasFilters = !!status || !!searchDraft;

  // Debounce search → URL navigation. The URL is the source of truth so
  // a hard reload or shared link restores the same filtered view.
  //
  // The no-op guard is load-bearing (#1019). searchDraft is INITIALISED
  // from the URL, so on first render this effect computed the URL the
  // page is already on and pushed it anyway — every single view of
  // /products fired a redundant client navigation 300ms after mount.
  //
  // That is what made links on this page lose clicks: a user clicking
  // while that spurious push is in flight had their own navigation
  // dropped, silently and permanently. Measured — a plain <Link> on
  // /products is stuck 4/4 under CPU throttling while the identical link
  // on /dashboard, which mounts no filter bar, navigates 2/2.
  //
  // Comparing the target to the current URL also makes the full
  // dependency list safe: re-running on a searchParams change now
  // recomputes, sees nothing to do, and returns instead of pushing
  // again. That is why there is no exhaustive-deps suppression here
  // anymore (#1000).
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
      const target = qs ? `${pathname}?${qs}` : pathname;
      if (target === `${pathname}${window.location.search}`) return;
      router.push(target);
    }, 300);
    return () => clearTimeout(handler);
  }, [searchDraft, pathname, router, searchParams]);

  const buildStatusHref = (next: StatusValue): string => {
    const params = new URLSearchParams();
    if (next) params.set("status", next);
    if (searchDraft) params.set("search", searchDraft);
    const qs = params.toString();
    return qs ? `${pathname}?${qs}` : pathname;
  };

  return (
    <div className="flex flex-col gap-5">
      <label className="flex items-center gap-3 border-b border-border-subtle pb-2">
        <span className="sr-only">Search products</span>
        <input
          type="search"
          value={searchDraft}
          onChange={(e) => setSearchDraft(e.target.value)}
          placeholder="Search products…"
          aria-label="Search products"
          className="w-full border-none bg-transparent py-2 text-sm text-foreground placeholder:text-foreground-tertiary focus:outline-none"
        />
        {searchDraft && (
          <button
            type="button"
            onClick={() => setSearchDraft("")}
            className="text-xs text-foreground-tertiary underline-offset-4 hover:text-foreground hover:underline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)]"
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
