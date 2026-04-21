"use client";

// Support section header with collapsible new-ticket form. Owns both
// the title row (Support + toggle) and the accordion panel below so
// the panel spans full width under the heading rather than floating
// beside it. Uses the storefront theme tokens (--storefront-*) so the
// block inherits the merchant's chosen palette, radius, and fonts
// instead of falling back to raw Paper/Ink/Moss defaults.

import { useId, useState, useTransition } from "react";
import { useRouter } from "next/navigation";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@tesserix/web";

import { submitSupportTicket } from "@/app/contact/actions";
import { toast } from "@/lib/toast";

type Priority = "low" | "medium" | "high";

const PRIORITY_OPTIONS: { value: Priority; label: string }[] = [
  { value: "low", label: "Low" },
  { value: "medium", label: "Medium" },
  { value: "high", label: "High" },
];

interface TicketsHeaderSectionProps {
  storeSlug: string;
  customerEmail: string;
  customerName?: string;
}

type Status =
  | { kind: "idle" }
  | { kind: "error"; message: string }
  | { kind: "success"; ticketNumber: string };

export function TicketsHeaderSection({
  storeSlug,
  customerEmail,
  customerName,
}: TicketsHeaderSectionProps) {
  const [open, setOpen] = useState(false);
  const [priority, setPriority] = useState<Priority>("medium");
  const [status, setStatus] = useState<Status>({ kind: "idle" });
  const [isPending, startTransition] = useTransition();
  const router = useRouter();
  const panelId = useId();

  function closePanel() {
    setOpen(false);
    setPriority("medium");
    setStatus({ kind: "idle" });
  }

  function handleSubmit(e: React.FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const form = new FormData(e.currentTarget);
    const subject = String(form.get("subject") ?? "").trim();
    const description = String(form.get("description") ?? "").trim();

    if (!subject || !description) {
      setStatus({
        kind: "error",
        message: "Subject and message are required.",
      });
      return;
    }

    setStatus({ kind: "idle" });
    startTransition(async () => {
      const result = await submitSupportTicket({
        storeSlug,
        name: customerName || customerEmail,
        email: customerEmail,
        subject,
        description,
        priority,
      });
      if (result.ok) {
        setStatus({ kind: "success", ticketNumber: result.ticket_number });
        toast({
          title: "Ticket created",
          description: `We'll get back to you at ${customerEmail}. Reference ${result.ticket_number}.`,
          tone: "success",
        });
        // Brief dwell on the success state, then collapse and pull a
        // fresh list so the new ticket appears at the top.
        setTimeout(() => {
          closePanel();
          router.refresh();
        }, 1400);
      } else {
        setStatus({ kind: "error", message: result.message });
        toast({
          title: "Couldn't send your ticket",
          description: result.message,
          tone: "error",
        });
      }
    });
  }

  return (
    <div className="space-y-4">
      <div className="flex items-baseline justify-between gap-4">
        <h1 className="font-[family-name:var(--storefront-heading-font,var(--font-source-serif))] text-2xl font-medium text-[color:var(--storefront-text,var(--ink-900))]">
          Support
        </h1>
        <button
          type="button"
          aria-expanded={open}
          aria-controls={panelId}
          onClick={() => (open ? closePanel() : setOpen(true))}
          className="inline-flex items-center gap-1.5 text-sm font-medium text-[color:var(--storefront-accent,var(--moss-700))] transition-opacity hover:opacity-80"
        >
          {open ? "Close" : "New ticket"}
          <svg
            aria-hidden="true"
            viewBox="0 0 16 16"
            className={`h-3.5 w-3.5 transition-transform duration-200 ${open ? "rotate-180" : ""}`}
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

      {/* CSS grid-rows trick: animates [0fr] → [1fr] so the accordion
          height transitions smoothly without measuring content. Inner
          div uses min-h-0 so grid can collapse it to zero. */}
      <div
        id={panelId}
        role="region"
        aria-label="New support ticket"
        className={`grid transition-[grid-template-rows] duration-200 ease-out ${open ? "grid-rows-[1fr]" : "grid-rows-[0fr]"}`}
      >
        <div className="min-h-0 overflow-hidden">
          <div
            className={`mb-2 rounded-[var(--storefront-radius,6px)] border border-[color:var(--storefront-text,var(--ink-900))]/10 bg-[color:var(--storefront-surface,#fff)] p-5 sm:p-6 transition-opacity duration-200 ${open ? "opacity-100" : "opacity-0"}`}
          >
            {status.kind === "success" ? (
              <div
                role="status"
                className="rounded-[var(--storefront-radius,6px)] border border-[color:var(--storefront-accent,var(--moss-700))]/30 bg-[color:var(--storefront-accent,var(--moss-700))]/5 px-4 py-3 text-sm text-[color:var(--storefront-text,var(--ink-900))]"
              >
                Thanks — ticket{" "}
                <span className="font-medium">{status.ticketNumber}</span>{" "}
                has been created.
              </div>
            ) : (
              <form onSubmit={handleSubmit} className="space-y-4">
                <div className="grid gap-4 sm:grid-cols-[2fr_1fr]">
                  <div className="space-y-1.5">
                    <label
                      htmlFor={`${panelId}-subject`}
                      className="block text-sm font-medium text-[color:var(--storefront-text,var(--ink-900))]"
                    >
                      Subject
                    </label>
                    <input
                      id={`${panelId}-subject`}
                      name="subject"
                      type="text"
                      required
                      maxLength={300}
                      disabled={isPending}
                      className="h-10 w-full rounded-[var(--storefront-radius,6px)] border border-[color:var(--storefront-text,var(--ink-900))]/15 bg-[color:var(--storefront-background,var(--paper-200))] px-3 text-sm text-[color:var(--storefront-text,var(--ink-900))] focus-visible:border-[color:var(--storefront-accent,var(--moss-700))] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[color:var(--storefront-accent,var(--moss-700))]/20 disabled:opacity-60"
                    />
                  </div>
                  <div className="space-y-1.5">
                    <label
                      htmlFor={`${panelId}-priority`}
                      className="block text-sm font-medium text-[color:var(--storefront-text,var(--ink-900))]"
                    >
                      Priority
                    </label>
                    <Select
                      value={priority}
                      onValueChange={(v) => setPriority(v as Priority)}
                      disabled={isPending}
                    >
                      <SelectTrigger
                        id={`${panelId}-priority`}
                        aria-label="Priority"
                        className="h-10 w-full rounded-[var(--storefront-radius,6px)] border border-[color:var(--storefront-text,var(--ink-900))]/15 bg-[color:var(--storefront-background,var(--paper-200))] px-3 text-sm text-[color:var(--storefront-text,var(--ink-900))]"
                      >
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        {PRIORITY_OPTIONS.map((o) => (
                          <SelectItem key={o.value} value={o.value}>
                            {o.label}
                          </SelectItem>
                        ))}
                      </SelectContent>
                    </Select>
                  </div>
                </div>

                <div className="space-y-1.5">
                  <label
                    htmlFor={`${panelId}-description`}
                    className="block text-sm font-medium text-[color:var(--storefront-text,var(--ink-900))]"
                  >
                    Message
                  </label>
                  <textarea
                    id={`${panelId}-description`}
                    name="description"
                    required
                    maxLength={5000}
                    rows={4}
                    disabled={isPending}
                    className="w-full rounded-[var(--storefront-radius,6px)] border border-[color:var(--storefront-text,var(--ink-900))]/15 bg-[color:var(--storefront-background,var(--paper-200))] px-3 py-2.5 text-sm text-[color:var(--storefront-text,var(--ink-900))] focus-visible:border-[color:var(--storefront-accent,var(--moss-700))] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[color:var(--storefront-accent,var(--moss-700))]/20 disabled:opacity-60"
                  />
                </div>

                {status.kind === "error" && (
                  <p
                    role="alert"
                    className="text-sm text-[color:var(--storefront-danger)]"
                  >
                    {status.message}
                  </p>
                )}

                <div className="flex flex-wrap items-center justify-between gap-4">
                  <p className="text-xs text-[color:var(--storefront-text,var(--ink-900))] opacity-60">
                    Replying as{" "}
                    <span className="font-medium">{customerEmail}</span>
                  </p>
                  <button
                    type="submit"
                    disabled={isPending}
                    className="inline-flex h-10 shrink-0 items-center rounded-[var(--storefront-radius,6px)] bg-[color:var(--storefront-primary,var(--ink-900))] px-5 text-sm font-medium text-[color:var(--storefront-on-primary,#fff)] transition-opacity duration-200 hover:opacity-90 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[color:var(--storefront-accent,var(--moss-700))] focus-visible:ring-offset-2 focus-visible:ring-offset-[color:var(--storefront-background,var(--paper-200))] disabled:opacity-60"
                  >
                    {isPending ? "Sending..." : "Send"}
                  </button>
                </div>
              </form>
            )}
          </div>
        </div>
      </div>
    </div>
  );
}
