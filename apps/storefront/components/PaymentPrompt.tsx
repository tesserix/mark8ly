"use client";

// Client island rendered inside the (server) /orders/[id] page. When
// the order is still pending and the storefront stashed a payment
// context for it on the checkout page, this component:
//
//   1. Loads the provider's checkout SDK on demand.
//   2. On "Pay now", opens the payment sheet keyed to the pre-created
//      provider-side payment token stashed at checkout.
//   3. Confirms the outcome SERVER-SIDE before treating the order as paid —
//      Razorpay by re-deriving the checkout HMAC over the returned triplet,
//      over the returned triplet. Both
//      reuse the same handler the webhook flow uses, so this works even when
//      no provider webhook reaches the cluster.
//   4. Clears the local sessionStorage context and refreshes the page.

import { useEffect, useState } from "react";
import { useRouter } from "next/navigation";
import { toast } from "@/lib/toast";
import {
  isEmbeddedProvider,
  openEmbeddedCheckout,
} from "@/lib/payments/launch";

interface PendingPayment {
  orderId: string;
  provider: string;
  paymentToken: string;
  publicKey: string;
  amount: string;
  currencyCode: string;
  /** Gateway mode ("test" | "live") — an embedded SDK picks its environment from it. */
  mode?: string;
  customerName?: string;
  customerEmail?: string;
}

interface Props {
  orderId: string;
  paymentStatus: string;
  storeName: string;
}

export function PaymentPrompt({ orderId, paymentStatus, storeName }: Props) {
  const router = useRouter();
  const [pending, setPending] = useState<PendingPayment | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (typeof window === "undefined") return;
    const raw = sessionStorage.getItem(`mark8ly.pendingPayment.${orderId}`);
    if (!raw) return;
    try {
      const parsed = JSON.parse(raw) as PendingPayment;
      if (parsed.orderId === orderId) setPending(parsed);
    } catch {
      // ignore malformed
    }
  }, [orderId]);

  if (paymentStatus === "paid" || paymentStatus === "captured") return null;

  // Hosted-checkout providers (Stripe) bring the buyer back here from
  // an off-domain payment page. Payment is captured asynchronously via
  // webhook, so the order may briefly show pending after redirect — say
  // so explicitly instead of rendering blank, and tell the buyer how to
  // recover if they cancelled mid-flow.
  if (!pending || !isEmbeddedProvider(pending.provider)) {
    return (
      <section
        aria-labelledby="payment-status-heading"
        className="mt-8 rounded-md border border-[color:var(--storefront-text,var(--ink-900))]/15 bg-[color:var(--storefront-background,var(--paper-200))] px-6 py-5"
      >
        <h2
          id="payment-status-heading"
          className="text-sm font-semibold uppercase tracking-[0.12em] text-[color:var(--storefront-text,var(--ink-900))] opacity-70"
        >
          Payment status
        </h2>
        <p className="mt-2 text-sm text-[color:var(--storefront-text,var(--ink-900))] opacity-80">
          We&apos;re confirming your payment. This usually takes a few seconds —
          refresh in a moment to see the latest status. If you closed the
          payment window without paying, your card was not charged; please
          contact the store to retry.
        </p>
      </section>
    );
  }

  async function handlePay() {
    if (!pending) return;
    setError(null);
    setBusy(true);
    await openEmbeddedCheckout(
      pending.provider,
      {
        orderId,
        paymentToken: pending.paymentToken,
        publicKey: pending.publicKey,
        amount: pending.amount,
        currencyCode: pending.currencyCode,
        storeName,
        mode: pending.mode,
        customerName: pending.customerName,
        customerEmail: pending.customerEmail,
      },
      {
        onSuccess: () => {
          sessionStorage.removeItem(`mark8ly.pendingPayment.${orderId}`);
          setPending(null);
          toast({
            title: "Payment received",
            description: "Your order is confirmed.",
            tone: "success",
          });
          router.refresh();
        },
        onDismiss: () => setBusy(false),
        onError: (message) => {
          setError(message);
          toast({
            title: "Payment could not be completed",
            description: message,
            tone: "error",
          });
          setBusy(false);
        },
      },
    );
  }

  return (
    <section
      aria-labelledby="pay-now-heading"
      className="mt-8 rounded-md border border-[color:var(--storefront-accent,var(--moss-700))]/30 bg-[color:var(--storefront-accent,var(--moss-700))]/5 px-6 py-5"
    >
      <h2
        id="pay-now-heading"
        className="text-sm font-semibold uppercase tracking-[0.12em] text-[color:var(--storefront-accent,var(--moss-700))]"
      >
        Complete your payment
      </h2>
      <p className="mt-2 text-sm text-[color:var(--storefront-text,var(--ink-900))] opacity-80">
        Your order is reserved. Pay {pending.currencyCode} {pending.amount} now to
        confirm and start fulfillment.
      </p>
      <button
        type="button"
        onClick={handlePay}
        disabled={busy}
        className="mt-4 inline-flex items-center gap-2 rounded-md bg-[color:var(--storefront-accent,var(--ink-900))] px-5 py-2.5 text-sm font-medium text-[color:var(--storefront-on-accent,var(--paper-200))] transition-opacity hover:opacity-90 disabled:cursor-not-allowed disabled:opacity-50"
      >
        {busy ? "Opening…" : "Pay now"}
      </button>
      {error && (
        <p role="alert" className="mt-3 text-sm text-[color:var(--storefront-danger)]">
          {error}
        </p>
      )}
    </section>
  );
}
