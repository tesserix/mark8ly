"use client";

// Client island rendered inside the (server) /orders/[id] page. When
// the order is still pending and the storefront stashed a payment
// context for it on the checkout page, this component:
//
//   1. Loads Razorpay's checkout.js script on demand.
//   2. On "Pay now", opens the Razorpay widget keyed to the merchant's
//      public key + the pre-created Razorpay order id (payment_token).
//   3. On widget success, POSTs {razorpay_order_id, razorpay_payment_id,
//      razorpay_signature} to /api/orders/{id}/verify-payment, which
//      proxies to marketplace-api's verify endpoint. The backend re-derives
//      the HMAC and marks the order paid via the same handler the webhook
//      flow uses, so this works even when no Razorpay webhook reaches
//      the cluster.
//   4. Clears the local sessionStorage context and refreshes the page.

import { useEffect, useState } from "react";
import { useRouter } from "next/navigation";
import { toast } from "@/lib/toast";

interface PendingPayment {
  orderId: string;
  provider: string;
  paymentToken: string;
  publicKey: string;
  amount: string;
  currencyCode: string;
  customerName?: string;
  customerEmail?: string;
}

declare global {
  interface Window {
    Razorpay?: new (options: Record<string, unknown>) => { open: () => void };
  }
}

const RZP_SCRIPT = "https://checkout.razorpay.com/v1/checkout.js";

function loadRazorpay(): Promise<boolean> {
  if (typeof window === "undefined") return Promise.resolve(false);
  if (window.Razorpay) return Promise.resolve(true);
  return new Promise((resolve) => {
    const existing = document.querySelector<HTMLScriptElement>(
      `script[src="${RZP_SCRIPT}"]`,
    );
    if (existing) {
      existing.addEventListener("load", () => resolve(!!window.Razorpay));
      existing.addEventListener("error", () => resolve(false));
      return;
    }
    const s = document.createElement("script");
    s.src = RZP_SCRIPT;
    s.async = true;
    s.onload = () => resolve(!!window.Razorpay);
    s.onerror = () => resolve(false);
    document.head.appendChild(s);
  });
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
  if (!pending) return null;
  if (pending.provider !== "razorpay") return null;

  async function handlePay() {
    if (!pending) return;
    setError(null);
    setBusy(true);
    const ok = await loadRazorpay();
    if (!ok || !window.Razorpay) {
      setError("Could not load the Razorpay widget. Check your connection and try again.");
      setBusy(false);
      return;
    }

    const amountPaise = Math.round(Number.parseFloat(pending.amount) * 100);
    const rzp = new window.Razorpay({
      key: pending.publicKey,
      amount: amountPaise,
      currency: pending.currencyCode,
      name: storeName || "Storefront",
      order_id: pending.paymentToken,
      prefill: {
        name: pending.customerName ?? "",
        email: pending.customerEmail ?? "",
      },
      handler: async (resp: {
        razorpay_order_id: string;
        razorpay_payment_id: string;
        razorpay_signature: string;
      }) => {
        try {
          const verifyRes = await fetch(`/api/orders/${orderId}/verify-payment`, {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify(resp),
          });
          if (!verifyRes.ok) {
            const body = await verifyRes.text().catch(() => "");
            setError(`Payment verification failed (${verifyRes.status}). ${body}`);
            toast({
              title: "Payment verification failed",
              description: body || `HTTP ${verifyRes.status}`,
              tone: "error",
            });
            setBusy(false);
            return;
          }
          sessionStorage.removeItem(`mark8ly.pendingPayment.${orderId}`);
          setPending(null);
          toast({
            title: "Payment received",
            description: "Your order is confirmed.",
            tone: "success",
          });
          router.refresh();
        } catch (e) {
          setError(e instanceof Error ? e.message : "Unknown error verifying payment.");
          setBusy(false);
        }
      },
      modal: {
        ondismiss: () => setBusy(false),
      },
      theme: { color: "#2C5530" },
    });
    rzp.open();
  }

  return (
    <section
      aria-labelledby="pay-now-heading"
      className="mt-8 rounded-md border border-[color:var(--moss-700)]/30 bg-[color:var(--moss-700)]/5 px-6 py-5"
    >
      <h2
        id="pay-now-heading"
        className="text-sm font-semibold uppercase tracking-[0.12em] text-[color:var(--moss-700)]"
      >
        Complete your payment
      </h2>
      <p className="mt-2 text-sm text-[color:var(--ink-900)] opacity-80">
        Your order is reserved. Pay {pending.currencyCode} {pending.amount} now to
        confirm and start fulfillment.
      </p>
      <button
        type="button"
        onClick={handlePay}
        disabled={busy}
        className="mt-4 inline-flex items-center gap-2 rounded-md bg-[color:var(--ink-900)] px-5 py-2.5 text-sm font-medium text-[color:var(--paper-200)] transition-opacity hover:opacity-90 disabled:cursor-not-allowed disabled:opacity-50"
      >
        {busy ? "Opening…" : "Pay now"}
      </button>
      {error && (
        <p role="alert" className="mt-3 text-sm text-red-700">
          {error}
        </p>
      )}
    </section>
  );
}
