import type { ReactNode } from "react";
import Link from "next/link";

export interface BrandBarProps {
  /** Home href. Defaults to "/" */
  homeHref?: string;
  /** Wordmark text. Defaults to "mark8ly" */
  wordmark?: string;
  /** Optional trailing slot (e.g. a sign-out button, tenant switcher). */
  trailing?: ReactNode;
}

/**
 * BrandBar — slim top bar used by every "funnel-style" page
 * across the Mark8ly apps: onboarding flow, admin auth pages,
 * post-submit screens, etc.
 *
 * Layout: wordmark-only home link on the left, optional trailing
 * slot on the right. No nav items, no CTAs, no glassmorphism —
 * the wordmark IS the home link and that's the only chrome.
 *
 * The consuming app supplies the fonts via its own next/font
 * setup and imports the mark8ly-tokens.css so `text-foreground`
 * and `border-border-subtle` resolve to the paper/ink/moss system.
 */
export function BrandBar({
  homeHref = "/",
  wordmark = "mark8ly",
  trailing,
}: BrandBarProps) {
  return (
    <header className="border-b border-border-subtle bg-background">
      <div className="mx-auto flex h-[64px] max-w-6xl items-center justify-between px-6">
        <Link
          href={homeHref}
          aria-label={`${wordmark} — home`}
          className="-mx-2 inline-flex items-center px-2 py-2"
        >
          <span className="font-serif text-[1.5rem] font-medium tracking-[-0.025em] text-foreground">
            {wordmark}
          </span>
        </Link>
        {trailing ? (
          <div className="flex items-center gap-2">{trailing}</div>
        ) : null}
      </div>
    </header>
  );
}
