"use client";

// StatusActions — customer-driven status transitions on the ticket
// detail page. The server enforces the same allowed-transition set, so
// this component's only job is to:
//   • hide buttons that aren't applicable for the current state
//   • give clear "are you sure?" feedback so a misclick is recoverable
//   • show inline error text + a toast on failure
//
// State is reloaded via router.refresh() rather than local state so the
// header stepper + status chip stay coherent with the rest of the page.

import { useState, useTransition } from "react";
import { useRouter } from "next/navigation";

import { toast } from "@/lib/toast";

import {
  updateTicketStatus,
  type CustomerStatusTarget,
  type CustomerTicketStatus,
} from "./actions";

interface StatusActionsProps {
  ticketId: string;
  status: CustomerTicketStatus;
}

// allowedTransitions mirrors backend CustomerAllowedStatusTransition.
// Keep this map in sync with services/marketplace-api/internal/ticket/
// service.go CustomerAllowedStatusTransition — but the server is the
// source of truth, so anything stale here just shows a button the
// backend will 409 on.
const allowedTransitions: Record<CustomerTicketStatus, CustomerStatusTarget[]> = {
  open: ["resolved", "closed"],
  in_progress: ["resolved", "closed"],
  resolved: ["open", "closed"],
  closed: [],
};

const labels: Record<CustomerStatusTarget, { button: string; success: string }> = {
  resolved: {
    button: "Mark as resolved",
    success: "Ticket marked as resolved.",
  },
  closed: {
    button: "Close ticket",
    success: "Ticket closed.",
  },
  open: {
    button: "Reopen ticket",
    success: "Ticket reopened.",
  },
};

export function StatusActions({ ticketId, status }: StatusActionsProps) {
  const router = useRouter();
  const [pendingTarget, setPendingTarget] = useState<CustomerStatusTarget | null>(
    null,
  );
  const [isPending, startTransition] = useTransition();
  const [error, setError] = useState<string | null>(null);

  const targets = allowedTransitions[status];
  if (targets.length === 0) {
    return null;
  }

  function handleClick(target: CustomerStatusTarget) {
    setPendingTarget(target);
    setError(null);
    startTransition(async () => {
      const result = await updateTicketStatus(ticketId, target);
      if (!result.ok) {
        setError(result.message);
        toast({
          title: "Couldn't update ticket",
          description: result.message,
          tone: "error",
        });
        setPendingTarget(null);
        return;
      }
      toast({ title: labels[target].success, tone: "success" });
      setPendingTarget(null);
      router.refresh();
    });
  }

  return (
    <div className="space-y-2">
      <div className="flex flex-wrap gap-2">
        {targets.map((target) => {
          const isThis = isPending && pendingTarget === target;
          // Closing is a destructive intent (archives the ticket and
          // blocks future replies) — render it as a subdued outline so
          // the resolve action stays the default visual emphasis.
          const isOutline = target === "closed";
          return (
            <button
              key={target}
              type="button"
              onClick={() => handleClick(target)}
              disabled={isPending}
              aria-busy={isThis}
              className={
                isOutline
                  ? "inline-flex h-9 items-center rounded-[var(--storefront-radius,6px)] border border-[color:var(--storefront-text,var(--ink-900))]/20 px-4 text-xs font-medium text-[color:var(--storefront-text,var(--ink-900))] transition-opacity hover:opacity-80 disabled:opacity-50"
                  : "inline-flex h-9 items-center rounded-[var(--storefront-radius,6px)] bg-[color:var(--storefront-primary,var(--ink-900))] px-4 text-xs font-medium text-[color:var(--storefront-on-primary,#fff)] transition-opacity hover:opacity-90 disabled:opacity-50"
              }
            >
              {isThis ? "Saving..." : labels[target].button}
            </button>
          );
        })}
      </div>
      {error && (
        <p role="alert" className="text-xs text-[color:var(--storefront-danger)]">
          {error}
        </p>
      )}
    </div>
  );
}
