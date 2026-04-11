"use client";

import { useRouter, useSearchParams } from "next/navigation";
import { useCallback, useState } from "react";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@tesserix/web";

const STATUS_ALL = "__all__";

export function CouponsListFilters() {
  const router = useRouter();
  const params = useSearchParams();

  const [search, setSearch] = useState(params.get("search") ?? "");
  const status = params.get("status") ?? "";

  const applyFilters = useCallback(
    (newStatus?: string, newSearch?: string) => {
      const sp = new URLSearchParams();
      const rawStatus = newStatus ?? status;
      const s = rawStatus === STATUS_ALL ? "" : rawStatus;
      const q = newSearch ?? search;
      if (s) sp.set("status", s);
      if (q) sp.set("search", q);
      router.push(`/marketing/coupons?${sp.toString()}`);
    },
    [router, status, search],
  );

  return (
    <div className="flex flex-wrap items-center gap-3">
      <div>
        <label htmlFor="coupon-search" className="sr-only">
          Search coupons
        </label>
        <input
          id="coupon-search"
          type="text"
          placeholder="Search by code or title..."
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === "Enter") applyFilters(undefined, search);
          }}
          className="rounded-md border border-[color:var(--ink-900)] border-opacity-20 bg-[color:var(--background-elevated,white)] px-3 py-2.5 text-sm text-[color:var(--ink-900)] placeholder:opacity-50 focus:border-[color:var(--moss-700)] focus:outline-none focus:ring-1 focus:ring-[color:var(--moss-700)]"
        />
      </div>
      <div>
        <Select
          value={status || STATUS_ALL}
          onValueChange={(next) => applyFilters(next)}
        >
          <SelectTrigger
            aria-label="Filter by status"
            className="min-w-[10rem]"
          >
            <SelectValue placeholder="All statuses" />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value={STATUS_ALL}>All statuses</SelectItem>
            <SelectItem value="active">Active</SelectItem>
            <SelectItem value="disabled">Disabled</SelectItem>
            <SelectItem value="expired">Expired</SelectItem>
          </SelectContent>
        </Select>
      </div>
    </div>
  );
}
