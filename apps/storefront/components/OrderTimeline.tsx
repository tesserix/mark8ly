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
  return "•";
}

function toneFor(kind: string): string {
  if (kind === "shipment_delivered" || kind === "fulfilled") {
    return "border-[color:var(--storefront-success)] bg-[color:var(--storefront-success-bg)] text-[color:var(--storefront-success)]";
  }
  if (kind === "shipment_exception" || kind === "cancelled") {
    return "border-[color:var(--storefront-danger-border)] bg-[color:var(--storefront-danger-bg)] text-[color:var(--storefront-danger)]";
  }
  if (kind.startsWith("shipment_")) {
    return "border-[color:var(--storefront-text,var(--ink-900))]/20 bg-[color:var(--storefront-background,var(--paper-200))] text-[color:var(--storefront-text,var(--ink-900))]";
  }
  return "border-[color:var(--storefront-text,var(--ink-900))]/15 bg-[color:var(--storefront-background,var(--paper-200))] text-[color:var(--storefront-text,var(--ink-900))]";
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
            toast({
              title: nextLast.description || nextLast.kind,
              tone:
                nextLast.kind === "shipment_delivered"
                  ? "success"
                  : nextLast.kind === "shipment_exception"
                    ? "error"
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

    const id = window.setInterval(pull, POLL_MS);
    // Prime once after mount so a tab reopened hours later catches up fast.
    const kick = window.setTimeout(pull, 750);

    // Re-pull the moment the tab regains focus or visibility — the
    // interval keeps running in the background on modern browsers,
    // but a suspended mobile Safari tab may have missed several
    // ticks, so an explicit trigger on resume avoids the customer
    // seeing a stale "In transit" when their package was actually
    // delivered hours ago.
    const onFocus = () => {
      if (document.visibilityState === "visible") {
        void pull();
      }
    };
    window.addEventListener("focus", onFocus);
    document.addEventListener("visibilitychange", onFocus);

    return () => {
      stopped = true;
      window.clearInterval(id);
      window.clearTimeout(kick);
      window.removeEventListener("focus", onFocus);
      document.removeEventListener("visibilitychange", onFocus);
    };
  }, [orderId, latestKey]);

  // Last event is the "current" step — every event before it is done,
  // every event after (there are none until a new one arrives) is
  // future. Terminal states (delivered / exception / cancelled) still
  // render as the current step so the operator sees the final state
  // explicitly instead of a quiet green-on-green chain.
  const lastIdx = events.length - 1;
  const shortLabelFor = (kind: string, description?: string): string => {
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
      default:
        return kind.replace(/_/g, " ");
    }
  };

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
              const isError =
                e.kind === "shipment_exception" || e.kind === "cancelled";
              const circleTone = isError
                ? "border-[color:var(--storefront-danger)] bg-[color:var(--storefront-danger-bg)] text-[color:var(--storefront-danger)]"
                : isLast
                  ? "border-[color:var(--storefront-accent,var(--moss-700))] bg-[color:var(--storefront-accent,var(--moss-700))] text-[color:var(--storefront-on-accent,#fff)] shadow-sm ring-4 ring-[color:var(--storefront-accent,var(--moss-700))]/15"
                  : done
                    ? "border-[color:var(--storefront-accent,var(--moss-700))]/80 bg-[color:var(--storefront-accent,var(--moss-700))]/90 text-[color:var(--storefront-on-accent,#fff)]"
                    : "border-[color:var(--storefront-text,var(--ink-900))]/25 bg-[color:var(--storefront-background,var(--paper-200))] text-[color:var(--storefront-text,var(--ink-900))]/60";
              const labelTone = isError
                ? "text-[color:var(--storefront-danger)]"
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
