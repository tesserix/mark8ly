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
  return (
    <header className="fixed top-0 left-0 right-0 z-50 bg-background border-b border-border">
      <div className="max-w-6xl mx-auto px-6 h-20 flex items-center justify-between">
        <Link href="/" className="flex items-center hover:opacity-80 transition-opacity">
          {/* eslint-disable-next-line @next/next/no-img-element */}
          <img
            src="/icon-192.png"
            alt="mark8ly icon"
            className="h-9 w-auto object-contain"
          />
          <span className="text-[1.375rem] font-serif font-medium tracking-[-0.015em] text-foreground-secondary">
            mark8ly
          </span>
        </Link>

        <nav className="hidden md:flex items-center gap-8">
          <Link
            href="/#features"
            className={
              currentPage === "home"
                ? "text-sm text-foreground font-medium transition-colors"
                : "text-sm text-foreground-secondary hover:text-foreground transition-colors"
            }
          >
            Features
          </Link>
          <Link
            href="/#pricing"
            className="text-sm text-foreground-secondary hover:text-foreground transition-colors"
          >
            Pricing
          </Link>
          <Link
            href="/#testimonial"
            className="text-sm text-foreground-secondary hover:text-foreground transition-colors"
          >
            Stories
          </Link>
          <Link
            href="/presentation"
            className="text-sm text-foreground-secondary hover:text-foreground transition-colors"
          >
            Demo
          </Link>
        </nav>

        <Link
          href="/onboarding"
          className="hidden sm:block bg-primary text-primary-foreground px-5 py-2 rounded-lg text-sm font-medium hover:bg-primary-hover transition-colors"
        >
          Start Free
        </Link>
      </div>
    </header>
  );
}
