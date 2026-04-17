"use client";

// Inline support-ticket form for the signed-in customer. Renders in
// both the header and footer slots of /account/layout.tsx. Name and
// email come from the session on the server; only subject + priority +
// message are captured here. On success the panel shows the new ticket
// reference and a link to the thread page.

import { useState, useTransition } from "react";
import Link from "next/link";
import { useRouter } from "next/navigation";

import { submitSupportTicket } from "@/app/contact/actions";

interface AccountSupportPanelProps {
  storeSlug: string;
  customerName: string;
  customerEmail: string;
  variant: "header" | "footer";
}

type Status =
  | { kind: "idle" }
  | { kind: "success"; ticketNumber: string }
  | { kind: "error"; message: string };

const HEADING: Record<AccountSupportPanelProps["variant"], string> = {
  header: "Need a hand?",
  footer: "Still have questions?",
};

const SUBHEADING: Record<AccountSupportPanelProps["variant"], string> = {
  header:
    "Send us a note and we'll get back to you at the email on your account.",
  footer:
    "If you need anything else, drop us a line — we typically respond within a business day.",
};

export function AccountSupportPanel({
  storeSlug,
  customerName,
  customerEmail,
  variant,
}: AccountSupportPanelProps) {
  const [status, setStatus] = useState<Status>({ kind: "idle" });
  const [isPending, startTransition] = useTransition();
  const router = useRouter();

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
        // Refresh so /account/tickets list picks up the new row on
        // the next navigation.
        router.refresh();
      } else {
        setStatus({ kind: "error", message: result.message });
      }
    });
  }

  const panelId = `account-support-${variant}`;

  return (
    <section
      aria-labelledby={`${panelId}-heading`}
      className="rounded-[6px] border border-[color:var(--storefront-text,var(--ink-900))]/10 bg-[color:var(--background-elevated,#fff)] p-5 sm:p-6"
    >
      <div className="mb-4 space-y-1">
        <h2
          id={`${panelId}-heading`}
          className="font-[family-name:var(--storefront-heading-font,var(--font-source-serif))] text-xl font-medium text-[color:var(--storefront-text,var(--ink-900))]"
        >
          {HEADING[variant]}
        </h2>
        <p className="text-sm text-[color:var(--storefront-text,var(--ink-900))] opacity-60">
          {SUBHEADING[variant]}
        </p>
      </div>

      {status.kind === "success" ? (
        <div
          role="status"
          className="space-y-2 rounded-[6px] border border-[color:var(--storefront-accent,var(--moss-700))]/30 bg-[color:var(--storefront-accent,var(--moss-700))]/5 px-4 py-3"
        >
          <p className="text-sm text-[color:var(--storefront-text,var(--ink-900))]">
            Thanks — we&apos;ve got your message.
          </p>
          <p className="text-xs text-[color:var(--storefront-text,var(--ink-900))] opacity-70">
            Reference{" "}
            <span className="font-medium">{status.ticketNumber}</span>.{" "}
            <Link
              href="/account/tickets"
              className="font-medium text-[color:var(--storefront-accent,var(--moss-700))] hover:underline"
            >
              View your tickets
            </Link>
            .
          </p>
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
                className="h-10 w-full rounded-[6px] border border-[color:var(--storefront-text,var(--ink-900))]/15 bg-white px-3 text-sm text-[color:var(--storefront-text,var(--ink-900))] focus-visible:border-[color:var(--storefront-accent,var(--moss-700))] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[color:var(--storefront-accent,var(--moss-700))]/20 disabled:opacity-60"
              />
            </div>
            <div className="space-y-1.5">
              <label
                htmlFor={`${panelId}-priority`}
                className="block text-sm font-medium text-[color:var(--storefront-text,var(--ink-900))]"
              >
                Priority
              </label>
              <select
                id={`${panelId}-priority`}
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
    </section>
  );
}
