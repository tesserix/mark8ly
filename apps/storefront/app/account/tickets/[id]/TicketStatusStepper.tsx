// Read-only ticket status stepper for the storefront. Mirrors the
// admin stepper visually so merchants and shoppers see the same
// workflow, but the customer can't trigger transitions — this is
// purely informational. No "use client" needed: pure presentational.

import { Check } from "lucide-react";

interface TicketStatusStepperProps {
  status: "open" | "in_progress" | "resolved" | "closed";
}

const STEPS: {
  status: TicketStatusStepperProps["status"];
  label: string;
  hint: string;
}[] = [
  { status: "open", label: "Opened", hint: "We received your ticket." },
  {
    status: "in_progress",
    label: "In progress",
    hint: "Our team is looking into it.",
  },
  {
    status: "resolved",
    label: "Resolved",
    hint: "We've sent a response — reply if you need more help.",
  },
  { status: "closed", label: "Closed", hint: "This ticket is archived." },
];

function currentStepIndex(status: TicketStatusStepperProps["status"]): number {
  return STEPS.findIndex((s) => s.status === status);
}

export function TicketStatusStepper({ status }: TicketStatusStepperProps) {
  const activeIndex = currentStepIndex(status);

  return (
    <div
      aria-label="Ticket progress"
      role="list"
      className="flex items-start gap-2 overflow-x-auto"
    >
      {STEPS.map((step, idx) => {
        const isActive = idx === activeIndex;
        const isPast = idx < activeIndex;

        const circleClass = isActive
          ? "bg-[color:var(--storefront-surface,white)] border-[color:var(--storefront-accent,var(--moss-700))] text-[color:var(--storefront-accent,var(--moss-700))]"
          : isPast
            ? "bg-[color:var(--storefront-accent,var(--moss-700))] border-[color:var(--storefront-accent,var(--moss-700))] text-white"
            : "bg-[color:var(--storefront-surface,white)] border-[color:var(--storefront-text,var(--ink-900))]/15 text-[color:var(--storefront-text,var(--ink-900))]/40";

        const labelClass = isActive
          ? "font-medium text-[color:var(--storefront-text,var(--ink-900))]"
          : isPast
            ? "text-[color:var(--storefront-text,var(--ink-900))] opacity-70"
            : "text-[color:var(--storefront-text,var(--ink-900))] opacity-40";

        const connectorClass =
          idx < STEPS.length - 1
            ? `mt-4 h-px flex-1 ${
                idx < activeIndex
                  ? "bg-[color:var(--storefront-accent,var(--moss-700))]"
                  : "bg-[color:var(--storefront-text,var(--ink-900))]/15"
              }`
            : "";

        return (
          <div key={step.status} role="listitem" className="contents">
            <div className="flex min-w-[88px] max-w-[160px] flex-col items-center gap-1 text-center">
              <span
                className={`flex h-8 w-8 shrink-0 items-center justify-center rounded-full border-2 text-sm font-semibold ${circleClass}`}
                aria-current={isActive ? "step" : undefined}
                aria-hidden={!isActive}
              >
                {isPast ? <Check className="h-4 w-4" /> : idx + 1}
              </span>
              <p className={`text-xs leading-tight ${labelClass}`}>
                {step.label}
              </p>
              {isActive && (
                <p className="text-[10px] leading-snug text-[color:var(--storefront-text,var(--ink-900))] opacity-60">
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
  );
}
