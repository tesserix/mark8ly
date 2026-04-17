"use client";

import { useActionState, useEffect, useRef } from "react";
import { useRouter } from "next/navigation";

import { replyToTicketAction } from "@/app/(admin)/support/actions";
import { useToast } from "@/components/feedback/Toaster";

interface TicketReplyFormProps {
  ticketId: string;
}

export function TicketReplyForm({ ticketId }: TicketReplyFormProps) {
  const [state, formAction, isPending] = useActionState(
    replyToTicketAction,
    null,
  );
  const formRef = useRef<HTMLFormElement | null>(null);
  const prevStateRef = useRef<typeof state>(null);
  const router = useRouter();
  const { toast } = useToast();

  // React to action results — fire toast + clear form + refresh the
  // thread on success, surface the error as a toast on failure.
  useEffect(() => {
    if (!state || state === prevStateRef.current) return;
    prevStateRef.current = state;

    if (state.ok) {
      toast.success("Reply sent", "The customer will see your message.");
      formRef.current?.reset();
      router.refresh();
    } else {
      toast.error("Couldn't send reply", state.error);
    }
  }, [state, toast, router]);

  return (
    <form
      ref={formRef}
      action={formAction}
      className="space-y-4 border-t border-border-subtle pt-6"
    >
      <input type="hidden" name="ticketId" value={ticketId} />

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
