import Link from "next/link";

export default function NotFound() {
  return (
    <main id="main" className="grid min-h-[70vh] grid-rows-[1fr_auto]">
      <div className="flex items-end px-6 pb-16 pt-24 sm:px-12 lg:px-24">
        <div className="w-full max-w-xl space-y-6">
          <div className="space-y-1">
            <p className="font-mono text-[11px] font-medium uppercase tracking-[0.2em] text-foreground-tertiary">
              Error 404
            </p>
            <div className="h-px w-12 bg-border" aria-hidden="true" />
          </div>
          <h1 className="font-serif text-[clamp(2rem,5vw,3.5rem)] font-medium leading-[1.1] tracking-tight text-foreground">
            Page not found
          </h1>
          <p className="max-w-md text-[15px] leading-relaxed text-foreground-secondary">
            This page doesn&rsquo;t exist. If you&rsquo;re setting up your
            store, head back to the beginning.
          </p>
          <div className="pt-2">
            <Link
              href="/"
              className="inline-flex h-11 items-center rounded-[var(--radius)] bg-[color:var(--ink-900)] px-5 text-sm font-medium text-[color:var(--paper-50,#fff)] transition-colors duration-200 hover:bg-[color:var(--ink-800)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[color:var(--moss-700)] focus-visible:ring-offset-2 focus-visible:ring-offset-[color:var(--background)]"
            >
              Start onboarding
            </Link>
          </div>
        </div>
      </div>
    </main>
  );
}
