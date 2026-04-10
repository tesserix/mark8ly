"use client";

import { useActionState, useState } from "react";
import { replyToTicketAction } from "@/app/support/actions";

interface TicketReplyFormProps {
  ticketId: string;
}

export function TicketReplyForm({ ticketId }: TicketReplyFormProps) {
  const [state, formAction, isPending] = useActionState(
    replyToTicketAction,
    null,
  );
  const [editing, setEditing] = useState(false);

  const showError = state?.error && !editing;

  return (
    <form action={formAction} className="space-y-4 border-t border-border-subtle pt-6">
      <input type="hidden" name="ticketId" value={ticketId} />

      {showError && (
        <p
          role="alert"
          className="rounded-md border border-[color:var(--signal)]/20 bg-[color:var(--signal)]/5 px-4 py-3 text-sm text-[color:var(--signal)]"
        >
          {state.error}
        </p>
      )}

      <label htmlFor="reply-body" className="block text-sm font-medium text-foreground">
        Reply
      </label>
      <textarea
        id="reply-body"
        name="body"
        required
        rows={4}
        className="w-full rounded-md border border-border bg-background px-4 py-3 text-sm text-foreground placeholder:text-foreground-tertiary focus:border-[color:var(--moss-700)] focus:outline-none focus:ring-1 focus:ring-[color:var(--moss-700)]"
        placeholder="Type your reply..."
        onInput={() => setEditing(true)}
      />
      <button
        type="submit"
        disabled={isPending}
        className="inline-flex h-11 items-center justify-center rounded-md bg-primary px-6 text-sm font-medium text-primary-foreground hover:bg-primary-hover disabled:opacity-50"
      >
        {isPending ? "Sending..." : "Send reply"}
      </button>
    </form>
  );
}
