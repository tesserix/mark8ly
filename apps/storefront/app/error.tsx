"use client";

interface ErrorProps {
  error: Error & { digest?: string };
  reset: () => void;
}

export default function Error({ error, reset }: ErrorProps) {
  return (
    <main id="main" className="grid min-h-[70vh] grid-rows-[1fr_auto]">
      <div className="flex items-end px-6 pb-16 pt-24 sm:px-12 lg:px-24">
        <div className="w-full max-w-xl space-y-6 motion-safe:animate-[fadeInUp_0.4s_ease-out]">
          <div className="space-y-1">
            <p className="font-mono text-[11px] font-medium uppercase tracking-[0.2em] text-[color:var(--storefront-danger)]">
              Something went wrong
            </p>
            <div className="h-px w-12 bg-[color:var(--storefront-danger)]/30" aria-hidden="true" />
          </div>
          <h1 className="font-[family-name:var(--storefront-heading-font,var(--font-editorial-serif))] text-[clamp(2rem,5vw,3.5rem)] font-medium leading-[1.1] tracking-tight text-foreground">
            We hit a snag
          </h1>
          <p className="max-w-md text-[15px] leading-relaxed text-foreground-secondary">
            Something unexpected happened while loading this page. Please try
            again&thinsp;&mdash;&thinsp;if the problem persists, we&rsquo;re
            already on it.
          </p>
          {error.digest && (
            <p className="font-mono text-[11px] tabular-nums text-foreground-tertiary">
              ref&ensp;{error.digest}
            </p>
          )}
          <div className="pt-2">
            <button
              type="button"
              onClick={reset}
              className="inline-flex h-11 items-center rounded-[var(--radius)] bg-[color:var(--storefront-primary,var(--ink-900))] px-5 text-sm font-medium text-[color:var(--storefront-on-accent,var(--paper-200))] transition-opacity duration-150 hover:opacity-90 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--storefront-accent,var(--moss-700))]"
            >
              Try again
            </button>
          </div>
        </div>
      </div>
    </main>
  );
}
