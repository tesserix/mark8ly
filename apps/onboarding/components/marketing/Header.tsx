"use client";

import { useEffect, useState } from "react";
import Link from "next/link";

interface NavItem {
  href: string;
  label: string;
}

const navItems: NavItem[] = [
  { href: "/#features", label: "Features" },
  { href: "/#pricing", label: "Pricing" },
  { href: "/#faq", label: "Questions" },
  { href: "/about", label: "About" },
];

/**
 * Marketing header.
 *
 * Solid background (no glassmorphism). Single hairline rule
 * separates it from the page. Logo on the left, nav in the
 * middle on desktop, primary CTA always visible. On mobile a
 * hamburger opens a full-height sheet that contains the same
 * nav AND the CTA — nothing is amputated below `md`.
 *
 * Touch targets are ≥44px on every interactive surface.
 */
export function Header() {
  const [mobileOpen, setMobileOpen] = useState(false);

  // Lock body scroll when the mobile sheet is open.
  useEffect(() => {
    if (typeof document === "undefined") return;
    const original = document.body.style.overflow;
    document.body.style.overflow = mobileOpen ? "hidden" : original;
    return () => {
      document.body.style.overflow = original;
    };
  }, [mobileOpen]);

  // Close on Escape.
  useEffect(() => {
    if (!mobileOpen) return;
    function onKey(e: KeyboardEvent) {
      if (e.key === "Escape") setMobileOpen(false);
    }
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [mobileOpen]);

  return (
    <header className="sticky top-0 z-40 border-b border-border-subtle bg-background">
      <div className="mx-auto flex h-[72px] max-w-6xl items-center justify-between gap-6 px-6">
        <Link
          href="/"
          aria-label="mark8ly — home"
          className="-mx-2 inline-flex items-center px-2 py-2"
          onClick={() => setMobileOpen(false)}
        >
          <span className="font-serif text-[1.5rem] font-medium tracking-[-0.025em] text-foreground">
            mark8ly
          </span>
        </Link>

        <nav
          aria-label="Primary"
          className="hidden md:flex items-center gap-1"
        >
          {navItems.map((item) => (
            <Link
              key={item.href}
              href={item.href}
              className="inline-flex h-11 items-center px-4 text-[0.9375rem] text-foreground-secondary hover:text-foreground"
            >
              {item.label}
            </Link>
          ))}
        </nav>

        <div className="flex items-center gap-2">
          <Link
            href="/onboarding"
            className="inline-flex h-11 items-center rounded-md bg-primary px-5 text-[0.9375rem] font-medium text-primary-foreground hover:bg-primary-hover"
          >
            Start free
          </Link>

          <button
            type="button"
            aria-label={mobileOpen ? "Close menu" : "Open menu"}
            aria-expanded={mobileOpen}
            aria-controls="mobile-nav-sheet"
            onClick={() => setMobileOpen((v) => !v)}
            className="md:hidden inline-flex h-11 w-11 items-center justify-center rounded-md text-foreground hover:bg-paper-300"
          >
            <svg
              width="22"
              height="22"
              viewBox="0 0 24 24"
              fill="none"
              stroke="currentColor"
              strokeWidth="1.75"
              strokeLinecap="round"
              aria-hidden="true"
            >
              {mobileOpen ? (
                <path d="M6 6l12 12M6 18L18 6" />
              ) : (
                <path d="M4 7h16M4 12h16M4 17h16" />
              )}
            </svg>
          </button>
        </div>
      </div>

      {/* Mobile sheet */}
      <div
        id="mobile-nav-sheet"
        hidden={!mobileOpen}
        className="md:hidden border-t border-border-subtle bg-background"
      >
        <nav
          aria-label="Mobile primary"
          className="mx-auto flex max-w-6xl flex-col gap-1 px-6 py-4"
        >
          {navItems.map((item) => (
            <Link
              key={item.href}
              href={item.href}
              onClick={() => setMobileOpen(false)}
              className="flex h-12 items-center text-base text-foreground"
            >
              {item.label}
            </Link>
          ))}
        </nav>
      </div>
    </header>
  );
}
