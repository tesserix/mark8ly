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
  counts: { open: number; in_progress?: number; resolved: number; closed: number };
  activeStatus: TicketStatus;
}

const tabs: { status: TicketStatus; label: string }[] = [
  { status: "open", label: "Open" },
  { status: "in_progress", label: "In progress" },
  { status: "resolved", label: "Resolved" },
  { status: "closed", label: "Closed" },
];

// Dot + label status badge. Matches the orders/returns/marketing pattern
// so the visual language stays consistent across every admin list.
function StatusBadge({ status }: { status: TicketStatus }) {
  const dotClass =
    status === "open"
      ? "border border-[color:var(--ink-900)]/60 bg-transparent"
      : status === "in_progress"
        ? "bg-[color:var(--warning)]"
        : status === "resolved"
          ? "bg-[color:var(--moss-700)]"
          : "bg-[color:var(--ink-900)] opacity-40";
  return (
    <span className="inline-flex items-center gap-2 text-sm text-foreground">
      <span
        aria-hidden="true"
        className={`inline-block h-2 w-2 rounded-full ${dotClass}`}
      />
      {statusLabel(status)}
    </span>
  );
}

function statusLabel(status: TicketStatus): string {
  switch (status) {
    case "open":
      return "Open";
    case "in_progress":
      return "In progress";
    case "resolved":
      return "Resolved";
    case "closed":
      return "Closed";
  }
}

function priorityLabelClass(priority: TicketPriority): string {
  switch (priority) {
    case "low":
      return "text-foreground-tertiary";
    case "medium":
      return "text-foreground-secondary";
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
    <div className="flex flex-col gap-6">
      {/* Underline tabs — hairline baseline, ink indicator, tabular-nums
          count. Matches the orders/returns/marketing pattern. */}
      <nav
        aria-label="Filter tickets by status"
        className="flex flex-wrap items-center gap-x-1 gap-y-0 border-b border-border-subtle px-1"
      >
        {tabs.map((tab) => {
          const count = counts[tab.status] ?? 0;
          const isActive = tab.status === activeStatus;
          return (
            <button
              key={tab.status}
              type="button"
              onClick={() => handleTabChange(tab.status)}
              aria-current={isActive ? "page" : undefined}
              className={
                "flex shrink-0 items-baseline gap-1.5 border-b-2 px-2.5 py-2.5 text-[11px] font-semibold uppercase tracking-[0.08em] transition-colors focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)] " +
                (isActive
                  ? "border-[color:var(--ink-900)] text-foreground"
                  : "border-transparent text-foreground-tertiary hover:text-foreground")
              }
            >
              <span>{tab.label}</span>
              {count > 0 && (
                <span
                  className={
                    "text-[11px] tabular-nums " +
                    (isActive
                      ? "text-foreground-secondary"
                      : "text-foreground-tertiary")
                  }
                >
                  {count}
                </span>
              )}
            </button>
          );
        })}
      </nav>

      {/* Search — hairline input, matches orders/products/customers. */}
      <form onSubmit={handleSearch} className="flex items-center gap-3 border-b border-border-subtle pb-2">
        <Search
          className="h-4 w-4 shrink-0 text-foreground-tertiary"
          aria-hidden="true"
        />
        <input
          type="search"
          value={search}
          onChange={(e) => handleSearchChange(e.target.value)}
          placeholder="Search tickets…"
          className="w-full border-none bg-transparent py-2 text-sm text-foreground placeholder:text-foreground-tertiary focus:outline-none"
          aria-label="Search tickets"
        />
      </form>

      {/* Rows */}
      {tickets.length === 0 ? (
        <div className="border-t border-border-subtle py-12">
          <p className="font-serif text-xl font-medium text-foreground">
            No {activeStatus.replace("_", " ")} tickets
          </p>
          <p className="mt-2 max-w-prose text-sm text-foreground-secondary">
            When a customer opens a support ticket, it will appear here.
          </p>
        </div>
      ) : (
        <ul role="list" className="flex flex-col">
          {tickets.map((ticket) => (
            <li key={ticket.id}>
              <Link
                href={`/support/tickets/${ticket.id}`}
                className="flex items-center justify-between gap-4 border-b border-border-subtle px-4 py-4 transition-colors hover:bg-[color:var(--ink-900)]/[0.03] focus-visible:outline-2 focus-visible:outline-offset-[-2px] focus-visible:outline-[color:var(--moss-700)]"
              >
                <div className="min-w-0 flex-1 flex-col gap-1.5">
                  <div className="flex flex-wrap items-center gap-x-3 gap-y-1">
                    <span className="font-serif text-sm tabular-nums text-foreground">
                      {ticket.ticket_number}
                    </span>
                    <StatusBadge status={ticket.status} />
                    <span
                      className={`text-[11px] font-semibold uppercase tracking-[0.12em] ${priorityLabelClass(ticket.priority)}`}
                    >
                      {ticket.priority}
                    </span>
                  </div>
                  <p className="mt-1 truncate text-sm text-foreground">
                    {ticket.subject}
                  </p>
                </div>
                <p className="shrink-0 text-xs tabular-nums text-foreground-tertiary">
                  {timeAgo(ticket.created_at)}
                </p>
              </Link>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}
