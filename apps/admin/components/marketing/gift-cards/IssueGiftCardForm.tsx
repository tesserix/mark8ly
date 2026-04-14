"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import { useToast } from "@/components/feedback/Toaster";

interface IssueGiftCardFormProps {
  currency: string;
}

export function IssueGiftCardForm({ currency }: IssueGiftCardFormProps) {
  const router = useRouter();
  const { toast } = useToast();
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  async function handleSubmit(e: React.FormEvent<HTMLFormElement>) {
    e.preventDefault();
    setSubmitting(true);
    setError(null);

    const form = new FormData(e.currentTarget);
    const body = {
      initial_balance: form.get("initial_balance") as string,
      // Currency is locked to the store's currency — never collected from
      // the form, so we always send the authoritative value.
      currency_code: currency,
      sender_name: (form.get("sender_name") as string) || undefined,
      sender_email: (form.get("sender_email") as string) || undefined,
      recipient_name: (form.get("recipient_name") as string) || undefined,
      recipient_email: (form.get("recipient_email") as string) || undefined,
      message: (form.get("message") as string) || undefined,
      expires_at: (form.get("expires_at") as string) || undefined,
    };

    try {
      const res = await fetch("/api/marketing/gift-cards", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(body),
      });

      if (!res.ok) {
        const err = await res.json().catch(() => ({ message: "Unknown error" }));
        const msg = err.message ?? "Failed to issue gift card";
        setError(msg);
        toast.error("Couldn't issue gift card", msg);
        setSubmitting(false);
        return;
      }

      const { data } = await res.json();
      toast.success("Gift card issued");
      router.push(`/marketing/gift-cards/${data.id}`);
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : "Unknown error";
      setError(msg);
      toast.error("Couldn't issue gift card", msg);
      setSubmitting(false);
    }
  }

  return (
    <form onSubmit={handleSubmit} className="flex max-w-xl flex-col gap-5">
      {error && (
        <div
          role="alert"
          aria-live="polite"
          className="rounded-md border border-[color:var(--signal)]/20 bg-[color:var(--signal)]/5 px-4 py-3 text-sm text-[color:var(--signal)]"
        >
          {error}
        </div>
      )}

      {/* Amount + Currency — currency is read-only, locked to the store */}
      <div className="flex flex-col gap-1.5">
        <label htmlFor="initial_balance" className="text-sm font-medium text-ink-900">
          Amount *
        </label>
        <div className="flex items-stretch gap-2">
          <input
            id="initial_balance"
            name="initial_balance"
            type="number"
            step="0.01"
            min="0.01"
            required
            placeholder="50.00"
            className="flex-1 rounded-md border border-ink-900/15 bg-white px-3 py-2.5 text-sm focus:border-moss-700 focus:outline-none focus:ring-1 focus:ring-moss-700"
          />
          <span
            aria-label={`Store currency: ${currency}`}
            className="inline-flex items-center rounded-md border border-ink-900/10 bg-ink-50 px-3 py-2.5 font-mono text-sm font-medium text-ink-700"
          >
            {currency}
          </span>
        </div>
        <p className="text-xs text-ink-500">
          Gift cards are issued in the store currency.
        </p>
      </div>

      <hr className="border-ink-900/10" />

      {/* Recipient */}
      <p className="text-xs font-medium uppercase tracking-wider text-ink-500">
        Recipient (optional)
      </p>
      <div className="grid grid-cols-2 gap-4">
        <div className="flex flex-col gap-1.5">
          <label htmlFor="recipient_name" className="text-sm font-medium text-ink-900">
            Name
          </label>
          <input
            id="recipient_name"
            name="recipient_name"
            type="text"
            className="rounded-md border border-ink-900/15 bg-white px-3 py-2.5 text-sm focus:border-moss-700 focus:outline-none focus:ring-1 focus:ring-moss-700"
          />
        </div>
        <div className="flex flex-col gap-1.5">
          <label htmlFor="recipient_email" className="text-sm font-medium text-ink-900">
            Email
          </label>
          <input
            id="recipient_email"
            name="recipient_email"
            type="email"
            className="rounded-md border border-ink-900/15 bg-white px-3 py-2.5 text-sm focus:border-moss-700 focus:outline-none focus:ring-1 focus:ring-moss-700"
          />
        </div>
      </div>

      {/* Sender */}
      <p className="text-xs font-medium uppercase tracking-wider text-ink-500">
        Sender (optional)
      </p>
      <div className="grid grid-cols-2 gap-4">
        <div className="flex flex-col gap-1.5">
          <label htmlFor="sender_name" className="text-sm font-medium text-ink-900">
            Name
          </label>
          <input
            id="sender_name"
            name="sender_name"
            type="text"
            className="rounded-md border border-ink-900/15 bg-white px-3 py-2.5 text-sm focus:border-moss-700 focus:outline-none focus:ring-1 focus:ring-moss-700"
          />
        </div>
        <div className="flex flex-col gap-1.5">
          <label htmlFor="sender_email" className="text-sm font-medium text-ink-900">
            Email
          </label>
          <input
            id="sender_email"
            name="sender_email"
            type="email"
            className="rounded-md border border-ink-900/15 bg-white px-3 py-2.5 text-sm focus:border-moss-700 focus:outline-none focus:ring-1 focus:ring-moss-700"
          />
        </div>
      </div>

      {/* Message */}
      <div className="flex flex-col gap-1.5">
        <label htmlFor="message" className="text-sm font-medium text-ink-900">
          Personal message (optional)
        </label>
        <textarea
          id="message"
          name="message"
          rows={3}
          maxLength={500}
          className="rounded-md border border-ink-900/15 bg-white px-3 py-2.5 text-sm focus:border-moss-700 focus:outline-none focus:ring-1 focus:ring-moss-700"
        />
      </div>

      {/* Expiry */}
      <div className="flex flex-col gap-1.5">
        <label htmlFor="expires_at" className="text-sm font-medium text-ink-900">
          Expiry date (optional)
        </label>
        <input
          id="expires_at"
          name="expires_at"
          type="date"
          className="w-48 rounded-md border border-ink-900/15 bg-white px-3 py-2.5 text-sm focus:border-moss-700 focus:outline-none focus:ring-1 focus:ring-moss-700"
        />
      </div>

      <div className="flex gap-3 pt-2">
        <button
          type="submit"
          disabled={submitting}
          className="inline-flex items-center gap-2 rounded-md bg-ink-900 px-5 py-2.5 text-sm font-medium text-paper-200 transition-colors hover:bg-moss-700 disabled:opacity-50"
        >
          {submitting ? "Issuing..." : "Issue Gift Card"}
        </button>
        <button
          type="button"
          onClick={() => router.back()}
          className="rounded-md border border-ink-900/15 px-5 py-2.5 text-sm font-medium text-ink-900 transition-colors hover:bg-ink-900/5"
        >
          Cancel
        </button>
      </div>
    </form>
  );
}
