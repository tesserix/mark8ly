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
      className="flex items-start gap-3 overflow-x-auto border-y border-border-subtle py-5 sm:gap-4"
    >
      {stages.map((stage, i) => (
        <div
          key={stage.key}
          className="flex min-w-0 flex-1 items-start gap-3 sm:gap-4"
        >
          <StageNode
            label={stage.label}
            timestamp={stage.timestamp}
            state={stage.state}
          />
          {i < stages.length - 1 && (
            <span
              aria-hidden="true"
              className="mt-1.5 h-px flex-1 bg-[color:var(--ink-900)]/10"
            />
          )}
        </div>
      ))}
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
          : "border border-[color:var(--ink-900)]/30 bg-transparent";

  const labelClass =
    state === "upcoming"
      ? "text-foreground-tertiary"
      : state === "cancelled"
        ? "text-[color:var(--danger)]"
        : "text-foreground";

  return (
    <div className="flex shrink-0 flex-col items-start gap-1">
      <div className="flex items-center gap-2">
        <span
          aria-hidden="true"
          className={`h-2.5 w-2.5 shrink-0 rounded-full ${dotClass}`}
        />
        <span
          className={`text-xs font-medium uppercase tracking-wider ${labelClass}`}
        >
          {label}
        </span>
      </div>
      {timestamp && (
        <span className="pl-4.5 text-[11px] text-foreground-tertiary">
          {timestamp}
        </span>
      )}
    </div>
  );
}
