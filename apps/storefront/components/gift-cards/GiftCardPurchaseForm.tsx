"use client";

import { useCallback, useState } from "react";

interface GiftCardPurchaseFormProps {
  storeSlug: string;
  currency: string;
  origin: string;
}

const PRESET_AMOUNTS = [25, 50, 100, 200];

export function GiftCardPurchaseForm({
  storeSlug,
  currency,
  origin,
}: GiftCardPurchaseFormProps) {
  const [amount, setAmount] = useState<string>("50");
  const [purchaserName, setPurchaserName] = useState("");
  const [purchaserEmail, setPurchaserEmail] = useState("");
  const [recipientName, setRecipientName] = useState("");
  const [recipientEmail, setRecipientEmail] = useState("");
  const [senderName, setSenderName] = useState("");
  const [message, setMessage] = useState("");
  const [expiresAt, setExpiresAt] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const onSubmit = useCallback(
    async (e: React.FormEvent<HTMLFormElement>) => {
      e.preventDefault();
      if (submitting) return;
      setSubmitting(true);
      setError(null);

      // Success/cancel URLs absolute so Stripe can redirect back.
      const successURL = `${origin}/gift-cards/purchase/success?session_id={CHECKOUT_SESSION_ID}`;
      const cancelURL = `${origin}/gift-cards/purchase?cancelled=1`;

      const body = {
        amount,
        purchaser_name: purchaserName.trim() || undefined,
        purchaser_email: purchaserEmail.trim(),
        recipient_name: recipientName.trim() || undefined,
        recipient_email: recipientEmail.trim(),
        sender_name: senderName.trim() || undefined,
        message: message.trim() || undefined,
        expires_at: expiresAt || undefined,
        success_url: successURL,
        cancel_url: cancelURL,
      };

      try {
        const qs = new URLSearchParams({ store: storeSlug }).toString();
        const res = await fetch(`/api/gift-cards/purchase?${qs}`, {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify(body),
        });
        if (!res.ok) {
          const err = await res.json().catch(() => null);
          setError(err?.message ?? "Could not start the purchase. Please try again.");
          setSubmitting(false);
          return;
        }
        const json = await res.json();
        const url = json?.data?.checkout_url;
        if (!url) {
          setError("Payment provider returned no checkout URL.");
          setSubmitting(false);
          return;
        }
        // Redirect the customer to Stripe Checkout.
        window.location.href = url;
      } catch (err: unknown) {
        setError(err instanceof Error ? err.message : "Unexpected error.");
        setSubmitting(false);
      }
    },
    [
      amount,
      purchaserName,
      purchaserEmail,
      recipientName,
      recipientEmail,
      senderName,
      message,
      expiresAt,
      origin,
      storeSlug,
      submitting,
    ],
  );

  return (
    <form onSubmit={onSubmit} className="space-y-8">
      {error && (
        <p
          role="alert"
          aria-live="polite"
          className="rounded-md bg-[color:var(--signal)]/5 px-4 py-3 text-sm text-[color:var(--signal)]"
        >
          {error}
        </p>
      )}

      {/* Amount */}
      <section className="space-y-3">
        <h2 className="text-xs font-medium uppercase tracking-[0.16em] text-[color:var(--storefront-text,var(--ink-900))]/60">
          Amount
        </h2>
        <div className="flex flex-wrap gap-2">
          {PRESET_AMOUNTS.map((preset) => (
            <button
              key={preset}
              type="button"
              onClick={() => setAmount(String(preset))}
              className={`rounded-md border px-4 py-2 text-sm font-medium transition ${
                amount === String(preset)
                  ? "border-[color:var(--storefront-text,var(--ink-900))] bg-[color:var(--storefront-text,var(--ink-900))] text-[color:var(--storefront-background,var(--paper-200))]"
                  : "border-[color:var(--storefront-text,var(--ink-900))]/20 bg-[color:var(--storefront-surface,#ffffff)] text-[color:var(--storefront-text,var(--ink-900))] hover:border-[color:var(--storefront-text,var(--ink-900))]/40"
              }`}
            >
              {preset} {currency}
            </button>
          ))}
        </div>
        <div className="flex items-stretch gap-2">
          <input
            type="number"
            step="0.01"
            min="1"
            value={amount}
            onChange={(e) => setAmount(e.target.value)}
            required
            placeholder="50.00"
            className="flex-1 rounded-md border border-[color:var(--storefront-text,var(--ink-900))]/20 bg-[color:var(--storefront-surface,#ffffff)] px-4 py-2.5 text-sm text-[color:var(--storefront-text,var(--ink-900))] placeholder:text-[color:var(--storefront-text,var(--ink-900))]/40 focus:border-[color:var(--storefront-accent,var(--moss-700))] focus:outline-none focus:ring-1 focus:ring-[color:var(--storefront-accent,var(--moss-700))]"
          />
          <span className="inline-flex items-center rounded-md border border-[color:var(--storefront-text,var(--ink-900))]/20 bg-[color:var(--storefront-background,var(--paper-200))] px-3 font-mono text-sm font-medium text-[color:var(--storefront-text,var(--ink-900))]/80">
            {currency}
          </span>
        </div>
      </section>

      <hr className="border-[color:var(--storefront-text,var(--ink-900))]/20" />

      {/* Purchaser */}
      <section className="space-y-3">
        <h2 className="text-xs font-medium uppercase tracking-[0.16em] text-[color:var(--storefront-text,var(--ink-900))]/60">
          Your details
        </h2>
        <div className="grid gap-4 sm:grid-cols-2">
          <label className="block">
            <span className="text-sm font-medium text-[color:var(--storefront-text,var(--ink-900))]">Your name</span>
            <input
              type="text"
              value={purchaserName}
              onChange={(e) => setPurchaserName(e.target.value)}
              placeholder="For your receipt"
              className="mt-1 block w-full rounded-md border border-[color:var(--storefront-text,var(--ink-900))]/20 bg-[color:var(--storefront-surface,#ffffff)] px-3 py-2.5 text-sm text-[color:var(--storefront-text,var(--ink-900))] focus:border-[color:var(--storefront-accent,var(--moss-700))] focus:outline-none focus:ring-1 focus:ring-[color:var(--storefront-accent,var(--moss-700))]"
            />
          </label>
          <label className="block">
            <span className="text-sm font-medium text-[color:var(--storefront-text,var(--ink-900))]">Your email *</span>
            <input
              type="email"
              value={purchaserEmail}
              onChange={(e) => setPurchaserEmail(e.target.value)}
              required
              placeholder="you@example.com"
              className="mt-1 block w-full rounded-md border border-[color:var(--storefront-text,var(--ink-900))]/20 bg-[color:var(--storefront-surface,#ffffff)] px-3 py-2.5 text-sm text-[color:var(--storefront-text,var(--ink-900))] focus:border-[color:var(--storefront-accent,var(--moss-700))] focus:outline-none focus:ring-1 focus:ring-[color:var(--storefront-accent,var(--moss-700))]"
            />
          </label>
        </div>
      </section>

      {/* Recipient */}
      <section className="space-y-3">
        <h2 className="text-xs font-medium uppercase tracking-[0.16em] text-[color:var(--storefront-text,var(--ink-900))]/60">
          Recipient
        </h2>
        <div className="grid gap-4 sm:grid-cols-2">
          <label className="block">
            <span className="text-sm font-medium text-[color:var(--storefront-text,var(--ink-900))]">
              Recipient name
            </span>
            <input
              type="text"
              value={recipientName}
              onChange={(e) => setRecipientName(e.target.value)}
              placeholder="Optional — shown in the email"
              className="mt-1 block w-full rounded-md border border-[color:var(--storefront-text,var(--ink-900))]/20 bg-[color:var(--storefront-surface,#ffffff)] px-3 py-2.5 text-sm text-[color:var(--storefront-text,var(--ink-900))] focus:border-[color:var(--storefront-accent,var(--moss-700))] focus:outline-none focus:ring-1 focus:ring-[color:var(--storefront-accent,var(--moss-700))]"
            />
          </label>
          <label className="block">
            <span className="text-sm font-medium text-[color:var(--storefront-text,var(--ink-900))]">
              Recipient email *
            </span>
            <input
              type="email"
              value={recipientEmail}
              onChange={(e) => setRecipientEmail(e.target.value)}
              required
              placeholder="gift-recipient@example.com"
              className="mt-1 block w-full rounded-md border border-[color:var(--storefront-text,var(--ink-900))]/20 bg-[color:var(--storefront-surface,#ffffff)] px-3 py-2.5 text-sm text-[color:var(--storefront-text,var(--ink-900))] focus:border-[color:var(--storefront-accent,var(--moss-700))] focus:outline-none focus:ring-1 focus:ring-[color:var(--storefront-accent,var(--moss-700))]"
            />
          </label>
        </div>
        <label className="block">
          <span className="text-sm font-medium text-[color:var(--storefront-text,var(--ink-900))]">
            Your name as sender
          </span>
          <input
            type="text"
            value={senderName}
            onChange={(e) => setSenderName(e.target.value)}
            placeholder='Appears as "From …" in the email'
            className="mt-1 block w-full rounded-md border border-[color:var(--storefront-text,var(--ink-900))]/20 bg-[color:var(--storefront-surface,#ffffff)] px-3 py-2.5 text-sm text-[color:var(--storefront-text,var(--ink-900))] focus:border-[color:var(--storefront-accent,var(--moss-700))] focus:outline-none focus:ring-1 focus:ring-[color:var(--storefront-accent,var(--moss-700))]"
          />
        </label>
        <label className="block">
          <span className="text-sm font-medium text-[color:var(--storefront-text,var(--ink-900))]">
            Personal message (optional)
          </span>
          <textarea
            value={message}
            onChange={(e) => setMessage(e.target.value)}
            rows={3}
            maxLength={500}
            placeholder="Write a short note for the recipient"
            className="mt-1 block w-full rounded-md border border-[color:var(--storefront-text,var(--ink-900))]/20 bg-[color:var(--storefront-surface,#ffffff)] px-3 py-2.5 text-sm text-[color:var(--storefront-text,var(--ink-900))] focus:border-[color:var(--storefront-accent,var(--moss-700))] focus:outline-none focus:ring-1 focus:ring-[color:var(--storefront-accent,var(--moss-700))]"
          />
        </label>
      </section>

      <hr className="border-[color:var(--storefront-text,var(--ink-900))]/20" />

      {/* Expiry */}
      <section className="space-y-3">
        <h2 className="text-xs font-medium uppercase tracking-[0.16em] text-[color:var(--storefront-text,var(--ink-900))]/60">
          Expiry (optional)
        </h2>
        <label className="block max-w-xs">
          <input
            type="date"
            value={expiresAt}
            onChange={(e) => setExpiresAt(e.target.value)}
            className="mt-1 block w-full rounded-md border border-[color:var(--storefront-text,var(--ink-900))]/20 bg-[color:var(--storefront-surface,#ffffff)] px-3 py-2.5 text-sm text-[color:var(--storefront-text,var(--ink-900))] focus:border-[color:var(--storefront-accent,var(--moss-700))] focus:outline-none focus:ring-1 focus:ring-[color:var(--storefront-accent,var(--moss-700))]"
          />
        </label>
      </section>

      {/* Submit */}
      <div className="pt-2">
        <button
          type="submit"
          disabled={submitting}
          className="inline-flex items-center justify-center rounded-md bg-[color:var(--storefront-text,var(--ink-900))] px-6 py-3 text-sm font-medium text-[color:var(--storefront-background,var(--paper-200))] transition hover:opacity-90 disabled:cursor-not-allowed disabled:opacity-50"
        >
          {submitting ? "Redirecting..." : "Continue to payment"}
        </button>
        <p className="mt-3 text-xs text-[color:var(--storefront-text,var(--ink-900))]/60">
          Payment is handled securely by Stripe. You'll be redirected to
          complete checkout.
        </p>
      </div>
    </form>
  );
}
