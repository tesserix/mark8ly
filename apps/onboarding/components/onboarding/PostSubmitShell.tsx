import type { ReactNode } from "react";
import Link from "next/link";

import { SlimFooter } from "./SlimFooter";

interface PostSubmitShellProps {
  eyebrow: string;
  title: string;
  description: string;
  /**
   * Position in the 3-step funnel (1 = signup form, 2 = verify email,
   * 3 = set password). When set, a quiet "Step N of 3" indicator with
   * progress dots renders above the eyebrow so the user always knows
   * where they are and how much is left (NN/g heuristic #1 — visibility
   * of system status). Omit on terminal pages like /welcome.
   */
  step?: 1 | 2 | 3;
  children: ReactNode;
}

const TOTAL_STEPS = 3;

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
  step,
  children,
}: PostSubmitShellProps) {
  return (
    <div className="flex min-h-screen flex-col bg-background text-foreground">
      {/* Slim brand bar — wordmark is the home link */}
      <header className="border-b border-border-subtle">
        <div className="mx-auto flex h-[64px] max-w-6xl items-center px-6">
          <Link
            href="/"
            aria-label="mark8ly — home"
            className="-mx-2 inline-flex items-center px-2 py-2"
          >
            <span className="font-serif text-[1.5rem] font-medium tracking-[-0.025em] text-foreground">
              mark8ly
            </span>
          </Link>
        </div>
      </header>

      <main
        id="main"
        className="flex-1 motion-safe:animate-[fadeInUp_0.35s_ease-out_both]"
      >
        <section className="mx-auto max-w-3xl px-6 pb-16 pt-16 sm:pb-20 sm:pt-24">
          {step && (
            <div className="mb-6 flex items-center gap-3">
              <div className="flex items-center gap-1.5" aria-hidden="true">
                {Array.from({ length: TOTAL_STEPS }, (_, i) => (
                  <span
                    key={i}
                    className={`h-1.5 rounded-full transition-all duration-300 ${
                      i < step
                        ? "w-6 bg-moss-700"
                        : "w-1.5 bg-border-subtle"
                    }`}
                  />
                ))}
              </div>
              <p className="text-xs font-medium uppercase tracking-[0.14em] text-foreground-tertiary">
                Step {step} of {TOTAL_STEPS}
              </p>
            </div>
          )}
          <p className="eyebrow mb-5">{eyebrow}</p>
          <h1 className="font-serif text-4xl font-medium leading-[1.05] tracking-[-0.02em] text-foreground">
            {title}
          </h1>
          <p className="mt-6 max-w-xl text-lg leading-[1.55] text-foreground-secondary">
            {description}
          </p>
        </section>

        <section className="mx-auto max-w-3xl px-6 pb-24">{children}</section>
      </main>

      <SlimFooter />
    </div>
  );
}
