import Link from "next/link";
import { notFound } from "next/navigation";

import { getServerSessionContext } from "@/lib/auth/serverSession";
import {
  getMyPlatformTicket,
  type PlatformTicketReply,
} from "@/lib/api/tesserix";
import { PlatformTicketReplyForm } from "@/components/support/PlatformTicketReplyForm";

// Always render fresh — the merchant just sent a reply via server
// action and we want the new reply visible without a hard reload.
export const dynamic = "force-dynamic";

export default async function PlatformTicketDetailPage({
  params,
}: {
  params: Promise<{ id: string }>;
}) {
  const { id } = await params;
  const session = await getServerSessionContext();
  if (!session.tenantId) notFound();

  const data = await getMyPlatformTicket(session.tenantId, id);
  if (!data) notFound();

  const { ticket, replies } = data;
  const isClosed = ticket.status === "closed";
  const isResolved = ticket.status === "resolved";

  return (
    <div className="space-y-10">
      <div>
        <Link
          href="/support/platform"
          className="text-xs text-foreground-tertiary hover:text-foreground"
        >
          ← Back to platform support
        </Link>
      </div>

      <header className="space-y-3">
        <div className="flex flex-wrap items-center gap-x-3 gap-y-1">
          <span className="font-serif text-sm tabular-nums text-foreground">
            {ticket.ticket_number}
          </span>
          <StatusPill status={ticket.status} />
          <PriorityLabel priority={ticket.priority} />
        </div>
        <h1 className="font-serif text-3xl font-medium tracking-tight text-foreground sm:text-4xl">
          {ticket.subject}
        </h1>
        <p className="text-sm text-foreground-secondary">
          Filed {formatDate(ticket.created_at)} · Last activity{" "}
          {formatDate(ticket.updated_at)}
        </p>
      </header>

      <section className="border-y border-border-subtle py-6">
        <h2 className="text-[11px] font-semibold uppercase tracking-[0.16em] text-foreground-tertiary">
          Original message
        </h2>
        <p className="mt-3 whitespace-pre-wrap text-sm leading-relaxed text-foreground">
          {ticket.description}
        </p>
      </section>

      <section
        aria-label="Reply thread"
        aria-live="polite"
        aria-relevant="additions"
        className="space-y-4"
      >
        <h2 className="text-[11px] font-semibold uppercase tracking-[0.16em] text-foreground-tertiary">
          Replies ({replies.length})
        </h2>
        {replies.length === 0 ? (
          <p className="text-sm text-foreground-secondary">
            No replies yet. The Tesserix team will respond by email when they
            reply.
          </p>
        ) : (
          <ul className="space-y-4">
            {replies.map((reply) => (
              <li key={reply.id}>
                <ReplyCard reply={reply} />
              </li>
            ))}
          </ul>
        )}
      </section>

      {isClosed ? (
        <div className="rounded-md border border-border-subtle bg-[color:var(--ink-900)]/[0.03] p-4 text-sm text-foreground-secondary">
          This ticket is closed. If you need further help, please file a new
          platform support ticket.
        </div>
      ) : (
        <section aria-labelledby="composer-heading" className="space-y-3">
          <h2
            id="composer-heading"
            className="text-[11px] font-semibold uppercase tracking-[0.16em] text-foreground-tertiary"
          >
            Reply to platform team
          </h2>
          {isResolved && (
            <p className="text-xs text-foreground-tertiary">
              This ticket was marked resolved. Sending a reply will reopen it.
            </p>
          )}
          <PlatformTicketReplyForm ticketId={ticket.id} />
        </section>
      )}
    </div>
  );
}

function ReplyCard({ reply }: { reply: PlatformTicketReply }) {
  const isPlatform = reply.author_type === "platform_admin";
  return (
    <article
      className={`rounded-md border border-border-subtle p-4 ${isPlatform ? "bg-[color:var(--moss-700)]/[0.04]" : "bg-background-elevated"}`}
    >
      <header className="mb-2 flex items-center justify-between gap-2">
        <span
          className={`inline-flex items-center gap-1.5 rounded-full px-2 py-0.5 text-[11px] font-medium ${isPlatform ? "bg-[color:var(--moss-700)]/10 text-[color:var(--moss-700)]" : "bg-[color:var(--ink-900)]/[0.06] text-foreground-secondary"}`}
        >
          {isPlatform ? "Tesserix support" : reply.author_name || "You"}
        </span>
        <time
          dateTime={reply.created_at}
          className="text-xs tabular-nums text-foreground-tertiary"
        >
          {formatDate(reply.created_at)}
        </time>
      </header>
      <p className="whitespace-pre-wrap text-sm leading-relaxed text-foreground">
        {reply.content}
      </p>
    </article>
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
      {priority} priority
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

function formatDate(dateStr: string): string {
  return new Date(dateStr).toLocaleString("en-US", {
    year: "numeric",
    month: "short",
    day: "numeric",
    hour: "numeric",
    minute: "2-digit",
  });
}
