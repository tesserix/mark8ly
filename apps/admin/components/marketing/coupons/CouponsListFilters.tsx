"use client";

import { useRouter, useSearchParams } from "next/navigation";
import { useCallback, useState } from "react";

export function CouponsListFilters() {
  const router = useRouter();
  const params = useSearchParams();

  const [search, setSearch] = useState(params.get("search") ?? "");
  const status = params.get("status") ?? "";

  const applyFilters = useCallback(
    (newStatus?: string, newSearch?: string) => {
      const sp = new URLSearchParams();
      const s = newStatus ?? status;
      const q = newSearch ?? search;
      if (s) sp.set("status", s);
      if (q) sp.set("search", q);
      router.push(`/marketing/coupons?${sp.toString()}`);
    },
    [router, status, search],
  );

  return (
    <div className="flex flex-wrap items-center gap-3">
      <input
        type="text"
        placeholder="Search by code or title..."
        value={search}
        onChange={(e) => setSearch(e.target.value)}
        onKeyDown={(e) => {
          if (e.key === "Enter") applyFilters(undefined, search);
        }}
        className="rounded-md border border-ink-200 bg-white px-3 py-1.5 text-sm text-ink-900 placeholder:text-ink-400 focus:border-moss-700 focus:outline-none focus:ring-1 focus:ring-moss-700"
      />
      <select
        value={status}
        onChange={(e) => applyFilters(e.target.value)}
        className="rounded-md border border-ink-200 bg-white px-3 py-1.5 text-sm text-ink-900 focus:border-moss-700 focus:outline-none focus:ring-1 focus:ring-moss-700"
      >
        <option value="">All statuses</option>
        <option value="active">Active</option>
        <option value="disabled">Disabled</option>
        <option value="expired">Expired</option>
      </select>
    </div>
  );
}
