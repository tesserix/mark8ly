// Editorial lifecycle rail for a return/replacement request.
// Four linear stages (Requested → Approved → Received → Refunded) with
// a terminal Rejected branch. Uses the same flex + flex-1 hairline
// connector pattern as OrderLifecycleStepper so lines auto-balance
// regardless of label width.

import { Fragment } from "react";

import type { AdminReturn, AdminReturnStatus } from "@/lib/api/marketplace-api";
import { formatDate } from "@/lib/format";

interface ReturnLifecycleStepperProps {
  rma: AdminReturn;
}

type StageKey = "requested" | "approved" | "received" | "refunded";
type StageState = "done" | "active" | "upcoming" | "cancelled";

interface Stage {
  key: string;
  label: string;
  state: StageState;
  timestamp?: string;
}

function computeStages(rma: AdminReturn): Stage[] {
  const isApproved =
    rma.status === "approved" ||
    rma.status === "received" ||
    rma.status === "refunded";
  const isReceived = rma.status === "received" || rma.status === "refunded";
  const isRefunded = rma.status === "refunded";

  let active: StageKey = "approved";
  if (isRefunded) active = "refunded";
  else if (isReceived) active = "refunded";
  else if (isApproved) active = "received";
  else active = "approved";

  const mark = (key: StageKey, done: boolean): StageState =>
    done ? "done" : key === active ? "active" : "upcoming";

  const stageAt = (done: boolean, iso?: string): string | undefined =>
    done && iso ? formatDate(iso) : undefined;

  return [
    {
      key: "requested",
      label: "Requested",
      state: mark("requested", true),
      timestamp: formatDate(rma.requested_at),
    },
    {
      key: "approved",
      label: "Approved",
      state: mark("approved", isApproved),
      timestamp: stageAt(isApproved, rma.approved_at),
    },
    {
      key: "received",
      label: "Received",
      state: mark("received", isReceived),
      timestamp: stageAt(isReceived, rma.received_at),
    },
    {
      key: "refunded",
      label: rma.type === "replace" ? "Replaced" : "Refunded",
      state: mark("refunded", isRefunded),
      timestamp: stageAt(isRefunded, rma.refunded_at),
    },
  ];
}

export function ReturnLifecycleStepper({ rma }: ReturnLifecycleStepperProps) {
  const stages: Stage[] =
    rma.status === "rejected"
      ? [
          {
            key: "requested",
            label: "Requested",
            state: "done",
            timestamp: formatDate(rma.requested_at),
          },
          {
            key: "rejected",
            label: "Rejected",
            state: "cancelled",
            timestamp: rma.rejected_at ? formatDate(rma.rejected_at) : undefined,
          },
        ]
      : computeStages(rma);

  return (
    <nav
      aria-label="Return lifecycle"
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
