"use client";

// ErrorState — consistent error fallback used by every route-level
// `error.tsx` boundary in the admin app. Mirrors the AdminPage header
// chrome so the page doesn't collapse when an error bubbles up: same
// eyebrow, same serif, same spacing rhythm, same max-width.
//
// Keep this file client-only so it can render inside error boundaries
// (which must be client components in App Router).

import Link from "next/link";
import { AlertCircle, ArrowLeft, RefreshCw } from "lucide-react";

interface ErrorStateProps {
  /** Short eyebrow — e.g. "Error", "Something went wrong". */
  eyebrow?: string;
  /** Large, human-readable headline. */
  title?: string;
  /** Optional description under the title. Be honest — don't hide what happened. */
  description?: string;
  /**
   * Raw error object from the Next.js error boundary. Its digest is
   * shown in a monospace footer so users can cite it in support.
   */
  error?: Error & { digest?: string };
  /** Called when the user hits "Try again". Usually `reset` from error.tsx. */
  onRetry?: () => void;
  /** Optional "Back to …" link shown next to Try again. */
  backHref?: string;
  backLabel?: string;
}

export function ErrorState({
  eyebrow = "Error",
  title = "Something went wrong",
  description = "We couldn't load this page. You can try again, or head back and try a different route.",
  error,
  onRetry,
  backHref,
  backLabel = "Go back",
}: ErrorStateProps) {
  return (
    <div className="mx-auto w-full max-w-6xl space-y-10">
      <header className="space-y-4">
        <p className="eyebrow text-danger">{eyebrow}</p>
        <h1 className="font-serif text-4xl font-medium tracking-tight text-foreground text-balance sm:text-5xl">
          {title}
        </h1>
        <p className="max-w-2xl text-base leading-7 text-foreground-secondary">
          {description}
        </p>
      </header>

      <div className="rounded-md border border-border-subtle bg-[color:var(--background-elevated)] p-6">
        <div className="flex items-start gap-4">
          <AlertCircle
            className="mt-1 h-5 w-5 shrink-0 text-danger"
            aria-hidden="true"
          />
          <div className="min-w-0 flex-1 space-y-4">
            <div className="space-y-1.5">
              <p className="text-sm font-medium text-foreground">
                {error?.message || "Unexpected error"}
              </p>
              {error?.digest && (
                <p className="font-mono text-xs text-foreground-tertiary">
                  {error.digest}
                </p>
              )}
            </div>

            <div className="flex flex-wrap items-center gap-2">
              {onRetry && (
                <button
                  type="button"
                  onClick={onRetry}
                  className="inline-flex items-center gap-2 rounded-md bg-[color:var(--ink-900)] px-4 py-2 text-sm font-medium text-[color:var(--primary-foreground)] transition-colors hover:bg-[color:var(--moss-700)] focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)]"
                >
                  <RefreshCw className="h-3.5 w-3.5" aria-hidden="true" />
                  Try again
                </button>
              )}
              {backHref && (
                <Link
                  href={backHref}
                  className="inline-flex items-center gap-2 rounded-md border border-[color:var(--ink-900)]/10 px-4 py-2 text-sm font-medium text-foreground transition-colors hover:border-[color:var(--ink-900)]/25 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)]"
                >
                  <ArrowLeft className="h-3.5 w-3.5" aria-hidden="true" />
                  {backLabel}
                </Link>
              )}
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}
