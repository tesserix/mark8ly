"use client";

// TranscriptSection — surfaces the otto chat history that spawned the
// ticket when the customer expands "View original chat". Loaded lazily
// via a server action so the page paints fast even when otto is slow,
// and so unused conversation_id tickets don't pay the round-trip cost.
//
// We deliberately render a read-only view here. Reopening the live
// chat is the otto widget's job — this component just shows what was
// said, with the merchant/customer attribution preserved.

import { useState, useTransition } from "react";

import { fetchTicketTranscript, type TranscriptData } from "./actions";

interface TranscriptSectionProps {
  ticketId: string;
}

function formatDateTime(iso: string): string {
  try {
    return new Date(iso).toLocaleString("en-US", {
      year: "numeric",
      month: "short",
      day: "numeric",
      hour: "numeric",
      minute: "2-digit",
    });
  } catch {
    return iso;
  }
}

export function TranscriptSection({ ticketId }: TranscriptSectionProps) {
  const [transcript, setTranscript] = useState<TranscriptData | null>(null);
  const [expanded, setExpanded] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [isPending, startTransition] = useTransition();

  function toggle() {
    // Cheap collapse — never tear down the cached transcript, just hide
    // it. A subsequent re-expand is instant.
    if (expanded) {
      setExpanded(false);
      return;
    }
    setExpanded(true);
    if (transcript) {
      return;
    }
    setError(null);
    startTransition(async () => {
      const result = await fetchTicketTranscript(ticketId);
      if (!result.ok) {
        setError(result.message);
        // Collapse on hard failures so the empty drawer doesn't linger.
        // The "not_found" case is benign — could be a ticket opened
        // manually that never had a chat — so we still leave the row
        // hidden afterwards.
        setExpanded(false);
        return;
      }
      setTranscript(result.transcript);
    });
  }

  return (
    <section
      aria-label="Original chat transcript"
      className="space-y-3 border-t border-[color:var(--storefront-text,var(--ink-900))]/10 pt-6"
    >
      <div className="flex items-center justify-between gap-3">
        <div className="min-w-0">
          <p className="text-xs font-medium text-[color:var(--storefront-text,var(--ink-900))] opacity-60">
            From an Otto chat
          </p>
          <p className="text-xs text-[color:var(--storefront-text,var(--ink-900))] opacity-50">
            This ticket was created from a live chat conversation.
          </p>
        </div>
        <button
          type="button"
          aria-expanded={expanded}
          onClick={toggle}
          disabled={isPending}
          className="inline-flex h-9 shrink-0 items-center gap-1.5 text-xs font-medium text-[color:var(--storefront-accent,var(--moss-700))] transition-opacity hover:opacity-80 disabled:opacity-60"
        >
          {expanded
            ? isPending
              ? "Loading..."
              : "Hide transcript"
            : isPending
              ? "Loading..."
              : "View original chat"}
          <svg
            aria-hidden="true"
            viewBox="0 0 16 16"
            className={`h-3.5 w-3.5 transition-transform duration-200 ${expanded ? "rotate-180" : ""}`}
            fill="none"
            stroke="currentColor"
            strokeWidth="1.75"
            strokeLinecap="round"
            strokeLinejoin="round"
          >
            <path d="M4 6l4 4 4-4" />
          </svg>
        </button>
      </div>

      {error && (
        <p
          role="alert"
          className="text-xs text-[color:var(--storefront-danger)]"
        >
          {error}
        </p>
      )}

      {expanded && transcript && (
        <div className="space-y-4 rounded-[var(--storefront-radius,6px)] border border-[color:var(--storefront-text,var(--ink-900))]/10 bg-[color:var(--storefront-surface,white)] p-5">
          <p className="text-xs text-[color:var(--storefront-text,var(--ink-900))] opacity-60">
            Case <span className="font-medium">{transcript.case_id}</span>
            {" · "}
            started {formatDateTime(transcript.created_at)}
            {transcript.closed_at && (
              <> · closed {formatDateTime(transcript.closed_at)}</>
            )}
          </p>

          {transcript.messages.length === 0 ? (
            <p className="text-xs text-[color:var(--storefront-text,var(--ink-900))] opacity-50">
              No messages were recorded for this chat.
            </p>
          ) : (
            <ol className="space-y-4">
              {transcript.messages.map((m) => {
                const isCustomer = m.author_type === "customer";
                const isSystem = m.author_type === "system";
                return (
                  <li key={m.id} className="space-y-1">
                    <p className="text-[11px] font-medium uppercase tracking-wide text-[color:var(--storefront-text,var(--ink-900))] opacity-50">
                      {isSystem
                        ? "System"
                        : isCustomer
                          ? `${m.author_name || "You"} (you)`
                          : `${m.author_name || "Support"} (support)`}
                      <span className="ml-2 normal-case tracking-normal opacity-70">
                        {formatDateTime(m.created_at)}
                      </span>
                    </p>
                    <p
                      className={`whitespace-pre-wrap text-sm ${
                        isSystem
                          ? "italic text-[color:var(--storefront-text,var(--ink-900))] opacity-70"
                          : "text-[color:var(--storefront-text,var(--ink-900))]"
                      }`}
                    >
                      {m.content}
                    </p>
                  </li>
                );
              })}
            </ol>
          )}
        </div>
      )}
    </section>
  );
}
