"use client";

import { useActionState } from "react";
import { updateTicketStatusAction } from "@/app/(admin)/support/actions";
import type { TicketStatus } from "@/lib/api/marketplace-api";

interface TicketStatusActionsProps {
  ticketId: string;
  status: TicketStatus;
}

export function TicketStatusActions({
  ticketId,
  status,
}: TicketStatusActionsProps) {
  const [state, formAction, isPending] = useActionState(
    updateTicketStatusAction,
    null,
  );

  return (
    <div className="flex flex-wrap items-center gap-3">
      {state?.error && (
        <p role="alert" className="w-full text-sm text-[color:var(--signal)]">
          {state.error}
        </p>
      )}

      {status === "open" && (
        <>
          <form action={formAction}>
            <input type="hidden" name="ticketId" value={ticketId} />
            <input type="hidden" name="status" value="resolved" />
            <button
              type="submit"
              disabled={isPending}
              className="inline-flex h-10 items-center justify-center rounded-md bg-[color:var(--moss-700)] px-5 text-sm font-medium text-white hover:bg-[color:var(--moss-700)]/90 disabled:opacity-50"
            >
              Mark resolved
            </button>
          </form>
          <form action={formAction}>
            <input type="hidden" name="ticketId" value={ticketId} />
            <input type="hidden" name="status" value="closed" />
            <button
              type="submit"
              disabled={isPending}
              className="inline-flex h-10 items-center justify-center rounded-md border border-border bg-background-elevated px-5 text-sm font-medium text-foreground hover:border-border-strong disabled:opacity-50"
            >
              Close ticket
            </button>
          </form>
        </>
      )}

      {status === "resolved" && (
        <>
          <form action={formAction}>
            <input type="hidden" name="ticketId" value={ticketId} />
            <input type="hidden" name="status" value="open" />
            <button
              type="submit"
              disabled={isPending}
              className="inline-flex h-10 items-center justify-center rounded-md border border-border bg-background-elevated px-5 text-sm font-medium text-foreground hover:border-border-strong disabled:opacity-50"
            >
              Reopen
            </button>
          </form>
          <form action={formAction}>
            <input type="hidden" name="ticketId" value={ticketId} />
            <input type="hidden" name="status" value="closed" />
            <button
              type="submit"
              disabled={isPending}
              className="inline-flex h-10 items-center justify-center rounded-md border border-border bg-background-elevated px-5 text-sm font-medium text-foreground hover:border-border-strong disabled:opacity-50"
            >
              Close ticket
            </button>
          </form>
        </>
      )}

      {status === "closed" && (
        <form action={formAction}>
          <input type="hidden" name="ticketId" value={ticketId} />
          <input type="hidden" name="status" value="open" />
          <button
            type="submit"
            disabled={isPending}
            className="inline-flex h-10 items-center justify-center rounded-md border border-border bg-background-elevated px-5 text-sm font-medium text-foreground hover:border-border-strong disabled:opacity-50"
          >
            Reopen ticket
          </button>
        </form>
      )}
    </div>
  );
}
