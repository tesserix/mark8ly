"use client";

import Link from "next/link";

interface HeaderProps {
  currentPage?: "home" | "pricing" | "onboarding" | "other";
}

/**
 * Marketing site header — matches the inline header used in the legacy
 * mark8ly_backup landing page (Features / Pricing / Stories / Demo nav,
 * dark "Start Free" pill on the right). Sticky on scroll.
 */
export function Header({ currentPage = "other" }: HeaderProps) {
  function navClass() {
    return "rounded-full px-4 py-2 text-sm text-foreground-secondary hover:bg-warm-50 hover:text-foreground transition-colors";
  }

  return (
    <header className="fixed top-0 left-0 right-0 z-50 border-b border-warm-200/70 bg-background/82 backdrop-blur-xl supports-[backdrop-filter]:bg-background/68">
      <div className="max-w-6xl mx-auto px-6 h-20 flex items-center justify-between gap-6">
        <Link
          href="/"
          className="flex items-center gap-3 hover:opacity-80 transition-opacity"
        >
          {/* eslint-disable-next-line @next/next/no-img-element */}
          <img
            src="/icon-192.png"
            alt="mark8ly icon"
            className="h-10 w-auto object-contain rounded-xl"
          />
          <div>
            <span className="block text-[1.375rem] font-serif font-medium tracking-[-0.025em] text-foreground">
              mark8ly
            </span>
          </div>
        </Link>

        <nav className="hidden md:flex items-center gap-2 rounded-full border border-warm-200/80 bg-white/78 px-3 py-2 shadow-sm">
          <Link href="/#features" className={navClass()}>
            Features
          </Link>
          <Link href="/#pricing" className={navClass()}>
            Pricing
          </Link>
          <Link href="/#testimonial" className={navClass()}>
            Stories
          </Link>
          <Link href="/presentation" className={navClass()}>
            Demo
          </Link>
        </nav>

        <Link
          href="/onboarding"
          className="hidden sm:inline-flex items-center gap-2 rounded-full bg-primary px-5 py-2.5 text-sm font-medium text-primary-foreground shadow-[0_12px_24px_rgba(31,30,28,0.18)] transition-[background-color,box-shadow,transform] hover:bg-primary-hover hover:shadow-[0_16px_30px_rgba(31,30,28,0.22)]"
        >
          <span className="h-2 w-2 rounded-full bg-white/70" aria-hidden />
          Start Free
        </Link>
      </div>
    </header>
  );
}
