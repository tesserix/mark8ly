import Link from "next/link";

export default function NotFound() {
  return (
    <main className="grid min-h-[70vh] grid-rows-[1fr_auto]">
      <div className="flex items-end px-6 pb-16 pt-24 sm:px-12 lg:px-24">
        <div className="w-full max-w-xl space-y-6">
          <div className="space-y-1">
            <p className="font-mono text-[11px] font-medium uppercase tracking-[0.2em] text-foreground-tertiary">
              Error 404
            </p>
            <div className="h-px w-12 bg-border" aria-hidden="true" />
          </div>
          <h1 className="font-[family-name:var(--storefront-heading-font,var(--font-editorial-serif))] text-[clamp(2rem,5vw,3.5rem)] font-medium leading-[1.1] tracking-tight text-foreground">
            Page not found
          </h1>
          <p className="max-w-md text-[15px] leading-relaxed text-foreground-secondary">
            We couldn&rsquo;t find what you were looking for. The product may
            have been removed, or the link might be outdated.
          </p>
          <nav className="flex items-center gap-6 pt-2">
            <Link
              href="/"
              className="inline-flex h-10 items-center rounded-[var(--radius)] bg-[color:var(--storefront-primary,var(--ink-900))] px-5 text-sm font-medium text-white transition-colors duration-200 hover:opacity-90 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[color:var(--storefront-accent,var(--moss-700))] focus-visible:ring-offset-2 focus-visible:ring-offset-[color:var(--storefront-background,var(--background))]"
            >
              Continue shopping
            </Link>
            <Link
              href="/categories"
              className="text-sm text-foreground-secondary underline underline-offset-4 decoration-border-subtle transition-colors duration-200 hover:text-foreground hover:decoration-foreground-tertiary"
            >
              Browse categories
            </Link>
          </nav>
        </div>
      </div>
    </main>
  );
}
