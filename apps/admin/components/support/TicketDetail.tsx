"use client";

import type {
  AdminTicket,
  TicketStatus,
  TicketPriority,
} from "@/lib/api/marketplace-api";
import { TicketReplyForm } from "./TicketReplyForm";
import { TicketStatusStepper } from "./TicketStatusStepper";

interface TicketDetailProps {
  ticket: AdminTicket;
}

// Dot + label status — matches TicketsList and the rest of admin.
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

function formatDate(dateStr: string): string {
  return new Date(dateStr).toLocaleDateString("en-US", {
    year: "numeric",
    month: "long",
    day: "numeric",
    hour: "numeric",
    minute: "2-digit",
  });
}

export function TicketDetail({ ticket }: TicketDetailProps) {
  return (
    <div className="flex flex-col gap-8">
      {/* Masthead — serif ticket number + subject, status + priority as
          editorial meta row. */}
      <header className="flex flex-col gap-3">
        <div className="flex flex-wrap items-center gap-x-3 gap-y-1">
          <span className="font-serif text-sm tabular-nums text-foreground">
            {ticket.ticket_number}
          </span>
          <StatusBadge status={ticket.status} />
          <span
            className={`text-[11px] font-semibold uppercase tracking-[0.12em] ${priorityLabelClass(ticket.priority)}`}
          >
            {ticket.priority} priority
          </span>
        </div>
        <h2 className="font-serif text-2xl font-medium tracking-tight text-foreground text-balance sm:text-3xl">
          {ticket.subject}
        </h2>
        <p className="text-sm text-foreground-secondary">
          Opened on {formatDate(ticket.created_at)} by {ticket.author_email}
        </p>
      </header>

      {/* Status stepper — hairline-framed, not a bordered card, so it
          matches the rest of the editorial chrome. */}
      <div className="border-y border-border-subtle py-5">
        <TicketStatusStepper ticketId={ticket.id} status={ticket.status} />
      </div>

      {/* Description */}
      <div className="border-b border-border-subtle pb-6">
        <p className="whitespace-pre-wrap text-sm leading-relaxed text-foreground">
          {ticket.description}
        </p>
      </div>

      {/* Reply thread — tolerate an undefined array so older backend
          responses (pre-dto fix) don't crash the whole detail page. */}
      {(ticket.replies?.length ?? 0) > 0 && (
        <div className="space-y-4">
          <h3 className="text-sm font-semibold text-foreground-tertiary uppercase tracking-wide">
            Replies
          </h3>
          <ul className="space-y-4">
            {(ticket.replies ?? []).map((reply) => {
              const isMerchant = reply.author_type === "merchant";
              const isCustomer = reply.author_type === "customer";
              const badge = isCustomer ? "Customer" : isMerchant ? null : "Support";
              return (
                <li
                  key={reply.id}
                  className={`px-4 py-3 ${
                    isMerchant
                      ? "border-b border-border-subtle mr-8"
                      : "bg-[color:var(--moss-700)]/5 border-l-2 border-l-[color:var(--moss-700)] ml-8"
                  }`}
                >
                  <div className="flex items-center justify-between gap-2">
                    <p className="text-xs font-medium text-foreground-secondary">
                      {reply.author_name || reply.author_email || "Unknown"}
                      {badge && (
                        <span className="ml-2 text-[color:var(--moss-700)]">
                          {badge}
                        </span>
                      )}
                    </p>
                    <time className="text-xs text-foreground-tertiary">
                      {formatDate(reply.created_at)}
                    </time>
                  </div>
                  <p className="mt-2 whitespace-pre-wrap text-sm leading-relaxed text-foreground">
                    {reply.content}
                  </p>
                </li>
              );
            })}
          </ul>
        </div>
      )}

      {/* Reply form — only for non-closed tickets */}
      {ticket.status !== "closed" && (
        <TicketReplyForm ticketId={ticket.id} />
      )}
    </div>
  );
}
