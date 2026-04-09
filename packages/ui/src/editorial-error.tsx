"use client";

// packages/ui/src/editorial-error.tsx
//
// Generic server-side error screen used by per-route `error.tsx`
// boundaries (Next App Router). Every admin section gets one — products,
// orders, customers, settings — so the copy, layout, and retry button
// stay consistent.
//
// Lives inside the consumer's shell. Callers wrap this in AdminShell
// or just drop it into a <main> — the component only renders the
// content, no chrome.

import type { ReactNode } from "react";

export interface EditorialErrorProps {
  title?: string;
  description?: string;
  /** Called when the user clicks "Try again". Typically Next's reset(). */
  onRetry?: () => void;
  /** Custom retry button label. */
  retryLabel?: string;
  /** Additional content (e.g., a "Go home" link) rendered after the retry button. */
  children?: ReactNode;
}

export function EditorialError({
  title = "Something went wrong",
  description = "Something went wrong on our side. Try again, or come back in a moment.",
  onRetry,
  retryLabel = "Try again",
  children,
}: EditorialErrorProps) {
  return (
    <section className="flex flex-col gap-4 px-8 py-16" aria-live="polite">
      <h1 className="font-[family-name:var(--font-serif,'Source_Serif_4',serif)] text-3xl text-[color:var(--ink-900)]">
        {title}
      </h1>
      <p className="max-w-prose text-[color:var(--ink-900)] opacity-70">
        {description}
      </p>
      {onRetry && (
        <button
          type="button"
          onClick={onRetry}
          className="inline-flex w-fit items-center gap-2 rounded-md bg-[color:var(--ink-900)] px-4 py-2 text-sm text-[color:var(--paper-200)] transition-opacity hover:opacity-90 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)]"
        >
          {retryLabel}
        </button>
      )}
      {children}
    </section>
  );
}
