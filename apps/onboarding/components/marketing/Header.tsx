"use client";

import Link from "next/link";
import { Button } from "@tesserix/web";

/**
 * Marketing site header — shared across the landing page and the static
 * stub pages (about, contact, etc.). Intentionally NOT used by the
 * onboarding wizard at /onboarding, which is a focused single-purpose
 * surface.
 */
export function Header() {
  return (
    <header className="sticky top-0 z-40 w-full border-b border-zinc-200 dark:border-zinc-800 bg-white/80 dark:bg-zinc-950/80 backdrop-blur">
      <div className="mx-auto max-w-6xl flex h-16 items-center justify-between px-6">
        <Link
          href="/"
          className="text-xl font-semibold tracking-tight text-zinc-900 dark:text-zinc-50"
        >
          mark8ly
        </Link>

        <nav className="hidden md:flex items-center gap-8 text-sm text-zinc-600 dark:text-zinc-400">
          <Link href="/#features" className="hover:text-zinc-900 dark:hover:text-zinc-100 transition-colors">
            Features
          </Link>
          <Link href="/about" className="hover:text-zinc-900 dark:hover:text-zinc-100 transition-colors">
            About
          </Link>
          <Link href="/help" className="hover:text-zinc-900 dark:hover:text-zinc-100 transition-colors">
            Help
          </Link>
          <Link href="/contact" className="hover:text-zinc-900 dark:hover:text-zinc-100 transition-colors">
            Contact
          </Link>
        </nav>

        <Button asChild size="sm">
          <Link href="/onboarding">Get started</Link>
        </Button>
      </div>
    </header>
  );
}
