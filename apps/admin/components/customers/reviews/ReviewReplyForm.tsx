"use client";

import { useState, useTransition } from "react";

import { replyToReviewAction } from "@/app/customers/reviews/actions";

interface ReviewReplyFormProps {
  reviewId: string;
}

const MAX_LENGTH = 5000;

export function ReviewReplyForm({ reviewId }: ReviewReplyFormProps) {
  const [content, setContent] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [isPending, startTransition] = useTransition();

  const onSubmit = (event: React.FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    setError(null);

    startTransition(async () => {
      const result = await replyToReviewAction(reviewId, content);
      if (!result.ok) {
        setError(result.error ?? "Failed to post reply.");
        return;
      }
      setContent("");
    });
  };

  const remaining = MAX_LENGTH - content.length;
  const disabled = isPending || content.trim().length === 0;

  return (
    <form className="flex flex-col gap-3" onSubmit={onSubmit}>
      <label
        htmlFor={`reply-${reviewId}`}
        className="text-[11px] font-semibold uppercase tracking-[0.16em] text-foreground-tertiary"
      >
        Write a reply
      </label>
      <textarea
        id={`reply-${reviewId}`}
        value={content}
        onChange={(e) => setContent(e.target.value)}
        rows={4}
        maxLength={MAX_LENGTH}
        placeholder="Respond to this review…"
        className="w-full resize-y rounded-sm border border-border-subtle bg-background-elevated px-3 py-2 text-sm text-foreground placeholder:text-foreground-tertiary focus:border-[color:var(--moss-700)] focus:outline-none"
        disabled={isPending}
      />
      <div className="flex items-center justify-between gap-3">
        <span
          className={`text-[11px] ${
            remaining < 0 ? "text-[color:var(--danger,#8e1a1a)]" : "text-foreground-tertiary"
          }`}
        >
          {remaining} characters remaining
        </span>
        <button
          type="submit"
          disabled={disabled}
          className="inline-flex min-h-[2.75rem] items-center rounded-sm bg-[color:var(--ink-900)] px-4 py-2 text-xs font-semibold uppercase tracking-wider text-[color:var(--paper-200)] transition-colors hover:bg-[color:var(--moss-700)] disabled:cursor-not-allowed disabled:opacity-40"
        >
          {isPending ? "Posting…" : "Post reply"}
        </button>
      </div>
      {error && (
        <p
          role="alert"
          className="text-xs text-[color:var(--danger,#8e1a1a)]"
        >
          {error}
        </p>
      )}
    </form>
  );
}
