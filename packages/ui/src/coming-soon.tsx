// packages/ui/src/coming-soon.tsx
//
// Placeholder rendered by every stub admin route until its real
// implementation lands. Previously lived in apps/admin/components/shell/;
// promoted here so onboarding and storefront apps can reuse the same
// shell if/when they need stub routes.
//
// Lives inside the consumer's shell — do NOT wrap in AdminShell here.
// The consumer owns the chrome.

// Uses a plain anchor rather than next/link: this package declares only
// react/react-dom and `next` is not hoisted to the workspace root, so
// importing next/link broke `tsc --noEmit` here and in apps/admin. Adding
// `next` would mean editing the root lockfile, which cannot be regenerated
// locally. This is a stub-route placeholder, so a document navigation on the
// back link is fine.

import { ArrowRight } from "lucide-react";

export interface ComingSoonProps {
  title: string;
  description: string;
  eta?: string;
  /** Where the "Back to ..." link navigates. Defaults to /dashboard. */
  backHref?: string;
  /** Label of the back link. Defaults to "Back to dashboard". */
  backLabel?: string;
}

export function ComingSoon({
  title,
  description,
  eta,
  backHref = "/dashboard",
  backLabel = "Back to dashboard",
}: ComingSoonProps) {
  return (
    <div className="mx-auto max-w-4xl">
      <div className="space-y-4">
        <p className="eyebrow">Coming soon</p>
        <h1 className="font-serif text-4xl font-medium tracking-tight text-foreground sm:text-5xl">
          {title}
        </h1>
        <p className="max-w-2xl text-base leading-7 text-foreground-secondary">
          {description}
        </p>
        {eta && (
          <p className="text-sm text-foreground-tertiary">
            Expected:{" "}
            <span className="font-medium text-foreground">{eta}</span>
          </p>
        )}
      </div>

      <div className="mt-10 border-t border-border-subtle pt-8">
        <p className="eyebrow">Why this exists</p>
        <div className="mt-3 max-w-2xl space-y-3 text-sm leading-7 text-foreground-secondary">
          <p>
            The route is already wired so the information architecture stays
            stable while features land.
          </p>
          <p>
            New slices can plug into a finished shell instead of being
            redesigned each time.
          </p>
        </div>
        <div className="mt-8">
          <a
            href={backHref}
            className="inline-flex h-12 items-center gap-2 rounded-md bg-primary px-6 text-sm font-medium text-primary-foreground hover:bg-primary-hover"
          >
            {backLabel}
            <ArrowRight className="h-4 w-4" aria-hidden="true" />
          </a>
        </div>
      </div>
    </div>
  );
}
