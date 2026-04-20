// Editorial lifecycle rail for order status. Five stages (Placed →
// Confirmed → Fulfilled → Shipped → Delivered) with a terminal
// Cancelled branch. Layout is flex-based with flex-1 hairline
// connectors between stages — lines auto-balance to the available
// gap regardless of label width, which the previous absolute-within-
// grid approach could not.

import { Fragment } from "react";

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
type StageState = "done" | "active" | "upcoming" | "cancelled";

interface Stage {
  key: string;
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

  let active: StageKey = "placed";
  if (isDelivered) active = "delivered";
  else if (isShipped) active = "delivered";
  else if (isFulfilled) active = "shipped";
  else if (isConfirmed) active = "fulfilled";
  else active = "confirmed";

  const mark = (key: StageKey, done: boolean): StageState =>
    done ? "done" : key === active ? "active" : "upcoming";

  const stageAt = (done: boolean, iso?: string | null): string | undefined =>
    done && iso ? formatDate(iso) : undefined;

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
  const stages: Stage[] =
    order.status === "cancelled"
      ? [
          {
            key: "placed",
            label: "Placed",
            state: "done",
            timestamp: formatDate(order.placed_at),
          },
          { key: "cancelled", label: "Cancelled", state: "cancelled" },
        ]
      : computeStages(
          order.status,
          order.fulfillment_status,
          shipmentStatus,
          order.placed_at,
          order.fulfilled_at,
        );

  return (
    <nav
      aria-label="Order lifecycle"
      className="flex items-start border-y border-border-subtle py-6"
    >
      {stages.map((stage, i) => (
        <Fragment key={stage.key}>
          <StageNode
            label={stage.label}
            timestamp={stage.timestamp}
            state={stage.state}
          />
          {i < stages.length - 1 && (
            <span
              aria-hidden="true"
              className="mx-4 mt-[5px] h-px min-w-6 flex-1 bg-[color:var(--ink-900)]/10"
            />
          )}
        </Fragment>
      ))}
    </nav>
  );
}

interface StageNodeProps {
  label: string;
  timestamp?: string;
  state: StageState;
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
    <div className="flex shrink-0 flex-col gap-1.5">
      <div className="flex items-center gap-2">
        <span
          aria-hidden="true"
          className={`h-2.5 w-2.5 shrink-0 rounded-full ${dotClass}`}
        />
        <span
          className={`whitespace-nowrap text-[11px] font-semibold uppercase tracking-[0.12em] ${labelClass}`}
        >
          {label}
        </span>
      </div>
      {timestamp && (
        <span className="pl-[18px] text-[11px] tabular-nums text-foreground-tertiary">
          {timestamp}
        </span>
      )}
    </div>
  );
}
