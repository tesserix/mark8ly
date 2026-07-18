// Amazon-style Cancel / Return action on /account/orders/[id].
//
// Renders a single clean button ("Cancel Order" or "Return Order")
// based on order + shipment state. On click opens a modal with
// predefined reason radio buttons + an "Other" option with text field.

"use client";

import { useState, useTransition, useRef, useEffect } from "react";
import type { KeyboardEvent } from "react";
import { useRouter } from "next/navigation";
import Link from "next/link";
import { toast } from "@/lib/toast";

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
  // Return flow only: "return" (refund only) vs "replace" (exchange).
  // Defaulted to "return" so the button still works if the customer
  // doesn't touch the toggle.
  const [returnType, setReturnType] = useState<"return" | "replace">("return");
  const [error, setError] = useState<string | null>(null);
  const [done, setDone] = useState(false);
  const dialogRef = useRef<HTMLDialogElement>(null);
  const returnTypeRefs = useRef<Array<HTMLButtonElement | null>>([]);

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
      // A shipment is created (status "pending") the instant a shipping
      // label is cut — well before the parcel physically moves. The old
      // copy here claimed the order was "in transit," which is false and
      // left the customer with no path forward. Mirror the backend's
      // actual guard (see marketplace-api order_detail.go's shipment_in_flight
      // 409) and give them a real next step instead of a dead end.
      return (
        <p className="max-w-sm text-xs leading-relaxed text-[color:var(--storefront-text,var(--ink-900))]/70">
          A shipping label has already been created for this order, so it
          can&apos;t be cancelled online.{" "}
          <Link
            href="/contact"
            className="font-medium text-[color:var(--storefront-accent,var(--moss-700))] underline decoration-1 underline-offset-2 hover:opacity-80"
          >
            Contact the store
          </Link>{" "}
          to arrange a return and refund.
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
        } else {
          // Return or Replace: persist a real request the admin will
          // see in the RMA inbox. The body omits the items array so the
          // backend defaults to requesting the full order (v1 UX; a
          // future iteration adds per-item multi-select).
          const resp = await fetch(`/api/orders/${encodeURIComponent(orderId)}/returns`, {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify({
              type: returnType,
              reason: finalReason,
            }),
          });
          if (!resp.ok) {
            const body = (await resp.json().catch(() => ({}))) as { message?: string };
            setError(body.message || "Could not submit your request. Please try again.");
            return;
          }
        }
        setDone(true);
        setOpen(false);
        toast({
          title:
            mode === "cancel"
              ? "Cancellation requested"
              : returnType === "replace"
                ? "Replacement requested"
                : "Return requested",
          description:
            mode === "cancel"
              ? "We've received your request and will confirm shortly."
              : "Our team will review your request and reach out.",
          tone: "success",
        });
        router.refresh();
      } catch {
        setError("Network error. Please try again.");
      }
    });
  }

  const RETURN_TYPE_OPTIONS = ["return", "replace"] as const;

  // WAI-ARIA APG radiogroup pattern: arrow keys move selection with
  // wraparound, Home/End jump to the ends, and only the checked radio
  // is in the tab order (roving tabindex) so Tab skips past the group
  // as a single stop.
  function handleReturnTypeKeyDown(
    event: KeyboardEvent<HTMLButtonElement>,
    index: number,
  ) {
    let nextIndex = index;
    switch (event.key) {
      case "ArrowRight":
      case "ArrowDown":
        nextIndex = (index + 1) % RETURN_TYPE_OPTIONS.length;
        break;
      case "ArrowLeft":
      case "ArrowUp":
        nextIndex =
          (index - 1 + RETURN_TYPE_OPTIONS.length) % RETURN_TYPE_OPTIONS.length;
        break;
      case "Home":
        nextIndex = 0;
        break;
      case "End":
        nextIndex = RETURN_TYPE_OPTIONS.length - 1;
        break;
      default:
        return;
    }
    event.preventDefault();
    setReturnType(RETURN_TYPE_OPTIONS[nextIndex] ?? RETURN_TYPE_OPTIONS[0]);
    returnTypeRefs.current[nextIndex]?.focus();
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
        aria-labelledby="cancel-return-modal-title"
        className="m-auto w-full max-w-md rounded-lg border border-[color:var(--storefront-text,var(--ink-900))]/10 bg-[color:var(--storefront-surface)] p-0 shadow-md backdrop:bg-[color:var(--storefront-text,var(--ink-900))]/30"
      >
        <div className="px-6 py-5">
          <h2
            id="cancel-return-modal-title"
            className="font-[family-name:var(--storefront-heading-font,var(--font-source-serif))] text-lg text-[color:var(--storefront-text,var(--ink-900))]"
          >
            {mode === "cancel" ? "Why are you cancelling?" : "Return or replace?"}
          </h2>
          <p className="mt-1 text-xs text-[color:var(--storefront-text,var(--ink-900))]/50">
            {mode === "cancel"
              ? "Select a reason for cancellation"
              : "Tell us what you'd like and why — our team will review and reach out."}
          </p>

          {mode === "return" && (
            <div className="mt-4 flex gap-2" role="radiogroup" aria-label="Request type">
              {RETURN_TYPE_OPTIONS.map((t, index) => (
                <button
                  key={t}
                  ref={(el) => {
                    returnTypeRefs.current[index] = el;
                  }}
                  type="button"
                  role="radio"
                  aria-checked={returnType === t}
                  tabIndex={returnType === t ? 0 : -1}
                  onClick={() => setReturnType(t)}
                  onKeyDown={(e) => handleReturnTypeKeyDown(e, index)}
                  className={`flex-1 rounded-md border px-3 py-2 text-sm font-medium capitalize transition-colors ${
                    returnType === t
                      ? "border-[color:var(--storefront-accent,var(--moss-700))] bg-[color:var(--storefront-accent,var(--moss-700))]/5 text-[color:var(--storefront-text,var(--ink-900))]"
                      : "border-[color:var(--storefront-text,var(--ink-900))]/15 text-[color:var(--storefront-text,var(--ink-900))]/70 hover:border-[color:var(--storefront-text,var(--ink-900))]/30"
                  }`}
                >
                  {t === "return" ? "Return for refund" : "Replace"}
                </button>
              ))}
            </div>
          )}

          <fieldset className="mt-4 space-y-2">
            <legend className="sr-only">
              {mode === "cancel" ? "Reason for cancellation" : "Reason for return"}
            </legend>
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
            <p role="alert" className="mt-3 text-xs text-[color:var(--storefront-danger)]">
              {error}
            </p>
          )}

          <div className="mt-5 flex items-center gap-3">
            <button
              type="button"
              onClick={handleSubmit}
              disabled={pending}
              className="flex-1 rounded-md bg-[color:var(--storefront-text,var(--ink-900))] px-4 py-2.5 text-sm font-medium text-[color:var(--storefront-background,var(--paper-200))] transition-opacity hover:opacity-90 disabled:cursor-not-allowed disabled:opacity-50"
            >
              {pending
                ? "Processing..."
                : mode === "cancel"
                  ? "Confirm Cancellation"
                  : returnType === "replace"
                    ? "Submit Replacement Request"
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
