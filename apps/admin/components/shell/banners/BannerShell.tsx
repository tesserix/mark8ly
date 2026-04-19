/**
 * BannerShell — reusable shell for full-width admin top banners.
 *
 * Design spec (Paper · Ink · Moss):
 *   - Full-width, hairline-bottom bar. NO shadow. NO boxed card.
 *   - Left-aligned heading (Source Serif 4, semibold) + body (Source Sans 3, ink-700).
 *   - CTA right-aligned, Source Sans, underline on hover, moss-700 text.
 *   - Tone variants:
 *       warning  → amber-bronze left border (3px, --warning token)
 *       danger   → oxblood left border (3px, --danger token)
 *       info     → ink-400 hairline left border only (no accent)
 *   - Dismissible variant: X button at far right with aria-label "Dismiss".
 *   - Icon-free: no lucide icons, no emoji.
 *   - Responsive: single row on desktop, two rows on narrow widths.
 */
'use client'

import * as React from 'react'

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

export type BannerTone = 'warning' | 'info' | 'danger'

interface BannerCta {
  label: string
  onClick?: () => void
  href?: string
  /** When href is provided, set to "_blank" to open in a new tab. */
  target?: '_blank' | '_self'
  /** Companion to target="_blank"; defaults to "noopener noreferrer" when target="_blank". */
  rel?: string
}

export interface BannerShellProps {
  heading: string
  body: React.ReactNode
  cta?: BannerCta
  tone: BannerTone
  dismissible?: boolean
  onDismiss?: () => void
}

// ---------------------------------------------------------------------------
// Tone styling maps
// ---------------------------------------------------------------------------

const TONE_BORDER: Record<BannerTone, string> = {
  warning: 'border-l-[3px] border-l-[var(--warning)]',
  danger: 'border-l-[3px] border-l-[var(--danger)]',
  info: 'border-l border-l-[var(--hairline)]',
}

const TONE_BG: Record<BannerTone, string> = {
  warning: 'bg-[var(--paper-200)]',
  danger: 'bg-[var(--paper-200)]',
  info: 'bg-[var(--paper-200)]',
}

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

export function BannerShell({
  heading,
  body,
  cta,
  tone,
  dismissible = false,
  onDismiss,
}: BannerShellProps) {
  return (
    <div
      role="status"
      aria-live="polite"
      data-testid="banner-shell"
      data-tone={tone}
      className={[
        'w-full border-b border-b-[var(--hairline)]',
        TONE_BG[tone],
        TONE_BORDER[tone],
      ].join(' ')}
    >
      <div className="mx-auto flex flex-wrap items-baseline justify-between gap-x-6 gap-y-1 px-4 py-3 sm:px-6 lg:px-8">
        {/* Left: heading + body */}
        <div className="flex flex-wrap items-baseline gap-x-2 gap-y-0.5">
          <span
            className="font-serif text-sm font-semibold leading-snug text-[var(--ink-900)]"
            style={{ fontFamily: "'Source Serif 4', serif" }}
          >
            {heading}
          </span>
          <span
            className="text-sm leading-snug text-[var(--ink-700)]"
            style={{ fontFamily: "'Source Sans 3', sans-serif" }}
          >
            {body}
          </span>
        </div>

        {/* Right: CTA + optional dismiss */}
        <div className="flex shrink-0 items-center gap-4">
          {cta && (
            <CtaElement cta={cta} />
          )}
          {dismissible && (
            <button
              type="button"
              aria-label="Dismiss"
              onClick={onDismiss}
              className="text-[var(--ink-700)] transition-colors hover:text-[var(--ink-900)] focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[var(--moss-700)]"
            >
              <CloseIcon />
            </button>
          )}
        </div>
      </div>
    </div>
  )
}

// ---------------------------------------------------------------------------
// Internal helpers
// ---------------------------------------------------------------------------

function CtaElement({ cta }: { cta: BannerCta }) {
  const ctaClass =
    'text-sm font-medium text-[var(--moss-700)] underline underline-offset-2 decoration-[var(--moss-700)/60] hover:decoration-[var(--moss-700)] transition-colors focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[var(--moss-700)]'

  if (cta.href) {
    const rel =
      cta.rel ?? (cta.target === '_blank' ? 'noopener noreferrer' : undefined)
    return (
      <a
        href={cta.href}
        target={cta.target}
        rel={rel}
        className={ctaClass}
        style={{ fontFamily: "'Source Sans 3', sans-serif" }}
      >
        {cta.label}
      </a>
    )
  }

  return (
    <button
      type="button"
      onClick={cta.onClick}
      className={ctaClass}
      style={{ fontFamily: "'Source Sans 3', sans-serif" }}
    >
      {cta.label}
    </button>
  )
}

function CloseIcon() {
  return (
    <svg
      xmlns="http://www.w3.org/2000/svg"
      width="16"
      height="16"
      viewBox="0 0 16 16"
      fill="none"
      aria-hidden="true"
    >
      <path
        d="M12 4L4 12M4 4l8 8"
        stroke="currentColor"
        strokeWidth="1.5"
        strokeLinecap="round"
        strokeLinejoin="round"
      />
    </svg>
  )
}
