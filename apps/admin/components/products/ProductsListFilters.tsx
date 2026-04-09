"use client";

// apps/admin/components/products/ProductsListFilters.tsx
//
// Client component: debounced search input + status dropdown. URL is the
// source of truth — every change pushes a new navigation with updated
// search params. Resets to page 1 on any filter change.

import { useRouter, useSearchParams, usePathname } from "next/navigation";
import { useEffect, useState } from "react";
import { Search, SlidersHorizontal, X } from "lucide-react";

export function ProductsListFilters() {
  const router = useRouter();
  const pathname = usePathname();
  const searchParams = useSearchParams();

  const [searchDraft, setSearchDraft] = useState(
    searchParams.get("search") ?? "",
  );
  const status = searchParams.get("status") ?? "";
  const hasFilters = !!status || !!searchDraft;

  // Debounce search input → URL navigation
  useEffect(() => {
    const handler = setTimeout(() => {
      const params = new URLSearchParams(searchParams.toString());
      if (searchDraft) {
        params.set("search", searchDraft);
      } else {
        params.delete("search");
      }
      params.delete("page"); // reset to page 1 on new search
      const qs = params.toString();
      router.push(qs ? `${pathname}?${qs}` : pathname);
    }, 300);
    return () => clearTimeout(handler);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [searchDraft]);

  const setStatus = (next: string) => {
    const params = new URLSearchParams(searchParams.toString());
    if (next) {
      params.set("status", next);
    } else {
      params.delete("status");
    }
    params.delete("page");
    const qs = params.toString();
    router.push(qs ? `${pathname}?${qs}` : pathname);
  };

  const clearAll = () => {
    setSearchDraft("");
    router.push(pathname);
  };

  return (
    <div className="flex flex-wrap items-center gap-3">
      <label className="relative min-w-64 flex-1">
        <Search
          className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-[color:var(--ink-900)] opacity-50"
          aria-hidden="true"
        />
        <input
          type="search"
          value={searchDraft}
          onChange={(e) => setSearchDraft(e.target.value)}
          placeholder="Search products…"
          aria-label="Search products"
          className="w-full rounded-md border border-[color:var(--ink-900)] border-opacity-20 bg-[color:var(--background-elevated,white)] py-2 pl-10 pr-3 text-sm text-[color:var(--ink-900)] placeholder:opacity-50 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)]"
        />
      </label>
      <StatusFilterDropdown value={status} onChange={setStatus} />
      {hasFilters && (
        <button
          type="button"
          onClick={clearAll}
          className="inline-flex items-center gap-1 text-sm text-[color:var(--moss-700)] underline-offset-4 hover:underline focus-visible:underline"
        >
          <X className="h-3 w-3" aria-hidden="true" /> Clear
        </button>
      )}
    </div>
  );
}

function StatusFilterDropdown({
  value,
  onChange,
}: {
  value: string;
  onChange: (next: string) => void;
}) {
  // Small native select for M7a. M7b+ upgrades to @tesserix/web filter-panel.
  return (
    <label className="inline-flex items-center gap-2 text-sm text-[color:var(--ink-900)]">
      <SlidersHorizontal className="h-4 w-4 opacity-60" aria-hidden="true" />
      <select
        value={value}
        onChange={(e) => onChange(e.target.value)}
        aria-label="Filter by status"
        className="rounded-md border border-[color:var(--ink-900)] border-opacity-20 bg-[color:var(--background-elevated,white)] py-2 pl-3 pr-8 text-sm text-[color:var(--ink-900)] focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)]"
      >
        <option value="">All statuses</option>
        <option value="draft">Draft</option>
        <option value="active">Active</option>
        <option value="archived">Archived</option>
      </select>
    </label>
  );
}
