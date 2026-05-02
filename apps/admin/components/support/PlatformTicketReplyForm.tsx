"use client";

import {
  useActionState,
  useEffect,
  useRef,
  useState,
  type FormEvent,
} from "react";
import { useRouter } from "next/navigation";
import { useToast } from "@/components/feedback/Toaster";
import {
  replyToPlatformTicketAction,
  type PlatformReplyActionState,
} from "@/app/(admin)/support/platform/[id]/actions";

interface PlatformTicketReplyFormProps {
  ticketId: string;
  /**
   * When true, this ticket is resolved/closed so the form is replaced
   * by a banner. The page already handles that branching; this prop
   * is just defense in case the form is mounted in the wrong state.
   */
  disabled?: boolean;
}

export function PlatformTicketReplyForm({
  ticketId,
  disabled,
}: PlatformTicketReplyFormProps) {
  const router = useRouter();
  const { toast } = useToast();
  const boundAction = replyToPlatformTicketAction.bind(null, ticketId);
  const [state, formAction, isPending] = useActionState<
    PlatformReplyActionState,
    FormData
  >(boundAction, null);

  const [content, setContent] = useState("");
  const [touched, setTouched] = useState(false);
  const [submitAttempted, setSubmitAttempted] = useState(false);
  const textareaRef = useRef<HTMLTextAreaElement>(null);

  const trimmed = content.trim();
  const error =
    (touched || submitAttempted) && !trimmed
      ? "Reply cannot be empty."
      : trimmed.length > 10_000
        ? "Reply is too long (max 10,000 characters)."
        : null;

  // Track last-handled state so the toast doesn't refire on re-render.
  const lastHandledRef = useRef<PlatformReplyActionState>(null);
  useEffect(() => {
    if (state === lastHandledRef.current) return;
    lastHandledRef.current = state;
    if (!state) return;

    if ("success" in state) {
      toast.success("Reply sent");
      setContent("");
      setTouched(false);
      setSubmitAttempted(false);
      router.refresh();
      // Return focus to the textarea so a follow-up reply can be drafted
      // immediately (matches UX-SPEC §3 "focus management after send").
      textareaRef.current?.focus();
    } else if ("error" in state) {
      toast.error("Couldn't send reply", state.error);
    }
  }, [state, toast, router]);

  function handleSubmit(e: FormEvent<HTMLFormElement>) {
    setSubmitAttempted(true);
    if (!trimmed || error) {
      e.preventDefault();
      textareaRef.current?.focus();
    }
  }

  function handleKeyDown(e: React.KeyboardEvent<HTMLTextAreaElement>) {
    if ((e.metaKey || e.ctrlKey) && e.key === "Enter") {
      e.preventDefault();
      e.currentTarget.form?.requestSubmit();
    }
  }

  if (disabled) return null;

  return (
    <form
      action={formAction}
      onSubmit={handleSubmit}
      noValidate
      aria-label="Reply composer"
      className="space-y-3 rounded-md border border-border bg-background-elevated p-4"
    >
      <textarea
        ref={textareaRef}
        name="content"
        value={content}
        onChange={(e) => setContent(e.target.value)}
        onBlur={() => setTouched(true)}
        onKeyDown={handleKeyDown}
        aria-label="Reply to platform team"
        aria-describedby="platform-reply-hint"
        aria-invalid={error ? true : undefined}
        placeholder="Write your reply…"
        rows={5}
        className={
          "w-full resize-y rounded-md border bg-background px-4 py-3 text-sm text-foreground placeholder:text-foreground-tertiary focus:outline-none focus:ring-1 " +
          (error
            ? "border-[color:var(--signal)] focus:border-[color:var(--signal)] focus:ring-[color:var(--signal)]"
            : "border-border focus:border-[color:var(--moss-700)] focus:ring-[color:var(--moss-700)]")
        }
      />
      {error ? (
        <p role="alert" className="text-xs text-[color:var(--signal)]">
          {error}
        </p>
      ) : (
        <p
          id="platform-reply-hint"
          className="text-xs text-foreground-tertiary"
        >
          Cmd/Ctrl + Enter to send.
        </p>
      )}
      <div className="flex justify-end">
        <button
          type="submit"
          disabled={isPending || !trimmed || !!error}
          className="inline-flex h-10 items-center justify-center rounded-md bg-primary px-5 text-sm font-medium text-primary-foreground hover:bg-primary-hover disabled:opacity-50"
        >
          {isPending ? "Sending…" : "Send reply"}
        </button>
      </div>
    </form>
  );
}
