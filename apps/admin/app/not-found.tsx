import Link from "next/link";

export default function NotFound() {
  return (
    <div className="grid min-h-screen grid-rows-[1fr_auto] bg-[color:var(--background)]">
      <main className="flex items-center px-6 py-24 sm:px-12 lg:px-24">
        <div className="w-full max-w-2xl space-y-6">
          <div className="space-y-1">
            <p className="font-mono text-[11px] font-medium uppercase tracking-[0.2em] text-foreground-tertiary">
              Error 404
            </p>
            <div className="h-px w-12 bg-border" aria-hidden="true" />
          </div>
          <h1 className="font-[family-name:var(--font-editorial-serif)] text-[clamp(2rem,5vw,3.5rem)] font-medium leading-[1.1] tracking-tight text-foreground">
            This page doesn&rsquo;t exist
          </h1>
          <p className="max-w-md text-[15px] leading-relaxed text-foreground-secondary">
            The page you followed may have been removed, renamed, or never
            existed in the first place. Check the URL or return to your
            dashboard.
          </p>
          <nav className="flex items-center gap-6 pt-2">
            <Link
              href="/"
              className="inline-flex h-10 items-center rounded-[var(--radius)] bg-[color:var(--ink-900)] px-5 text-sm font-medium text-white transition-colors duration-200 hover:bg-[color:var(--ink-800)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[color:var(--moss-700)] focus-visible:ring-offset-2 focus-visible:ring-offset-[color:var(--background)]"
            >
              Back to dashboard
            </Link>
            <Link
              href="/settings"
              className="text-sm text-foreground-secondary underline underline-offset-4 decoration-border-subtle transition-colors duration-200 hover:text-foreground hover:decoration-foreground-tertiary"
            >
              Settings
            </Link>
          </nav>
        </div>
      </main>
      <footer className="border-t border-border-subtle px-6 py-5 sm:px-12 lg:px-24">
        <p className="text-xs text-foreground-tertiary">mark8ly admin</p>
      </footer>
    </div>
  );
}
