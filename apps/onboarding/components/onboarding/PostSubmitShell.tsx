import type { ReactNode } from "react";
import Link from "next/link";

import { SlimFooter } from "./SlimFooter";

interface PostSubmitShellProps {
  eyebrow: string;
  title: string;
  description: string;
  children: ReactNode;
}

/**
 * PostSubmitShell — wrapper used by every page after the
 * onboarding form has been submitted: /onboarding/check-inbox,
 * /onboarding/set-password, /onboarding/verify, /welcome.
 *
 * Layout: slim brand bar → editorial hero (eyebrow + serif
 * title + lede) → children → slim footer. Single column,
 * left-aligned, no card chrome, no blur blobs, no fake
 * decoration.
 */
export function PostSubmitShell({
  eyebrow,
  title,
  description,
  children,
}: PostSubmitShellProps) {
  return (
    <div className="flex min-h-screen flex-col bg-background text-foreground">
      {/* Slim brand bar */}
      <header className="border-b border-border-subtle">
        <div className="mx-auto flex h-[64px] max-w-6xl items-center justify-between px-6">
          <Link
            href="/"
            aria-label="mark8ly — home"
            className="-mx-2 inline-flex items-center px-2 py-2"
          >
            <span className="font-serif text-xl font-medium tracking-[-0.025em] text-foreground">
              mark8ly
            </span>
          </Link>
          <Link
            href="/"
            className="inline-flex h-10 items-center px-3 text-sm text-foreground-secondary hover:text-foreground"
          >
            <span aria-hidden="true" className="mr-2">
              ←
            </span>
            Back to home
          </Link>
        </div>
      </header>

      <main id="main" className="flex-1">
        <section className="mx-auto max-w-3xl px-6 pb-16 pt-16 sm:pb-20 sm:pt-24">
          <p className="eyebrow mb-5">{eyebrow}</p>
          <h1
            className="font-serif font-medium text-foreground"
            style={{
              fontSize: "var(--text-4xl)",
              lineHeight: 1.05,
              letterSpacing: "-0.02em",
            }}
          >
            {title}
          </h1>
          <p
            className="mt-6 max-w-xl text-foreground-secondary"
            style={{ fontSize: "var(--text-lg)", lineHeight: 1.55 }}
          >
            {description}
          </p>
        </section>

        <section className="mx-auto max-w-3xl px-6 pb-24">{children}</section>
      </main>

      <SlimFooter />
    </div>
  );
}
