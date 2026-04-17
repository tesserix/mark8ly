"use client";

import { useActionState } from "react";
import { updateTicketStatusAction } from "@/app/(admin)/support/actions";
import type { TicketStatus } from "@/lib/api/marketplace-api";

interface TicketStatusActionsProps {
  ticketId: string;
  status: TicketStatus;
}

// Which status actions to show for each current status. Kept as data
// so adding a new state in the future doesn't require touching the
// markup twice. The first action in each list is styled as the
// primary CTA; the rest are secondary.
const ACTIONS: Record<TicketStatus, { target: TicketStatus; label: string }[]> = {
  open: [
    { target: "in_progress", label: "Start working" },
    { target: "resolved", label: "Mark resolved" },
    { target: "closed", label: "Close" },
  ],
  in_progress: [
    { target: "resolved", label: "Mark resolved" },
    { target: "open", label: "Move back to open" },
    { target: "closed", label: "Close" },
  ],
  resolved: [
    { target: "in_progress", label: "Reopen as in progress" },
    { target: "open", label: "Reopen" },
    { target: "closed", label: "Close" },
  ],
  // Closed is terminal in the backend — no outbound transitions.
  closed: [],
};

export function TicketStatusActions({
  ticketId,
  status,
}: TicketStatusActionsProps) {
  const [state, formAction, isPending] = useActionState(
    updateTicketStatusAction,
    null,
  );

  const actions = ACTIONS[status] ?? [];

  return (
    <div className="flex flex-wrap items-center gap-3">
      {state?.error && (
        <p role="alert" className="w-full text-sm text-[color:var(--signal)]">
          {state.error}
        </p>
      )}

      {actions.length === 0 && (
        <p className="text-sm text-foreground-secondary">
          This ticket is closed and can&apos;t be reopened.
        </p>
      )}

      {actions.map((action, idx) => {
        const isPrimary = idx === 0;
        return (
          <form key={action.target} action={formAction}>
            <input type="hidden" name="ticketId" value={ticketId} />
            <input type="hidden" name="status" value={action.target} />
            <button
              type="submit"
              disabled={isPending}
              className={
                isPrimary
                  ? "inline-flex h-10 items-center justify-center rounded-md bg-[color:var(--moss-700)] px-5 text-sm font-medium text-white hover:bg-[color:var(--moss-700)]/90 disabled:opacity-50"
                  : "inline-flex h-10 items-center justify-center rounded-md border border-border bg-background-elevated px-5 text-sm font-medium text-foreground hover:border-border-strong disabled:opacity-50"
              }
            >
              {action.label}
            </button>
          </form>
        );
      })}
    </div>
  );
}
