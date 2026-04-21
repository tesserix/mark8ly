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
            <p className="font-mono text-[11px] font-medium uppercase tracking-[0.2em] text-[color:var(--signal)]">
              Something went wrong
            </p>
            <div className="h-px w-12 bg-[color:var(--signal)]/30" aria-hidden="true" />
          </div>
          <h1 className="font-serif text-[clamp(2rem,5vw,3.5rem)] font-medium leading-[1.1] tracking-tight text-foreground">
            Unexpected error
          </h1>
          <p className="max-w-md text-[15px] leading-relaxed text-foreground-secondary">
            We hit a problem during onboarding. Don&rsquo;t worry&thinsp;&mdash;&thinsp;your
            progress is saved. Please try again.
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
              className="inline-flex h-11 items-center rounded-[var(--radius)] bg-[color:var(--ink-900)] px-5 text-sm font-medium text-[color:var(--paper-50,#fff)] transition-colors duration-200 hover:bg-[color:var(--ink-800)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[color:var(--moss-700)] focus-visible:ring-offset-2 focus-visible:ring-offset-[color:var(--background)]"
            >
              Try again
            </button>
          </div>
        </div>
      </div>
    </main>
  );
}
