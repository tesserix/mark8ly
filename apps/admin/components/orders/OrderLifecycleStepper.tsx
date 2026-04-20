// Editorial milestone rail for order lifecycle. Five linear stages
// (Placed → Confirmed → Fulfilled → Shipped → Delivered) plus a
// terminal Cancelled branch. The colour of each dot carries the state —
// no filled progress bar. The connecting line is a hairline only.

import type {
  AdminOrder,
  OrderStatus,
  FulfillmentStatus,
} from "@/lib/api/marketplace-api";
import { formatDate } from "@/lib/format";

interface OrderLifecycleStepperProps {
  order: AdminOrder;
  shipmentStatus?: string | null;
}

type StageKey = "placed" | "confirmed" | "fulfilled" | "shipped" | "delivered";
type StageState = "done" | "active" | "upcoming";

interface Stage {
  key: StageKey;
  label: string;
  state: StageState;
  timestamp?: string;
}

function computeStages(
  status: OrderStatus,
  fulfillment: FulfillmentStatus,
  shipmentStatus: string | null | undefined,
  placedAt: string,
  fulfilledAt: string | null | undefined,
): Stage[] {
  const isFulfilled = fulfillment === "fulfilled" || status === "fulfilled";
  const hasShipment = Boolean(shipmentStatus);
  const isShipped =
    hasShipment &&
    shipmentStatus !== "pending" &&
    shipmentStatus !== "cancelled";
  const isDelivered = shipmentStatus === "delivered";
  const isConfirmed =
    status === "confirmed" || isFulfilled || isDelivered || hasShipment;

  const stageAt = (done: boolean, iso?: string | null): string | undefined =>
    done && iso ? formatDate(iso) : undefined;

  // Pick the single active stage — the last stage that isn't done yet.
  let active: StageKey = "placed";
  if (isDelivered) active = "delivered";
  else if (isShipped) active = "delivered";
  else if (isFulfilled) active = "shipped";
  else if (isConfirmed) active = "fulfilled";
  else active = "confirmed";

  const mark = (key: StageKey, done: boolean): StageState =>
    done ? "done" : key === active ? "active" : "upcoming";

  return [
    {
      key: "placed",
      label: "Placed",
      state: mark("placed", true),
      timestamp: formatDate(placedAt),
    },
    {
      key: "confirmed",
      label: "Confirmed",
      state: mark("confirmed", isConfirmed),
    },
    {
      key: "fulfilled",
      label: "Fulfilled",
      state: mark("fulfilled", isFulfilled),
      timestamp: stageAt(isFulfilled, fulfilledAt),
    },
    {
      key: "shipped",
      label: "Shipped",
      state: mark("shipped", isShipped),
    },
    {
      key: "delivered",
      label: "Delivered",
      state: mark("delivered", isDelivered),
    },
  ];
}

export function OrderLifecycleStepper({
  order,
  shipmentStatus,
}: OrderLifecycleStepperProps) {
  if (order.status === "cancelled") {
    return (
      <nav
        aria-label="Order lifecycle"
        className="flex items-center gap-6 border-y border-border-subtle py-5"
      >
        <StageNode
          label="Placed"
          timestamp={formatDate(order.placed_at)}
          state="done"
        />
        <span
          aria-hidden="true"
          className="h-px flex-1 bg-[color:var(--ink-900)]/10"
        />
        <StageNode label="Cancelled" state="cancelled" />
      </nav>
    );
  }

  const stages = computeStages(
    order.status,
    order.fulfillment_status,
    shipmentStatus,
    order.placed_at,
    order.fulfilled_at,
  );

  return (
    <nav
      aria-label="Order lifecycle"
      className="border-y border-border-subtle py-6"
    >
      <ol className="grid grid-cols-5 gap-0">
        {stages.map((stage, i) => (
          <li key={stage.key} className="relative flex flex-col gap-1.5">
            {i < stages.length - 1 && (
              <span
                aria-hidden="true"
                className="absolute left-[calc(50%+0.5rem+0.25rem)] right-0 top-[5px] h-px bg-[color:var(--ink-900)]/10"
              />
            )}
            <StageNode
              label={stage.label}
              timestamp={stage.timestamp}
              state={stage.state}
            />
          </li>
        ))}
      </ol>
    </nav>
  );
}

interface StageNodeProps {
  label: string;
  timestamp?: string;
  state: StageState | "cancelled";
}

function StageNode({ label, timestamp, state }: StageNodeProps) {
  const dotClass =
    state === "done"
      ? "bg-[color:var(--ink-900)]"
      : state === "active"
        ? "bg-[color:var(--moss-700)] ring-4 ring-[color:var(--moss-700)]/20"
        : state === "cancelled"
          ? "bg-[color:var(--danger)]"
          : "border border-[color:var(--ink-900)]/25 bg-[color:var(--background)]";

  const labelClass =
    state === "upcoming"
      ? "text-foreground-tertiary"
      : state === "cancelled"
        ? "text-[color:var(--danger)]"
        : "text-foreground";

  return (
    <>
      <div className="flex items-center gap-2 pl-0">
        <span
          aria-hidden="true"
          className={`relative z-10 h-2.5 w-2.5 shrink-0 rounded-full ${dotClass}`}
        />
        <span
          className={`truncate text-[11px] font-semibold uppercase tracking-[0.12em] ${labelClass}`}
        >
          {label}
        </span>
      </div>
      {/* Reserved row keeps every stage the same height so the connector
          line stays horizontal even when only some stages have dates. */}
      <span
        className={`block min-h-[14px] pl-[18px] text-[11px] tabular-nums text-foreground-tertiary ${timestamp ? "" : "invisible"}`}
      >
        {timestamp ?? "—"}
      </span>
    </>
  );
}
