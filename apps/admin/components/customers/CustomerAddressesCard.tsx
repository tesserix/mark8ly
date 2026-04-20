"use client";

import { useMemo, useState } from "react";

import type { AdminCustomerAddress } from "@/lib/api/marketplace-api";

interface CustomerAddressesCardProps {
  addresses: AdminCustomerAddress[];
}

type FilterKey = "all" | "default" | string;

export function CustomerAddressesCard({ addresses }: CustomerAddressesCardProps) {
  const labels = useMemo(() => {
    const set = new Set<string>();
    for (const addr of addresses) {
      if (addr.label) set.add(addr.label);
    }
    return Array.from(set);
  }, [addresses]);

  const hasDefault = addresses.some((a) => a.is_default);
  const showChips = addresses.length > 1 && (hasDefault || labels.length > 0);

  const [filter, setFilter] = useState<FilterKey>("all");

  const visible = useMemo(() => {
    if (filter === "all") return addresses;
    if (filter === "default") return addresses.filter((a) => a.is_default);
    return addresses.filter((a) => a.label === filter);
  }, [addresses, filter]);

  return (
    <section aria-labelledby="addresses-heading" className="flex flex-col gap-4">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <h2
          id="addresses-heading"
          className="font-serif text-2xl font-medium text-foreground"
        >
          Addresses
        </h2>
        {showChips && (
          <div role="tablist" aria-label="Filter addresses" className="flex flex-wrap gap-1.5">
            <FilterChip active={filter === "all"} onClick={() => setFilter("all")}>
              All ({addresses.length})
            </FilterChip>
            {hasDefault && (
              <FilterChip active={filter === "default"} onClick={() => setFilter("default")}>
                Default
              </FilterChip>
            )}
            {labels.map((label) => (
              <FilterChip
                key={label}
                active={filter === label}
                onClick={() => setFilter(label)}
              >
                {label}
              </FilterChip>
            ))}
          </div>
        )}
      </div>

      {addresses.length === 0 ? (
        <p className="text-sm text-foreground-tertiary">No saved addresses.</p>
      ) : visible.length === 0 ? (
        <p className="text-sm text-foreground-tertiary">
          No addresses match this filter.
        </p>
      ) : (
        <ul className="flex flex-col gap-4">
          {visible.map((addr) => (
            <li
              key={addr.id}
              className="border-b border-border-subtle pb-4 last:border-b-0"
            >
              <div className="flex items-start justify-between gap-4">
                <div className="flex flex-col gap-1 text-sm text-foreground">
                  <span className="font-medium">{addr.name}</span>
                  <span className="text-foreground-secondary">{addr.line1}</span>
                  {addr.line2 && (
                    <span className="text-foreground-secondary">{addr.line2}</span>
                  )}
                  <span className="text-foreground-secondary">
                    {addr.city}
                    {addr.region ? `, ${addr.region}` : ""}{" "}
                    {addr.postal_code ?? ""}
                  </span>
                  <span className="text-foreground-tertiary">{addr.country_code}</span>
                  {addr.phone && (
                    <span className="text-foreground-tertiary">{addr.phone}</span>
                  )}
                </div>
                <div className="flex shrink-0 flex-wrap justify-end gap-2">
                  {addr.is_default && (
                    <span className="rounded-sm bg-[color:var(--accent-tint)] px-2 py-0.5 text-xs font-medium text-[color:var(--moss-700)]">
                      Default
                    </span>
                  )}
                  {addr.label && (
                    <span className="rounded-sm bg-[color:var(--ink-900)]/[0.06] px-2 py-0.5 text-xs text-foreground-secondary">
                      {addr.label}
                    </span>
                  )}
                </div>
              </div>
            </li>
          ))}
        </ul>
      )}
    </section>
  );
}

interface FilterChipProps {
  active: boolean;
  onClick: () => void;
  children: React.ReactNode;
}

function FilterChip({ active, onClick, children }: FilterChipProps) {
  return (
    <button
      type="button"
      role="tab"
      aria-selected={active}
      onClick={onClick}
      className={
        "rounded-sm px-2.5 py-1 text-xs font-medium transition-colors focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)] " +
        (active
          ? "bg-[color:var(--ink-900)] text-[color:var(--primary-foreground)]"
          : "bg-[color:var(--ink-900)]/[0.06] text-foreground-secondary hover:bg-[color:var(--ink-900)]/[0.1] hover:text-foreground")
      }
    >
      {children}
    </button>
  );
}
