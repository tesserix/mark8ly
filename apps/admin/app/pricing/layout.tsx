/**
 * Public pricing layout — no admin shell, no tenant extraction.
 *
 * Minimal public wrapper: Mark8ly wordmark masthead + hairline footer.
 * Font variables are inherited from root layout (Source Serif 4 + Source Sans 3).
 * Background: --paper-200 (the canonical Mark8ly page background).
 */
import type { ReactNode } from 'react'
import Link from 'next/link'

import { MailLink } from '@repo/ui/mail-link'

interface PricingLayoutProps {
  children: ReactNode
}

export default function PricingLayout({ children }: PricingLayoutProps) {
  return (
    <div
      className="min-h-screen flex flex-col"
      style={{ background: 'var(--paper-200)', color: 'var(--ink-900)' }}
    >
      {/* Masthead — text-only wordmark, Source Serif 4 */}
      <header className="px-8 py-6 border-b border-[var(--hairline,var(--ink-100))]">
        <Link
          href="/"
          aria-label="Mark8ly home"
          className="inline-block focus:outline-none focus-visible:ring-2 focus-visible:ring-[var(--moss-700)] rounded-sm"
        >
          <span
            className="text-2xl tracking-tight"
            style={{
              fontFamily: 'var(--font-editorial-serif, serif)',
              color: 'var(--ink-900)',
            }}
          >
            Mark8ly
          </span>
        </Link>
      </header>

      {/* Page content */}
      <main id="main-content" className="flex-1">
        {children}
      </main>

      {/* Footer — hairline separator, small Source Sans 3 links */}
      <footer
        className="px-8 py-6 border-t border-[var(--hairline,var(--ink-100))]"
        style={{ color: 'var(--ink-600)' }}
      >
        <nav aria-label="Footer" className="flex gap-6 text-sm">
          <MailLink
            email="hello@mark8ly.com"
            className="hover:text-[var(--moss-700)] transition-colors focus:outline-none focus-visible:ring-2 focus-visible:ring-[var(--moss-700)] rounded-sm"
          >
            Contact
          </MailLink>
          <Link
            href="/privacy"
            className="hover:text-[var(--moss-700)] transition-colors focus:outline-none focus-visible:ring-2 focus-visible:ring-[var(--moss-700)] rounded-sm"
          >
            Privacy
          </Link>
          <Link
            href="/terms"
            className="hover:text-[var(--moss-700)] transition-colors focus:outline-none focus-visible:ring-2 focus-visible:ring-[var(--moss-700)] rounded-sm"
          >
            Terms
          </Link>
        </nav>
      </footer>
    </div>
  )
}
