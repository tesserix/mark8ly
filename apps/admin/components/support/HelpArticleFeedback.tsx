"use client";

import { useState } from "react";
import { ThumbsUp, ThumbsDown } from "lucide-react";

export function HelpArticleFeedback() {
  const [submitted, setSubmitted] = useState(false);

  if (submitted) {
    return (
      <p className="text-sm text-foreground-secondary" role="status">
        Thank you for your feedback.
      </p>
    );
  }

  return (
    <div className="flex items-center gap-4">
      <p className="text-sm text-foreground-secondary">
        Was this helpful?
      </p>
      <button
        type="button"
        onClick={() => setSubmitted(true)}
        className="inline-flex h-10 items-center gap-1.5 rounded-md border border-border bg-background-elevated px-3 text-sm text-foreground-secondary hover:border-border-strong hover:text-foreground"
        aria-label="Yes, this was helpful"
      >
        <ThumbsUp className="h-3.5 w-3.5" aria-hidden="true" />
        Yes
      </button>
      <button
        type="button"
        onClick={() => setSubmitted(true)}
        className="inline-flex h-10 items-center gap-1.5 rounded-md border border-border bg-background-elevated px-3 text-sm text-foreground-secondary hover:border-border-strong hover:text-foreground"
        aria-label="No, this was not helpful"
      >
        <ThumbsDown className="h-3.5 w-3.5" aria-hidden="true" />
        No
      </button>
    </div>
  );
}
