"use client";

// Client-rendered order/shipment timeline. Renders the initial server
// payload, then polls GET /api/account/orders/:id/timeline every 5s
// while the tab is in the foreground so the customer sees "Out for
// delivery" → "Delivered" transitions the moment the marketplace-api
// sync loop (every 2 min) or the Delhivery webhook writes a new
// order_events row. On top of the timer we also re-pull on window
// focus / tab visibility change — many customers leave an order tab
// open for days, and the focus refresh catches them up instantly
// instead of waiting up to a full tick.

import { useEffect, useMemo, useRef, useState } from "react";
import { toast } from "@/lib/toast";
import type { OrderShipment, OrderTimelineEntry } from "@/lib/api/checkout-api";

interface Props {
  orderId: string;
  initialShipment?: OrderShipment | null;
  initialTimeline: OrderTimelineEntry[];
}

interface TimelineResponse {
  data: {
    shipment?: OrderShipment | null;
    timeline: OrderTimelineEntry[];
  };
}

const POLL_MS = 5000;

function iconFor(kind: string): string {
  if (kind.startsWith("shipment_")) {
    if (kind === "shipment_delivered") return "✓";
    if (kind === "shipment_exception") return "!";
    return "→";
  }
  if (kind === "payment_recorded") return "$";
  if (kind === "fulfilled") return "✓";
  if (kind === "cancelled") return "×";
  // Return/refund lifecycle — mirrors the return_requested → approved →
  // received → refunded (or rejected) states surfaced by ReturnStatusBanner.
  if (kind === "return_requested") return "↩";
  if (kind === "return_approved") return "✓";
  if (kind === "return_received") return "✓";
  if (kind === "return_refunded") return "$";
  if (kind === "return_rejected") return "×";
  return "•";
}

type EventTone = "success" | "danger" | "warning" | "neutral";

// Semantic status per event kind, driving both the stepper circle/label
// styling and the "new event" toast tone below. Delivered/fulfilled and
// their return-flow equivalents (approved/refunded) all read as positive
// outcomes; exception/cancelled/rejected read as negative or cautionary.
function toneFor(kind: string): EventTone {
  if (
    kind === "shipment_delivered" ||
    kind === "fulfilled" ||
    kind === "return_approved" ||
    kind === "return_refunded"
  ) {
    return "success";
  }
  if (kind === "shipment_exception" || kind === "cancelled") {
    return "danger";
  }
  if (kind === "return_rejected") {
    return "warning";
  }
  return "neutral";
}

function fmtDate(iso: string): string {
  try {
    return new Date(iso).toLocaleString(undefined, {
      month: "short",
      day: "numeric",
      hour: "numeric",
      minute: "2-digit",
    });
  } catch {
    return iso;
  }
}

// Module-scope (not a per-render closure) so both the stepper labels and
// the "new event" toast title below can reuse the same humanized copy.
function shortLabelFor(kind: string, description?: string): string {
  if (description && description.length > 0 && description.length <= 28) {
    // Use the backend-provided copy when it fits on one line of a
    // stepper pill; fall back to a predictable short label otherwise.
    return description.replace(/\.$/, "");
  }
  switch (kind) {
    case "order_placed":
      return "Placed";
    case "payment_recorded":
      return "Paid";
    case "status_changed":
    case "confirmed":
      return "Confirmed";
    case "fulfilled":
      return "Fulfilled";
    case "shipment_created":
      return "Label created";
    case "pickup_scheduled":
      return "Pickup scheduled";
    case "shipment_in_transit":
      return "In transit";
    case "shipment_out_for_delivery":
      return "Out for delivery";
    case "shipment_delivered":
      return "Delivered";
    case "shipment_exception":
      return "Exception";
    case "cancelled":
      return "Cancelled";
    // Return/refund lifecycle — previously fell through to the raw
    // snake_case kind (e.g. "return_requested") because these cases
    // didn't exist yet.
    case "return_requested":
      return "Return requested";
    case "return_approved":
      return "Return approved";
    case "return_received":
      return "Return received";
    case "return_refunded":
      return "Refund issued";
    case "return_rejected":
      return "Return declined";
    default:
      return kind.replace(/_/g, " ");
  }
}

export function OrderTimeline({ orderId, initialShipment, initialTimeline }: Props) {
  const [shipment, setShipment] = useState<OrderShipment | null>(initialShipment ?? null);
  const [events, setEvents] = useState<OrderTimelineEntry[]>(initialTimeline);

  // Keep the latest seen kind so we can toast on new arrivals.
  const latestKey = useMemo(() => {
    const last = events[events.length - 1];
    return last ? `${last.kind}@${last.occurred_at}` : "";
  }, [events]);

  // pullRef holds the current pull() closure so the focus/visibility
  // handlers can invoke it without being redeclared every render — the
  // outer effect owns the interval lifecycle, the inner one only
  // wires listeners.
  const pullRef = useRef<(() => void) | null>(null);

  useEffect(() => {
    let stopped = false;
    let seenKey = latestKey;
    let intervalId: ReturnType<typeof setInterval> | null = null;
    let kickId: ReturnType<typeof setTimeout> | null = null;

    async function pull() {
      try {
        const res = await fetch(`/api/orders/${orderId}/timeline`, {
          cache: "no-store",
        });
        if (!res.ok) return;
        const body = (await res.json()) as TimelineResponse;
        const nextTimeline = body.data.timeline ?? [];
        const nextShipment = body.data.shipment ?? null;
        if (stopped) return;

        const nextLast = nextTimeline[nextTimeline.length - 1];
        const nextKey = nextLast ? `${nextLast.kind}@${nextLast.occurred_at}` : "";
        if (nextKey !== seenKey) {
          // A new event arrived since the last poll — pop a toast.
          if (nextLast && seenKey !== "") {
            const tone = toneFor(nextLast.kind);
            toast({
              title: shortLabelFor(nextLast.kind, nextLast.description),
              tone:
                tone === "success"
                  ? "success"
                  : tone === "danger"
                    ? "error"
                    : tone === "warning"
                      ? "warn"
                      : "info",
            });
          }
          seenKey = nextKey;
          setEvents(nextTimeline);
          setShipment(nextShipment);
        }
      } catch {
        // Transient network error — next tick will retry.
      }
    }
    pullRef.current = pull;

    function startPolling() {
      if (intervalId !== null) return;
      intervalId = setInterval(pull, POLL_MS);
    }
    function stopPolling() {
      if (intervalId === null) return;
      clearInterval(intervalId);
      intervalId = null;
    }

    // Prime once after mount so a tab reopened hours later catches up fast.
    kickId = setTimeout(pull, 750);

    // Only poll while the tab is visible — no point hitting the API
    // every 5s for a tab the user has backgrounded. Focus/visibility
    // listener re-pulls once on resume to catch up, then resumes the
    // interval. Covers the iOS Safari suspended-tab edge case too.
    if (document.visibilityState === "visible") startPolling();

    const onVisibilityOrFocus = () => {
      if (document.visibilityState === "visible") {
        void pull();
        startPolling();
      } else {
        stopPolling();
      }
    };
    window.addEventListener("focus", onVisibilityOrFocus);
    document.addEventListener("visibilitychange", onVisibilityOrFocus);

    return () => {
      stopped = true;
      stopPolling();
      if (kickId !== null) clearTimeout(kickId);
      window.removeEventListener("focus", onVisibilityOrFocus);
      document.removeEventListener("visibilitychange", onVisibilityOrFocus);
    };
  }, [orderId, latestKey]);

  // Last event is the "current" step — every event before it is done,
  // every event after (there are none until a new one arrives) is
  // future. Terminal states (delivered / exception / cancelled) still
  // render as the current step so the operator sees the final state
  // explicitly instead of a quiet green-on-green chain.
  const lastIdx = events.length - 1;

  return (
    <section aria-labelledby="timeline-heading" className="mt-8">
      <div className="flex flex-wrap items-baseline justify-between gap-2">
        <h2
          id="timeline-heading"
          className="text-sm font-semibold uppercase tracking-[0.12em] text-[color:var(--storefront-text,var(--ink-900))] opacity-60"
        >
          Delivery timeline
        </h2>
        {(() => {
          const last = events[lastIdx];
          if (!last) return null;
          return (
            <p className="text-xs text-[color:var(--storefront-text,var(--ink-900))] opacity-60">
              Latest: {shortLabelFor(last.kind, last.description)}
              {" · "}
              {fmtDate(last.occurred_at)}
            </p>
          );
        })()}
      </div>

      {shipment && (
        <div className="mt-3 flex flex-wrap items-center gap-x-4 gap-y-1 text-xs text-[color:var(--storefront-text,var(--ink-900))] opacity-70">
          <span className="font-medium uppercase tracking-wider">
            {shipment.carrier}
          </span>
          {shipment.tracking_number && (
            <span>
              Tracking:{" "}
              <span className="font-mono tracking-tight text-[color:var(--storefront-text,var(--ink-900))]">
                {shipment.tracking_number}
              </span>
            </span>
          )}
          <span>Status: {shipment.status}</span>
          {shipment.estimated_delivery && (
            <span>ETA: {fmtDate(shipment.estimated_delivery)}</span>
          )}
        </div>
      )}

      {events.length === 0 ? (
        <p className="mt-4 text-sm text-[color:var(--storefront-text,var(--ink-900))] opacity-60">
          We&apos;ll post updates here the moment your order is on the way.
        </p>
      ) : (
        // Horizontal stepper. The `overflow-x-auto` wrapper scrolls on
        // narrow viewports; the inner min-width keeps each pill
        // legible instead of wrapping to a squashed grid. The
        // connecting line sits behind the circles via an ::before
        // gradient on the <ol> so it animates past every node.
        <div className="mt-5 -mx-4 overflow-x-auto px-4 pb-2 sm:-mx-0 sm:px-0">
          <ol
            className="relative flex min-w-max items-start gap-0"
            role="list"
          >
            {/* connecting line. Grows from the first to the last
                completed step; future hops render muted. */}
            <div
              aria-hidden
              className="pointer-events-none absolute left-5 right-5 top-4 h-px bg-[color:var(--storefront-text,var(--ink-900))]/15"
            />
            {events.map((e, i) => {
              const isLast = i === lastIdx;
              const done = i < lastIdx;
              // Negative (shipment_exception/cancelled) and cautionary
              // (return_rejected) kinds override the position-based
              // color below. Positive kinds (delivered/fulfilled and the
              // return_approved/return_refunded equivalents) intentionally
              // get no override — the normal isLast/done moss coloring
              // already reads as "success", same treatment as before.
              const tone = toneFor(e.kind);
              const isError = tone === "danger";
              const isWarning = tone === "warning";
              const circleTone = isError
                ? "border-[color:var(--storefront-danger)] bg-[color:var(--storefront-danger-bg)] text-[color:var(--storefront-danger)]"
                : isWarning
                  ? "border-[color:var(--storefront-warning)] bg-[color:var(--storefront-warning-bg)] text-[color:var(--storefront-warning)]"
                  : isLast
                    ? "border-[color:var(--storefront-accent,var(--moss-700))] bg-[color:var(--storefront-accent,var(--moss-700))] text-[color:var(--storefront-on-accent,#fff)] shadow-sm ring-4 ring-[color:var(--storefront-accent,var(--moss-700))]/15"
                    : done
                      ? "border-[color:var(--storefront-accent,var(--moss-700))]/80 bg-[color:var(--storefront-accent,var(--moss-700))]/90 text-[color:var(--storefront-on-accent,#fff)]"
                      : "border-[color:var(--storefront-text,var(--ink-900))]/25 bg-[color:var(--storefront-background,var(--paper-200))] text-[color:var(--storefront-text,var(--ink-900))]/60";
              const labelTone = isError
                ? "text-[color:var(--storefront-danger)]"
                : isWarning
                  ? "text-[color:var(--storefront-warning)]"
                  : isLast
                    ? "text-[color:var(--storefront-text,var(--ink-900))] font-semibold"
                    : "text-[color:var(--storefront-text,var(--ink-900))]/80";
              return (
                <li
                  key={`${e.kind}-${e.occurred_at}-${i}`}
                  className="relative flex w-[148px] shrink-0 flex-col items-center text-center"
                  aria-current={isLast ? "step" : undefined}
                >
                  <span
                    aria-hidden
                    className={`relative z-10 grid h-9 w-9 place-items-center rounded-full border text-sm font-semibold transition-colors ${circleTone}`}
                  >
                    {done ? "✓" : iconFor(e.kind)}
                  </span>
                  <p className={`mt-2 px-1 text-xs leading-tight ${labelTone}`}>
                    {shortLabelFor(e.kind, e.description)}
                  </p>
                  <p className="mt-1 text-[11px] leading-tight text-[color:var(--storefront-text,var(--ink-900))]/55">
                    {fmtDate(e.occurred_at)}
                  </p>
                </li>
              );
            })}
          </ol>
        </div>
      )}
    </section>
  );
}
