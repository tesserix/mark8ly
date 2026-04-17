// Amazon-style Cancel / Return action on /account/orders/[id].
//
// Renders a single clean button ("Cancel Order" or "Return Order")
// based on order + shipment state. On click opens a modal with
// predefined reason radio buttons + an "Other" option with text field.

"use client";

import { useState, useTransition, useRef, useEffect } from "react";
import { useRouter } from "next/navigation";

interface Props {
  orderId: string;
  orderStatus: string;
  shipmentStatus: string | null;
}

const CANCELLABLE = new Set(["pending", "confirmed"]);

const CANCEL_REASONS = [
  "I placed the order by mistake",
  "Item no longer needed",
  "Found a better price elsewhere",
  "Delivery time is too long",
  "Wrong item ordered",
];

const RETURN_REASONS = [
  "Item damaged on arrival",
  "Item not as described",
  "Wrong item received",
  "Quality not as expected",
  "Changed my mind",
];

export function CancelOrderButton({ orderId, orderStatus, shipmentStatus }: Props) {
  const router = useRouter();
  const [pending, startTransition] = useTransition();
  const [open, setOpen] = useState(false);
  const [selectedReason, setSelectedReason] = useState("");
  const [otherText, setOtherText] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [done, setDone] = useState(false);
  const dialogRef = useRef<HTMLDialogElement>(null);

  const canCancel = CANCELLABLE.has(orderStatus) && shipmentStatus === null;
  const isDelivered = shipmentStatus === "delivered";
  const isInFlight = shipmentStatus !== null && shipmentStatus !== "delivered";
  const isCancelled = orderStatus === "cancelled";

  // Sync dialog open/close with state
  useEffect(() => {
    if (open) {
      dialogRef.current?.showModal();
    } else {
      dialogRef.current?.close();
    }
  }, [open]);

  if (isCancelled || done) return null;

  // Determine mode
  const mode: "cancel" | "return" | null = canCancel
    ? "cancel"
    : isDelivered
      ? "return"
      : null;

  if (!mode) {
    if (isInFlight) {
      return (
        <p className="text-xs text-[color:var(--storefront-text,var(--ink-900))]/50 italic">
          This order is in transit — cancellation is no longer available.
        </p>
      );
    }
    return null;
  }

  const reasons = mode === "cancel" ? CANCEL_REASONS : RETURN_REASONS;
  const finalReason = selectedReason === "Other" ? otherText.trim() : selectedReason;

  function handleSubmit() {
    if (!finalReason) {
      setError("Please select a reason.");
      return;
    }
    setError(null);
    startTransition(async () => {
      try {
        if (mode === "cancel") {
          const resp = await fetch(`/api/orders/${encodeURIComponent(orderId)}/cancel`, {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify({ reason: finalReason }),
          });
          if (!resp.ok) {
            const body = (await resp.json().catch(() => ({}))) as { message?: string };
            setError(body.message || "Could not cancel the order. Please try again.");
            return;
          }
        }
        // For returns: just close the modal with a confirmation (no backend endpoint yet)
        setDone(true);
        setOpen(false);
        router.refresh();
      } catch {
        setError("Network error. Please try again.");
      }
    });
  }

  function handleClose() {
    if (pending) return;
    setOpen(false);
    setSelectedReason("");
    setOtherText("");
    setError(null);
  }

  const label = mode === "cancel" ? "Cancel Order" : "Return or Replace";

  return (
    <>
      <button
        type="button"
        onClick={() => setOpen(true)}
        className="inline-flex items-center rounded-md border border-[color:var(--storefront-text,var(--ink-900))]/25 px-4 py-2 text-sm font-medium text-[color:var(--storefront-text,var(--ink-900))] transition-colors hover:border-[color:var(--storefront-accent,var(--moss-700))] hover:text-[color:var(--storefront-accent,var(--moss-700))]"
      >
        {label}
      </button>

      {/* Modal */}
      <dialog
        ref={dialogRef}
        onClose={handleClose}
        className="m-auto w-full max-w-md rounded-lg border border-[color:var(--storefront-text,var(--ink-900))]/10 bg-white p-0 shadow-xl backdrop:bg-black/40"
      >
        <div className="px-6 py-5">
          <h2 className="font-[family-name:var(--storefront-heading-font,var(--font-source-serif))] text-lg text-[color:var(--storefront-text,var(--ink-900))]">
            {mode === "cancel" ? "Why are you cancelling?" : "Why are you returning?"}
          </h2>
          <p className="mt-1 text-xs text-[color:var(--storefront-text,var(--ink-900))]/50">
            {mode === "cancel"
              ? "Select a reason for cancellation"
              : "Select a reason for your return"}
          </p>

          <fieldset className="mt-4 space-y-2">
            {reasons.map((r) => (
              <label
                key={r}
                className={`flex cursor-pointer items-center gap-3 rounded-md border px-3 py-2.5 text-sm transition-colors ${
                  selectedReason === r
                    ? "border-[color:var(--storefront-accent,var(--moss-700))] bg-[color:var(--storefront-accent,var(--moss-700))]/5 text-[color:var(--storefront-text,var(--ink-900))]"
                    : "border-[color:var(--storefront-text,var(--ink-900))]/10 text-[color:var(--storefront-text,var(--ink-900))]/80 hover:border-[color:var(--storefront-text,var(--ink-900))]/25"
                }`}
              >
                <input
                  type="radio"
                  name="reason"
                  value={r}
                  checked={selectedReason === r}
                  onChange={() => setSelectedReason(r)}
                  className="accent-[color:var(--storefront-accent,var(--moss-700))]"
                />
                {r}
              </label>
            ))}
            <label
              className={`flex cursor-pointer items-center gap-3 rounded-md border px-3 py-2.5 text-sm transition-colors ${
                selectedReason === "Other"
                  ? "border-[color:var(--storefront-accent,var(--moss-700))] bg-[color:var(--storefront-accent,var(--moss-700))]/5 text-[color:var(--storefront-text,var(--ink-900))]"
                  : "border-[color:var(--storefront-text,var(--ink-900))]/10 text-[color:var(--storefront-text,var(--ink-900))]/80 hover:border-[color:var(--storefront-text,var(--ink-900))]/25"
              }`}
            >
              <input
                type="radio"
                name="reason"
                value="Other"
                checked={selectedReason === "Other"}
                onChange={() => setSelectedReason("Other")}
                className="accent-[color:var(--storefront-accent,var(--moss-700))]"
              />
              Other
            </label>
            {selectedReason === "Other" && (
              <input
                type="text"
                value={otherText}
                onChange={(e) => setOtherText(e.target.value)}
                placeholder="Tell us more..."
                autoFocus
                className="mt-1 w-full rounded-md border border-[color:var(--storefront-text,var(--ink-900))]/15 px-3 py-2 text-sm text-[color:var(--storefront-text,var(--ink-900))] placeholder:opacity-40 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--storefront-accent,var(--moss-700))]"
              />
            )}
          </fieldset>

          {error && (
            <p role="alert" className="mt-3 text-xs text-red-600">
              {error}
            </p>
          )}

          <div className="mt-5 flex items-center gap-3">
            <button
              type="button"
              onClick={handleSubmit}
              disabled={pending}
              className="flex-1 rounded-md bg-[color:var(--storefront-text,var(--ink-900))] px-4 py-2.5 text-sm font-medium text-white transition-opacity hover:opacity-90 disabled:cursor-not-allowed disabled:opacity-50"
            >
              {pending
                ? "Processing..."
                : mode === "cancel"
                  ? "Confirm Cancellation"
                  : "Submit Return Request"}
            </button>
            <button
              type="button"
              onClick={handleClose}
              disabled={pending}
              className="rounded-md border border-[color:var(--storefront-text,var(--ink-900))]/20 px-4 py-2.5 text-sm text-[color:var(--storefront-text,var(--ink-900))]/70 transition-colors hover:border-[color:var(--storefront-text,var(--ink-900))]/40 disabled:cursor-not-allowed disabled:opacity-40"
            >
              Go Back
            </button>
          </div>
        </div>
      </dialog>
    </>
  );
}
