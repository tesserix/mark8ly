/**
 * TrialBanner — renders a soft nudge banner during the 90-day trial period.
 *
 * Variants:
 *   day60 (days 60–74 since signup) — "30 days left in your trial"
 *   day75 (days 75–84 since signup) — "2 weeks left in your trial"
 *   day85 (days 85+  since signup) — "5 days left in your trial"
 *
 * Returns null when bannerVariant === 'none' (pre-day-60 or not trialing).
 *
 * Design (Paper · Ink · Moss):
 *   - Tone 'info' — trial is a positive state, never warning/danger.
 *   - No urgency. No "!". No emoji.
 *   - CTA → Stripe Customer Portal via useOpenPortal.
 *
 * Wiring: Task 28 (BannerStack) handles injection into the layout.
 * Do NOT wire this directly into PageShell here.
 */
'use client'

import * as React from 'react'
import { BannerShell } from './BannerShell'
import { useTrialStatus } from '@/lib/api/subscription/hooks/useTrial'
import { useOpenPortal } from '@/lib/api/subscription/hooks/useBilling'
import { subscriptionCopy } from '@/lib/copy/subscription'
import { formatBillingDate } from '@/lib/format/date'
import type { TrialBannerVariant } from '@/lib/api/subscription/hooks/useTrial'

// ---------------------------------------------------------------------------
// Props
// ---------------------------------------------------------------------------

interface TrialBannerProps {
  storeId: string
}

// ---------------------------------------------------------------------------
// Variant → copy helpers
// ---------------------------------------------------------------------------

const { banner: bannerCopy } = subscriptionCopy.trial

interface VariantCopy {
  heading: string
  body: string
}

function buildVariantCopy(
  variant: TrialBannerVariant,
  trialEndsAt: Date | null,
): VariantCopy | null {
  const endsAt = trialEndsAt ? formatBillingDate(trialEndsAt) : 'the end of your trial'

  switch (variant) {
    case 'day60':
      return {
        heading: bannerCopy.day60Heading,
        body: bannerCopy.day60BodyTemplate(endsAt),
      }
    case 'day75':
      return {
        heading: bannerCopy.day75Heading,
        body: bannerCopy.day75BodyTemplate(endsAt),
      }
    case 'day85':
      return {
        heading: bannerCopy.day85Heading,
        body: bannerCopy.day85BodyTemplate(endsAt),
      }
    case 'none':
      return null
  }
}

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

/**
 * Returns true when TrialBanner would render (bannerVariant !== 'none').
 * Used by BannerStack to determine priority without double-rendering.
 */
export function useTrialBannerActive(storeId: string): boolean {
  const { bannerVariant } = useTrialStatus(storeId)
  return bannerVariant !== 'none'
}

export function TrialBanner({ storeId }: TrialBannerProps) {
  const { bannerVariant, trialEndsAt } = useTrialStatus(storeId)
  const openPortal = useOpenPortal(storeId)

  if (bannerVariant === 'none') return null

  const copy = buildVariantCopy(bannerVariant, trialEndsAt)
  if (!copy) return null

  return (
    <div data-testid="trial-banner">
      <BannerShell
        tone="info"
        heading={copy.heading}
        body={copy.body}
        cta={{
          label: bannerCopy.cta,
          onClick: () => openPortal.mutate(),
        }}
      />
    </div>
  )
}
