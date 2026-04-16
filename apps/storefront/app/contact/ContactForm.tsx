"use client";

import { useState, useTransition } from "react";
import { submitSupportTicket } from "./actions";

interface ContactFormProps {
  storeSlug: string;
  defaultEmail?: string;
  defaultName?: string;
}

type Status =
  | { kind: "idle" }
  | { kind: "submitting" }
  | { kind: "success"; ticketNumber: string }
  | { kind: "error"; message: string };

export function ContactForm({
  storeSlug,
  defaultEmail,
  defaultName,
}: ContactFormProps) {
  const [status, setStatus] = useState<Status>({ kind: "idle" });
  const [isPending, startTransition] = useTransition();

  function handleSubmit(e: React.FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const form = new FormData(e.currentTarget);
    const input = {
      storeSlug,
      name: String(form.get("name") ?? ""),
      email: String(form.get("email") ?? ""),
      subject: String(form.get("subject") ?? ""),
      description: String(form.get("description") ?? ""),
      priority: String(form.get("priority") ?? "medium") as
        | "low"
        | "medium"
        | "high",
    };

    setStatus({ kind: "submitting" });
    startTransition(async () => {
      const result = await submitSupportTicket(input);
      if (result.ok) {
        setStatus({ kind: "success", ticketNumber: result.ticket_number });
      } else {
        setStatus({ kind: "error", message: result.message });
      }
    });
  }

  if (status.kind === "success") {
    return (
      <div
        role="status"
        className="space-y-3 rounded-[6px] border border-border bg-[color:var(--background-elevated)] p-6"
      >
        <p className="font-serif text-2xl text-foreground">Thank you.</p>
        <p className="text-sm text-foreground-secondary">
          Your message has been received. Reference{" "}
          <span className="font-medium text-foreground">
            {status.ticketNumber}
          </span>
          . We&apos;ll get back to you at the email you provided.
        </p>
      </div>
    );
  }

  const disabled = isPending || status.kind === "submitting";

  return (
    <form onSubmit={handleSubmit} className="space-y-6">
      <div className="grid gap-6 sm:grid-cols-2">
        <div className="space-y-1.5">
          <label
            htmlFor="name"
            className="block text-sm font-medium text-foreground"
          >
            Name
          </label>
          <input
            id="name"
            name="name"
            type="text"
            autoComplete="name"
            required
            maxLength={200}
            defaultValue={defaultName}
            disabled={disabled}
            className="h-11 w-full rounded-[6px] border border-border bg-[color:var(--background-elevated)] px-3 text-sm text-foreground focus-visible:border-[color:var(--moss-700)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[color:var(--moss-700)]/20 disabled:opacity-60"
          />
        </div>
        <div className="space-y-1.5">
          <label
            htmlFor="email"
            className="block text-sm font-medium text-foreground"
          >
            Email
          </label>
          <input
            id="email"
            name="email"
            type="email"
            autoComplete="email"
            required
            maxLength={300}
            defaultValue={defaultEmail}
            disabled={disabled}
            className="h-11 w-full rounded-[6px] border border-border bg-[color:var(--background-elevated)] px-3 text-sm text-foreground focus-visible:border-[color:var(--moss-700)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[color:var(--moss-700)]/20 disabled:opacity-60"
          />
        </div>
      </div>

      <div className="space-y-1.5">
        <label
          htmlFor="subject"
          className="block text-sm font-medium text-foreground"
        >
          Subject
        </label>
        <input
          id="subject"
          name="subject"
          type="text"
          required
          maxLength={300}
          disabled={disabled}
          className="h-11 w-full rounded-[6px] border border-border bg-[color:var(--background-elevated)] px-3 text-sm text-foreground focus-visible:border-[color:var(--moss-700)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[color:var(--moss-700)]/20 disabled:opacity-60"
        />
      </div>

      <div className="space-y-1.5">
        <label
          htmlFor="priority"
          className="block text-sm font-medium text-foreground"
        >
          Priority
        </label>
        <select
          id="priority"
          name="priority"
          defaultValue="medium"
          disabled={disabled}
          className="h-11 w-full rounded-[6px] border border-border bg-[color:var(--background-elevated)] px-3 text-sm text-foreground focus-visible:border-[color:var(--moss-700)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[color:var(--moss-700)]/20 disabled:opacity-60"
        >
          <option value="low">Low — general enquiry</option>
          <option value="medium">Medium — standard request</option>
          <option value="high">High — urgent issue</option>
        </select>
      </div>

      <div className="space-y-1.5">
        <label
          htmlFor="description"
          className="block text-sm font-medium text-foreground"
        >
          Message
        </label>
        <textarea
          id="description"
          name="description"
          required
          maxLength={5000}
          rows={6}
          disabled={disabled}
          className="w-full rounded-[6px] border border-border bg-[color:var(--background-elevated)] px-3 py-2.5 text-sm text-foreground focus-visible:border-[color:var(--moss-700)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[color:var(--moss-700)]/20 disabled:opacity-60"
        />
      </div>

      {status.kind === "error" && (
        <p role="alert" className="text-sm text-[color:var(--signal)]">
          {status.message}
        </p>
      )}

      <button
        type="submit"
        disabled={disabled}
        className="h-11 rounded-[6px] bg-[color:var(--ink-900)] px-6 text-sm font-medium text-white transition-colors hover:bg-[color:var(--ink-900)]/90 disabled:opacity-60"
      >
        {disabled ? "Sending..." : "Send message"}
      </button>
    </form>
  );
}
