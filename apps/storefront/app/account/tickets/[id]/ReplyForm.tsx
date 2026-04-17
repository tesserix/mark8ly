"use client";

import { useState, useTransition } from "react";
import { useRouter } from "next/navigation";

import { toast } from "@/lib/toast";
import { replyToTicket } from "./actions";

interface ReplyFormProps {
  ticketId: string;
  disabled?: boolean;
  disabledReason?: string;
}

export function ReplyForm({
  ticketId,
  disabled,
  disabledReason,
}: ReplyFormProps) {
  const router = useRouter();
  const [content, setContent] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [isPending, startTransition] = useTransition();

  if (disabled) {
    return (
      <p className="text-sm text-[color:var(--storefront-text,var(--ink-900))] opacity-60">
        {disabledReason ?? "Replies are disabled on this ticket."}
      </p>
    );
  }

  function handleSubmit(e: React.FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const value = content.trim();
    if (!value) {
      setError("Message cannot be empty.");
      return;
    }
    setError(null);
    startTransition(async () => {
      const result = await replyToTicket(ticketId, value);
      if (!result.ok) {
        setError(result.message);
        toast({
          title: "Couldn't send reply",
          description: result.message,
          tone: "error",
        });
        return;
      }
      setContent("");
      toast({ title: "Reply sent", tone: "success" });
      router.refresh();
    });
  }

  return (
    <form onSubmit={handleSubmit} className="space-y-3">
      <label htmlFor="reply" className="sr-only">
        Your reply
      </label>
      <textarea
        id="reply"
        name="reply"
        value={content}
        onChange={(e) => setContent(e.target.value)}
        required
        maxLength={5000}
        rows={5}
        placeholder="Write a reply..."
        disabled={isPending}
        className="w-full rounded-[6px] border border-[color:var(--storefront-text,var(--ink-900))]/15 bg-white px-3 py-2.5 text-sm text-[color:var(--storefront-text,var(--ink-900))] focus-visible:border-[color:var(--storefront-accent,var(--moss-700))] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[color:var(--storefront-accent,var(--moss-700))]/20 disabled:opacity-60"
      />
      {error && (
        <p role="alert" className="text-sm text-[color:var(--signal)]">
          {error}
        </p>
      )}
      <button
        type="submit"
        disabled={isPending}
        className="h-10 rounded-[6px] bg-[color:var(--ink-900)] px-5 text-sm font-medium text-white transition-colors hover:bg-[color:var(--ink-900)]/90 disabled:opacity-60"
      >
        {isPending ? "Sending..." : "Send reply"}
      </button>
    </form>
  );
}
