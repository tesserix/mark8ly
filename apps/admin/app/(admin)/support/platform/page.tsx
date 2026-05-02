import { Suspense } from "react";
import Link from "next/link";

import { getServerSessionContext } from "@/lib/auth/serverSession";
import {
  listMyPlatformTickets,
  type PlatformTicket,
} from "@/lib/api/tesserix";
import { PlatformTicketForm } from "@/components/support/PlatformTicketForm";

// Always render fresh — the action's revalidatePath + router.refresh()
// chain repaints the list after a ticket is filed, and we want every
// poll/visit to see the live ticket state from tesserix-home rather
// than a stale RSC cache.
export const dynamic = "force-dynamic";

export default async function PlatformSupportPage() {
  const session = await getServerSessionContext();

  return (
    <div className="space-y-10">
      <header className="space-y-3">
        <p className="text-[11px] font-semibold uppercase tracking-[0.16em] text-foreground-tertiary">
          Support
        </p>
        <h1 className="font-serif text-5xl font-medium tracking-tight text-foreground">
          Platform support
        </h1>
        <p className="max-w-2xl text-base leading-7 text-foreground-secondary">
          File a ticket with the Tesserix platform team. Use this for
          billing, subdomain or DNS issues, account access, security
          concerns, or anything where the mark8ly app itself is the problem.
          For customer-facing support, use{" "}
          <a className="underline" href="/support/tickets">
            Support → Tickets
          </a>
          .
        </p>
      </header>

      <section className="space-y-5">
        <h2 className="font-serif text-2xl font-medium text-foreground">
          New platform ticket
        </h2>
        <PlatformTicketForm />
      </section>

      <section className="space-y-5">
        <h2 className="font-serif text-2xl font-medium text-foreground">
          Your platform tickets
        </h2>
        <Suspense fallback={<TicketListSkeleton />}>
          <TicketList tenantId={session.tenantId} />
        </Suspense>
      </section>
    </div>
  );
}

async function TicketList({ tenantId }: { tenantId: string }) {
  const tickets = tenantId ? await listMyPlatformTickets(tenantId) : [];
  if (tickets.length === 0) {
    return (
      <div className="border-t border-border-subtle py-10">
        <p className="text-sm text-foreground-secondary">
          You haven&apos;t filed any platform tickets yet.
        </p>
      </div>
    );
  }
  return (
    <ul role="list" className="flex flex-col">
      {tickets.map((ticket) => (
        <li key={ticket.id}>
          <TicketRow ticket={ticket} />
        </li>
      ))}
    </ul>
  );
}

function TicketRow({ ticket }: { ticket: PlatformTicket }) {
  return (
    <Link
      href={`/support/platform/${ticket.id}`}
      className="flex items-center justify-between gap-4 border-b border-border-subtle px-1 py-4 transition-colors hover:bg-[color:var(--ink-900)]/[0.03] focus-visible:outline-2 focus-visible:outline-offset-[-2px] focus-visible:outline-[color:var(--moss-700)]"
    >
      <div className="min-w-0 flex-1">
        <div className="flex flex-wrap items-center gap-x-3 gap-y-1">
          <span className="font-serif text-sm tabular-nums text-foreground">
            {ticket.ticket_number}
          </span>
          <StatusPill status={ticket.status} />
          <PriorityLabel priority={ticket.priority} />
        </div>
        <p className="mt-1 truncate text-sm text-foreground">{ticket.subject}</p>
      </div>
      <p className="shrink-0 text-xs tabular-nums text-foreground-tertiary">
        {timeAgo(ticket.updated_at)}
      </p>
    </Link>
  );
}

function StatusPill({ status }: { status: string }) {
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
      {labelForStatus(status)}
    </span>
  );
}

function PriorityLabel({ priority }: { priority: string }) {
  const tone =
    priority === "urgent"
      ? "text-[color:var(--danger)]"
      : priority === "high"
        ? "text-[color:var(--signal)]"
        : priority === "medium"
          ? "text-foreground-secondary"
          : "text-foreground-tertiary";
  return (
    <span
      className={`text-[11px] font-semibold uppercase tracking-[0.12em] ${tone}`}
    >
      {priority}
    </span>
  );
}

function labelForStatus(status: string): string {
  switch (status) {
    case "open":
      return "Open";
    case "in_progress":
      return "In progress";
    case "resolved":
      return "Resolved";
    case "closed":
      return "Closed";
    default:
      return status;
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

function TicketListSkeleton() {
  return (
    <div className="space-y-3">
      {[0, 1, 2].map((i) => (
        <div
          key={i}
          className="h-12 animate-pulse rounded bg-[color:var(--ink-900)]/5"
        />
      ))}
    </div>
  );
}
