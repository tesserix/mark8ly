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
    return "border-[color:var(--storefront-accent,var(--moss-700))] bg-[color:var(--storefront-accent,var(--moss-700))]/10 text-[color:var(--storefront-accent,var(--moss-700))]";
  }
  if (kind === "shipment_exception" || kind === "cancelled") {
    return "border-red-300 bg-red-50 text-red-700";
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

  return (
    <section aria-labelledby="timeline-heading" className="mt-8">
      <h2
        id="timeline-heading"
        className="text-sm font-semibold uppercase tracking-[0.12em] text-[color:var(--storefront-text,var(--ink-900))] opacity-60"
      >
        Delivery timeline
      </h2>

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
        <ol className="mt-4 space-y-3">
          {events.map((e, i) => (
            <li
              key={`${e.kind}-${e.occurred_at}-${i}`}
              className={`flex items-start gap-3 rounded-md border px-3 py-2 text-sm ${toneFor(e.kind)}`}
            >
              <span
                aria-hidden
                className="mt-0.5 grid h-5 w-5 place-items-center rounded-full border border-current text-[11px] font-semibold"
              >
                {iconFor(e.kind)}
              </span>
              <div className="flex-1">
                <p className="font-medium">{e.description || e.kind}</p>
                <p className="mt-0.5 text-xs opacity-70">{fmtDate(e.occurred_at)}</p>
              </div>
            </li>
          ))}
        </ol>
      )}
    </section>
  );
}
