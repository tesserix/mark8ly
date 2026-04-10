"use client";

import { useState, useRef, useCallback, useEffect } from "react";
import Link from "next/link";
import { useRouter, useSearchParams } from "next/navigation";
import { Search } from "lucide-react";

import type {
  AdminTicket,
  TicketStatus,
  TicketPriority,
} from "@/lib/api/marketplace-api";

interface TicketsListProps {
  tickets: AdminTicket[];
  counts: { open: number; resolved: number; closed: number };
  activeStatus: TicketStatus;
}

const tabs: { status: TicketStatus; label: string }[] = [
  { status: "open", label: "Open" },
  { status: "resolved", label: "Resolved" },
  { status: "closed", label: "Closed" },
];

function statusBadgeClass(status: TicketStatus): string {
  switch (status) {
    case "open":
      return "bg-[color:var(--moss-700)]/10 text-[color:var(--moss-700)]";
    case "resolved":
      return "bg-[color:var(--moss-700)] text-white";
    case "closed":
      return "bg-[color:var(--ink-900)]/10 text-[color:var(--ink-900)]/60";
  }
}

function priorityBadgeClass(priority: TicketPriority): string {
  switch (priority) {
    case "low":
      return "text-[color:var(--ink-900)]/40";
    case "medium":
      return "text-[color:var(--ink-900)]/60";
    case "high":
      return "text-[color:var(--signal)]";
  }
}

function timeAgo(dateStr: string): string {
  const diff = Date.now() - new Date(dateStr).getTime();
  const mins = Math.floor(diff / 60_000);
  if (mins < 1) return "just now";
  if (mins < 60) return `${mins}m ago`;
  const hrs = Math.floor(mins / 60);
  if (hrs < 24) return `${hrs}h ago`;
  const days = Math.floor(hrs / 24);
  return `${days}d ago`;
}

export function TicketsList({
  tickets,
  counts,
  activeStatus,
}: TicketsListProps) {
  const router = useRouter();
  const searchParams = useSearchParams();
  const [search, setSearch] = useState(searchParams.get("search") ?? "");
  const debounceRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  function handleTabChange(status: TicketStatus) {
    const params = new URLSearchParams();
    params.set("status", status);
    if (search) params.set("search", search);
    router.push(`/support/tickets?${params.toString()}`);
  }

  const pushSearch = useCallback(
    (value: string) => {
      const params = new URLSearchParams();
      params.set("status", activeStatus);
      if (value) params.set("search", value);
      router.push(`/support/tickets?${params.toString()}`);
    },
    [activeStatus, router],
  );

  function handleSearchChange(value: string) {
    setSearch(value);
    if (debounceRef.current) clearTimeout(debounceRef.current);
    debounceRef.current = setTimeout(() => pushSearch(value), 300);
  }

  useEffect(() => {
    return () => {
      if (debounceRef.current) clearTimeout(debounceRef.current);
    };
  }, []);

  function handleSearch(e: React.FormEvent) {
    e.preventDefault();
    if (debounceRef.current) clearTimeout(debounceRef.current);
    pushSearch(search);
  }

  return (
    <div className="space-y-6">
      {/* Tabs */}
      <div className="flex items-center gap-6 border-b border-border-subtle">
        {tabs.map((tab) => {
          const count = counts[tab.status];
          const isActive = tab.status === activeStatus;
          return (
            <button
              key={tab.status}
              type="button"
              onClick={() => handleTabChange(tab.status)}
              className={`relative min-h-[44px] pb-3 text-sm font-medium transition-colors ${
                isActive
                  ? "text-foreground"
                  : "text-foreground-secondary hover:text-foreground"
              }`}
              aria-current={isActive ? "page" : undefined}
            >
              {tab.label} ({count})
              {isActive && (
                <span className="absolute bottom-0 left-0 right-0 h-0.5 bg-[color:var(--moss-700)]" />
              )}
            </button>
          );
        })}
      </div>

      {/* Search */}
      <form onSubmit={handleSearch} className="relative w-full sm:w-auto">
        <Search
          className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-foreground-tertiary"
          aria-hidden="true"
        />
        <input
          type="search"
          value={search}
          onChange={(e) => handleSearchChange(e.target.value)}
          placeholder="Search tickets..."
          className="h-11 w-full rounded-md border border-border bg-background pl-10 pr-4 text-sm text-foreground placeholder:text-foreground-tertiary focus:border-[color:var(--moss-700)] focus:outline-none focus:ring-1 focus:ring-[color:var(--moss-700)] sm:w-80"
          aria-label="Search tickets"
        />
      </form>

      {/* Table */}
      {tickets.length === 0 ? (
        <div className="py-10 text-center">
          <p className="text-sm text-foreground-secondary">
            No {activeStatus} tickets found.
          </p>
        </div>
      ) : (
        <div className="overflow-x-auto">
        <ul className="divide-y divide-border-subtle" role="list">
          {tickets.map((ticket) => (
            <li key={ticket.id}>
              <Link
                href={`/support/tickets/${ticket.id}`}
                className="flex min-h-[44px] items-center justify-between gap-4 py-4 transition-colors hover:bg-paper-100"
              >
                <div className="min-w-0 flex-1 space-y-1">
                  <div className="flex items-center gap-3">
                    <span
                      className="font-mono text-sm text-foreground-secondary"
                      style={{ fontFeatureSettings: '"tnum" 1' }}
                    >
                      {ticket.ticket_number}
                    </span>
                    <span
                      className={`inline-flex items-center rounded-full px-2 py-0.5 text-[11px] font-medium ${statusBadgeClass(ticket.status)}`}
                    >
                      {ticket.status}
                    </span>
                    <span
                      className={`text-[11px] font-medium uppercase ${priorityBadgeClass(ticket.priority)}`}
                    >
                      {ticket.priority}
                    </span>
                  </div>
                  <p className="truncate text-sm font-medium text-foreground">
                    {ticket.subject}
                  </p>
                </div>
                <p className="shrink-0 text-xs text-foreground-tertiary">
                  {timeAgo(ticket.created_at)}
                </p>
              </Link>
            </li>
          ))}
        </ul>
        </div>
      )}
    </div>
  );
}
