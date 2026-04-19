'use client'

import Link from 'next/link'
import type { CurrentPlan } from '@/lib/api/subscription/schemas/billing'
import { subscriptionCopy } from '@/lib/copy/subscription'

interface WhiteLabelAppCardProps {
  plan: CurrentPlan
}

const copy = subscriptionCopy.billing

/**
 * WhiteLabelAppCard — shown only for Pro merchants.
 *
 * Visibility rules:
 *   - Hide entirely if plan !== 'pro'.
 *   - If Pro + white_label_app add-on: show active state.
 *   - If Pro, no add-on: show upsell with Add CTA.
 *
 * Design: hairline at bottom only, Source Serif 4 H2.
 */
export function WhiteLabelAppCard({ plan }: WhiteLabelAppCardProps) {
  if (plan.plan !== 'pro') {
    return null
  }

  const hasAddon = plan.addOns.includes('white_label_app')

  return (
    <section
      aria-labelledby="white-label-app-heading"
      className="border-b border-[var(--hairline,var(--ink-100))] pb-10"
    >
      <h2
        id="white-label-app-heading"
        className="font-[family-name:var(--font-source-serif),'Source_Serif_4',serif] text-2xl font-medium tracking-tight text-[var(--ink-900)]"
      >
        {copy.whiteLabelAppHeading}
      </h2>

      <p className="mt-3 max-w-xl text-sm leading-6 text-[var(--ink-700)]">
        {hasAddon ? copy.whiteLabelAppActive : copy.whiteLabelAppUpsell}
      </p>

      <div className="mt-6">
        {hasAddon ? (
          <Link
            href="/settings/pro-app-purchase"
            className="inline-flex h-10 items-center rounded-[6px] border border-[var(--hairline,var(--ink-100))] bg-[var(--background-elevated)] px-5 text-sm font-medium text-[var(--ink-900)] transition-colors hover:bg-[var(--paper-200)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--moss-700)]"
          >
            {copy.whiteLabelAppCta}
          </Link>
        ) : (
          <Link
            href="/settings/billing/pro-app-purchase"
            className="inline-flex h-10 items-center rounded-[6px] bg-[var(--ink-900)] px-5 text-sm font-medium text-white transition-colors hover:bg-[var(--ink-900)]/90 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--moss-700)]"
          >
            {copy.whiteLabelAppAddCta}
          </Link>
        )}
      </div>
    </section>
  )
}
