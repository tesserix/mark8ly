"use client";

// Interactive ticket status stepper for the admin detail page.
// Replaces the old row of action buttons with a 4-step visual flow:
//   Open → In progress → Resolved → Closed
// Tapping any step attempts the transition via the same server action
// the old buttons used. Invalid transitions (current step, or anything
// from a closed ticket) stay disabled and grey.
//
// The status machine is kept in lockstep with
// services/marketplace-api/internal/ticket/models.go CanTransitionTo.

import { useActionState, useEffect, useRef } from "react";
import { useRouter } from "next/navigation";
import { Check } from "lucide-react";

import { updateTicketStatusAction } from "@/app/(admin)/support/actions";
import { useToast } from "@/components/feedback/Toaster";
import type { TicketStatus } from "@/lib/api/marketplace-api";

interface TicketStatusStepperProps {
  ticketId: string;
  status: TicketStatus;
}

const STEPS: { status: TicketStatus; label: string; hint: string }[] = [
  {
    status: "open",
    label: "Open",
    hint: "Received, not yet actioned",
  },
  {
    status: "in_progress",
    label: "In progress",
    hint: "Being worked on",
  },
  {
    status: "resolved",
    label: "Resolved",
    hint: "Answered — awaiting confirmation",
  },
  {
    status: "closed",
    label: "Closed",
    hint: "Archived — no further replies",
  },
];

// Matches backend CanTransitionTo. Closed is terminal.
const ALLOWED_TARGETS: Record<TicketStatus, TicketStatus[]> = {
  open: ["in_progress", "resolved", "closed"],
  in_progress: ["open", "resolved", "closed"],
  resolved: ["open", "in_progress", "closed"],
  closed: [],
};

function currentStepIndex(status: TicketStatus): number {
  return STEPS.findIndex((s) => s.status === status);
}

export function TicketStatusStepper({
  ticketId,
  status,
}: TicketStatusStepperProps) {
  const [state, formAction, isPending] = useActionState(
    updateTicketStatusAction,
    null,
  );
  const prevStateRef = useRef<typeof state>(null);
  const router = useRouter();
  const { toast } = useToast();

  useEffect(() => {
    if (!state || state === prevStateRef.current) return;
    prevStateRef.current = state;

    if (state.ok) {
      toast.success("Ticket updated", "Status change saved.");
      router.refresh();
    } else {
      toast.error("Couldn't update status", state.error ?? "Please try again.");
    }
  }, [state, toast, router]);

  const activeIndex = currentStepIndex(status);
  const allowedTargets = new Set(ALLOWED_TARGETS[status]);

  return (
    <div className="space-y-3">
      <div
        aria-label="Ticket status"
        role="list"
        className="relative flex items-start gap-2 overflow-x-auto"
      >
        {STEPS.map((step, idx) => {
          const isActive = idx === activeIndex;
          const isPast = idx < activeIndex;
          const canTransition = allowedTargets.has(step.status);
          const canClick = canTransition && !isPending;

          // Visual states: past (filled moss), active (outlined moss),
          // future+clickable (outlined ink-tint, hover moss), future+disabled
          // (muted grey).
          const circleClass = isActive
            ? "bg-[color:var(--background-elevated,white)] border-[color:var(--moss-700)] text-[color:var(--moss-700)]"
            : isPast
              ? "bg-[color:var(--moss-700)] border-[color:var(--moss-700)] text-white"
              : canClick
                ? "bg-[color:var(--background-elevated,white)] border-border text-foreground-secondary hover:border-[color:var(--moss-700)] hover:text-[color:var(--moss-700)]"
                : "bg-paper-100 border-border text-foreground-tertiary";

          const labelClass = isActive
            ? "font-medium text-foreground"
            : isPast
              ? "text-foreground-secondary"
              : canClick
                ? "text-foreground-secondary"
                : "text-foreground-tertiary";

          const connectorClass =
            idx < STEPS.length - 1
              ? `mt-4 h-px flex-1 ${
                  idx < activeIndex
                    ? "bg-[color:var(--moss-700)]"
                    : "bg-border"
                }`
              : "";

          const StepCircle = (
            <span
              className={`flex h-8 w-8 shrink-0 items-center justify-center rounded-full border-2 text-sm font-semibold transition-colors ${circleClass}`}
              aria-hidden="true"
            >
              {isPast ? <Check className="h-4 w-4" /> : idx + 1}
            </span>
          );

          return (
            <div key={step.status} role="listitem" className="contents">
              <div className="flex min-w-[88px] max-w-[160px] flex-col items-center gap-1 text-center">
                {canClick ? (
                  <form action={formAction}>
                    <input type="hidden" name="ticketId" value={ticketId} />
                    <input
                      type="hidden"
                      name="status"
                      value={step.status}
                    />
                    <button
                      type="submit"
                      aria-label={`Move ticket to ${step.label}`}
                      aria-current={isActive ? "step" : undefined}
                      className="block rounded-full focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)] disabled:opacity-60"
                      disabled={!canClick}
                    >
                      {StepCircle}
                    </button>
                  </form>
                ) : (
                  <span
                    aria-current={isActive ? "step" : undefined}
                    aria-disabled="true"
                  >
                    {StepCircle}
                  </span>
                )}
                <p className={`text-xs leading-tight ${labelClass}`}>
                  {step.label}
                </p>
                {isActive && (
                  <p className="text-[10px] leading-snug text-foreground-tertiary">
                    {step.hint}
                  </p>
                )}
              </div>
              {idx < STEPS.length - 1 && (
                <div className={connectorClass} aria-hidden="true" />
              )}
            </div>
          );
        })}
      </div>

      {isPending && (
        <p className="text-xs text-foreground-tertiary">Updating status...</p>
      )}

      {state && !state.ok && (
        <p role="alert" className="text-sm text-[color:var(--signal)]">
          {state.error}
        </p>
      )}

      {status === "closed" && (
        <p className="text-xs text-foreground-tertiary">
          This ticket is closed and can&apos;t be reopened from the stepper.
        </p>
      )}
    </div>
  );
}
