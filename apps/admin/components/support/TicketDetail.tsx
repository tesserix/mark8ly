"use client";

import type {
  AdminTicket,
  TicketStatus,
  TicketPriority,
} from "@/lib/api/marketplace-api";
import { TicketReplyForm } from "./TicketReplyForm";
import { TicketStatusActions } from "./TicketStatusActions";

interface TicketDetailProps {
  ticket: AdminTicket;
}

function statusBadgeClass(status: TicketStatus): string {
  switch (status) {
    case "open":
      return "bg-[color:var(--moss-700)]/10 text-[color:var(--moss-700)]";
    case "in_progress":
      return "bg-amber-100 text-amber-900";
    case "resolved":
      return "bg-[color:var(--moss-700)] text-white";
    case "closed":
      return "bg-[color:var(--ink-900)]/10 text-[color:var(--ink-900)]/60";
  }
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
    <div className="space-y-8">
      {/* Header */}
      <div className="space-y-3">
        <div className="flex flex-wrap items-center gap-3">
          <span
            className="font-mono text-sm text-foreground-secondary"
            style={{ fontFeatureSettings: '"tnum" 1' }}
          >
            {ticket.ticket_number}
          </span>
          <span
            className={`inline-flex items-center rounded-full px-2.5 py-0.5 text-xs font-medium ${statusBadgeClass(ticket.status)}`}
          >
            {statusLabel(ticket.status)}
          </span>
          <span
            className={`text-xs font-medium uppercase ${priorityBadgeClass(ticket.priority)}`}
          >
            {ticket.priority} priority
          </span>
        </div>
        <h2 className="font-[family-name:var(--font-source-serif),'Source_Serif_4',serif] text-2xl font-medium text-foreground">
          {ticket.subject}
        </h2>
        <p className="text-sm text-foreground-secondary">
          Opened on {formatDate(ticket.created_at)} by {ticket.author_email}
        </p>
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

      {/* Status actions */}
      <TicketStatusActions ticketId={ticket.id} status={ticket.status} />

      {/* Reply form — only for non-closed tickets */}
      {ticket.status !== "closed" && (
        <TicketReplyForm ticketId={ticket.id} />
      )}
    </div>
  );
}
