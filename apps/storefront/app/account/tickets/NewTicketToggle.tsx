"use client";

// Collapsible "new ticket" form for the /account/tickets page. Shows
// a single "New ticket" button in its collapsed state; expands to the
// full form when clicked. On successful submission the form collapses
// and the page refreshes so the new ticket appears in the list below.

import { useState, useTransition } from "react";
import { useRouter } from "next/navigation";

import { submitSupportTicket } from "@/app/contact/actions";

interface NewTicketToggleProps {
  storeSlug: string;
  customerEmail: string;
  customerName?: string;
}

type Status =
  | { kind: "idle" }
  | { kind: "error"; message: string }
  | { kind: "success"; ticketNumber: string };

export function NewTicketToggle({
  storeSlug,
  customerEmail,
  customerName,
}: NewTicketToggleProps) {
  const [open, setOpen] = useState(false);
  const [status, setStatus] = useState<Status>({ kind: "idle" });
  const [isPending, startTransition] = useTransition();
  const router = useRouter();

  function resetAndClose() {
    setOpen(false);
    setStatus({ kind: "idle" });
  }

  function handleSubmit(e: React.FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const form = new FormData(e.currentTarget);
    const subject = String(form.get("subject") ?? "").trim();
    const description = String(form.get("description") ?? "").trim();
    const priority = String(form.get("priority") ?? "medium") as
      | "low"
      | "medium"
      | "high";

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
        // Pull fresh tickets into the list below. A short timeout keeps
        // the success confirmation visible just long enough to register
        // before the panel collapses.
        setTimeout(() => {
          setOpen(false);
          setStatus({ kind: "idle" });
          router.refresh();
        }, 1200);
      } else {
        setStatus({ kind: "error", message: result.message });
      }
    });
  }

  if (!open) {
    return (
      <button
        type="button"
        onClick={() => setOpen(true)}
        className="text-sm font-medium text-[color:var(--storefront-accent,var(--moss-700))] hover:underline"
      >
        New ticket
      </button>
    );
  }

  return (
    <div className="w-full rounded-[6px] border border-[color:var(--storefront-text,var(--ink-900))]/10 bg-[color:var(--background-elevated,#fff)] p-5">
      <div className="mb-4 flex items-baseline justify-between gap-4">
        <h2 className="font-[family-name:var(--storefront-heading-font,var(--font-source-serif))] text-lg font-medium text-[color:var(--storefront-text,var(--ink-900))]">
          New ticket
        </h2>
        <button
          type="button"
          onClick={resetAndClose}
          disabled={isPending}
          className="text-xs text-[color:var(--storefront-text,var(--ink-900))] opacity-60 hover:opacity-100 disabled:opacity-40"
        >
          Cancel
        </button>
      </div>

      {status.kind === "success" ? (
        <div
          role="status"
          className="rounded-[6px] border border-[color:var(--storefront-accent,var(--moss-700))]/30 bg-[color:var(--storefront-accent,var(--moss-700))]/5 px-4 py-3 text-sm text-[color:var(--storefront-text,var(--ink-900))]"
        >
          Thanks — ticket{" "}
          <span className="font-medium">{status.ticketNumber}</span> has been
          created. Refreshing your list...
        </div>
      ) : (
        <form onSubmit={handleSubmit} className="space-y-4">
          <div className="grid gap-4 sm:grid-cols-[2fr_1fr]">
            <div className="space-y-1.5">
              <label
                htmlFor="subject"
                className="block text-sm font-medium text-[color:var(--storefront-text,var(--ink-900))]"
              >
                Subject
              </label>
              <input
                id="subject"
                name="subject"
                type="text"
                required
                maxLength={300}
                disabled={isPending}
                className="h-10 w-full rounded-[6px] border border-[color:var(--storefront-text,var(--ink-900))]/15 bg-white px-3 text-sm text-[color:var(--storefront-text,var(--ink-900))] focus-visible:border-[color:var(--storefront-accent,var(--moss-700))] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[color:var(--storefront-accent,var(--moss-700))]/20 disabled:opacity-60"
              />
            </div>
            <div className="space-y-1.5">
              <label
                htmlFor="priority"
                className="block text-sm font-medium text-[color:var(--storefront-text,var(--ink-900))]"
              >
                Priority
              </label>
              <select
                id="priority"
                name="priority"
                defaultValue="medium"
                disabled={isPending}
                className="h-10 w-full rounded-[6px] border border-[color:var(--storefront-text,var(--ink-900))]/15 bg-white px-3 text-sm text-[color:var(--storefront-text,var(--ink-900))] focus-visible:border-[color:var(--storefront-accent,var(--moss-700))] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[color:var(--storefront-accent,var(--moss-700))]/20 disabled:opacity-60"
              >
                <option value="low">Low</option>
                <option value="medium">Medium</option>
                <option value="high">High</option>
              </select>
            </div>
          </div>

          <div className="space-y-1.5">
            <label
              htmlFor="description"
              className="block text-sm font-medium text-[color:var(--storefront-text,var(--ink-900))]"
            >
              Message
            </label>
            <textarea
              id="description"
              name="description"
              required
              maxLength={5000}
              rows={4}
              disabled={isPending}
              className="w-full rounded-[6px] border border-[color:var(--storefront-text,var(--ink-900))]/15 bg-white px-3 py-2.5 text-sm text-[color:var(--storefront-text,var(--ink-900))] focus-visible:border-[color:var(--storefront-accent,var(--moss-700))] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[color:var(--storefront-accent,var(--moss-700))]/20 disabled:opacity-60"
            />
          </div>

          {status.kind === "error" && (
            <p role="alert" className="text-sm text-[color:var(--signal)]">
              {status.message}
            </p>
          )}

          <div className="flex items-center justify-between gap-4">
            <p className="text-xs text-[color:var(--storefront-text,var(--ink-900))] opacity-50">
              Replying as{" "}
              <span className="font-medium">{customerEmail}</span>
            </p>
            <button
              type="submit"
              disabled={isPending}
              className="h-10 shrink-0 rounded-[6px] bg-[color:var(--ink-900)] px-5 text-sm font-medium text-white transition-colors hover:bg-[color:var(--ink-900)]/90 disabled:opacity-60"
            >
              {isPending ? "Sending..." : "Send"}
            </button>
          </div>
        </form>
      )}
    </div>
  );
}
