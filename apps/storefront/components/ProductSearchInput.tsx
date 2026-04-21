"use client";

// apps/storefront/components/ProductSearchInput.tsx
//
// URL-driven debounced search input for the catalogue page. Updates the
// `?search=` query param after a 300ms debounce so the page refetches
// the filtered list. Resets pagination on every change.

import { useRouter, useSearchParams, usePathname } from "next/navigation";
import { useEffect, useState } from "react";
import { Search, X } from "lucide-react";

export function ProductSearchInput() {
  const router = useRouter();
  const pathname = usePathname();
  const searchParams = useSearchParams();
  const [draft, setDraft] = useState(searchParams.get("search") ?? "");

  useEffect(() => {
    const handle = setTimeout(() => {
      const params = new URLSearchParams(searchParams.toString());
      if (draft) {
        params.set("search", draft);
      } else {
        params.delete("search");
      }
      params.delete("page");
      const qs = params.toString();
      router.push(qs ? `${pathname}?${qs}` : pathname);
    }, 300);
    return () => clearTimeout(handle);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [draft]);

  return (
    <label className="relative block w-full max-w-xs sm:max-w-md lg:max-w-lg">
      <Search
        className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-[color:var(--storefront-text,var(--ink-900))] opacity-50"
        aria-hidden="true"
      />
      <input
        type="search"
        value={draft}
        onChange={(e) => setDraft(e.target.value)}
        placeholder="Search the shop…"
        aria-label="Search products"
        className="w-full rounded-md border border-[color:var(--storefront-text,var(--ink-900))] border-opacity-20 bg-[color:var(--background-elevated,white)] py-2 pl-10 pr-9 text-sm text-[color:var(--storefront-text,var(--ink-900))] placeholder:opacity-50 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--storefront-accent,var(--moss-700))]"
      />
      {draft && (
        <button
          type="button"
          onClick={() => setDraft("")}
          aria-label="Clear search"
          className="absolute right-2 top-1/2 inline-flex h-6 w-6 -translate-y-1/2 items-center justify-center rounded-md text-[color:var(--storefront-text,var(--ink-900))] opacity-60 hover:opacity-100 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--storefront-accent,var(--moss-700))]"
        >
          <X className="h-3 w-3" aria-hidden="true" />
        </button>
      )}
    </label>
  );
}
