// Customer-facing cancel control on /account/orders/[id].
//
// Visible only when cancellation is plausible (order is pending/confirmed
// AND no shipment has dispatched). The button still hits the backend on
// click — the backend re-validates ownership + state-machine + shipment
// guards — so the disabled UI here is purely an affordance hint.
//
// Two-step interaction (button → confirm with optional reason) so a
// single accidental click can't take a customer's order out from under
// them.

"use client";

import { useState, useTransition } from "react";
import { useRouter } from "next/navigation";

interface Props {
  orderId: string;
  orderStatus: string;
  shipmentStatus: string | null;
}

const CANCELLABLE_ORDER_STATUSES = new Set(["pending", "confirmed"]);
const SHIPMENT_BLOCKS = new Set(["in_transit", "out_for_delivery", "delivered"]);

export function CancelOrderButton({ orderId, orderStatus, shipmentStatus }: Props) {
  const router = useRouter();
  const [pending, startTransition] = useTransition();
  const [step, setStep] = useState<"button" | "confirm" | "done">("button");
  const [reason, setReason] = useState("");
  const [error, setError] = useState<string | null>(null);

  const orderCancellable = CANCELLABLE_ORDER_STATUSES.has(orderStatus);
  const shipmentBlocks = shipmentStatus !== null && SHIPMENT_BLOCKS.has(shipmentStatus);

  if (!orderCancellable || step === "done") {
    if (step === "done") {
      return (
        <section className="rounded-md border border-[color:var(--storefront-accent,var(--moss-700))]/20 bg-[color:var(--storefront-accent,var(--moss-700))]/5 px-4 py-3 text-sm text-[color:var(--storefront-text,var(--ink-900))]">
          Your cancellation request has been received. The page will refresh with
          the updated status shortly.
        </section>
      );
    }
    return null;
  }

  if (shipmentBlocks) {
    return (
      <section className="rounded-md border border-[color:var(--storefront-text,var(--ink-900))]/15 bg-[color:var(--storefront-background,var(--paper-200))]/40 px-4 py-3 text-sm text-[color:var(--storefront-text,var(--ink-900))]/70">
        This order has already shipped — please reply to your order
        confirmation email to arrange a return and refund.
      </section>
    );
  }

  function submit() {
    setError(null);
    startTransition(async () => {
      try {
        const resp = await fetch(`/api/orders/${encodeURIComponent(orderId)}/cancel`, {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ reason: reason.trim() || "Cancelled by customer" }),
        });
        if (resp.ok) {
          setStep("done");
          // Pull the freshly-cancelled order so status badges + the
          // disappearing button reflect the new state.
          router.refresh();
          return;
        }
        const body = (await resp.json().catch(() => ({}))) as { message?: string; error?: string };
        setError(
          body.message ??
            (body.error === "shipment_in_flight"
              ? "Your order has already shipped — contact support for a return."
              : "Could not cancel the order. Please try again."),
        );
      } catch (e) {
        setError(e instanceof Error ? e.message : "Network error.");
      }
    });
  }

  if (step === "button") {
    return (
      <section className="border-t border-[color:var(--storefront-text,var(--ink-900))]/10 pt-4">
        <h2 className="text-sm font-semibold uppercase tracking-[0.12em] text-[color:var(--storefront-text,var(--ink-900))] opacity-60">
          Need to cancel?
        </h2>
        <p className="mt-2 text-sm text-[color:var(--storefront-text,var(--ink-900))]/70">
          You can cancel this order while it's still being prepared. Once it
          ships you'll need to reach out to support for a return.
        </p>
        <button
          type="button"
          onClick={() => setStep("confirm")}
          className="mt-3 inline-flex items-center gap-2 rounded-md border border-[color:var(--storefront-text,var(--ink-900))]/30 px-4 py-2 text-sm font-medium text-[color:var(--storefront-text,var(--ink-900))] transition-colors hover:border-[color:var(--storefront-accent,var(--moss-700))] hover:text-[color:var(--storefront-accent,var(--moss-700))]"
        >
          Request cancellation
        </button>
      </section>
    );
  }

  return (
    <section className="rounded-md border border-[color:var(--storefront-text,var(--ink-900))]/20 bg-white px-4 py-4">
      <h2 className="font-[family-name:var(--storefront-heading-font,var(--font-source-serif))] text-base text-[color:var(--storefront-text,var(--ink-900))]">
        Cancel this order
      </h2>
      <p className="mt-1 text-xs text-[color:var(--storefront-text,var(--ink-900))]/60">
        We'll cancel the order and email you a confirmation. If a payment was
        captured, the refund will follow shortly.
      </p>
      <label className="mt-3 block text-xs uppercase tracking-wider text-[color:var(--storefront-text,var(--ink-900))]/60">
        Reason (optional)
        <input
          type="text"
          value={reason}
          onChange={(e) => setReason(e.target.value)}
          placeholder="e.g. ordered the wrong size"
          className="mt-1 w-full rounded-md border border-[color:var(--storefront-text,var(--ink-900))]/15 bg-white px-3 py-2 text-sm text-[color:var(--storefront-text,var(--ink-900))] placeholder:opacity-40 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--storefront-accent,var(--moss-700))]"
        />
      </label>
      {error && (
        <p role="alert" className="mt-3 text-xs text-[color:var(--danger,#5a1010)]">
          {error}
        </p>
      )}
      <div className="mt-3 flex items-center gap-3">
        <button
          type="button"
          onClick={submit}
          disabled={pending}
          className="inline-flex items-center gap-2 rounded-md bg-[color:var(--storefront-text,var(--ink-900))] px-4 py-2 text-sm font-medium text-white transition-opacity hover:opacity-90 disabled:cursor-not-allowed disabled:opacity-50"
        >
          {pending ? "Cancelling…" : "Confirm cancellation"}
        </button>
        <button
          type="button"
          onClick={() => setStep("button")}
          disabled={pending}
          className="text-sm text-[color:var(--storefront-text,var(--ink-900))]/70 underline-offset-4 hover:underline hover:text-[color:var(--storefront-text,var(--ink-900))] disabled:cursor-not-allowed disabled:opacity-40"
        >
          Keep order
        </button>
      </div>
    </section>
  );
}
