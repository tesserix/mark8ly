/**
 * ArbitrageBanner — shown in the admin shell when the store has an open
 * geo-pricing arbitrage flag.
 *
 * Design: Paper · Ink · Moss. Tone 'info' — calm and editorial, not alarming.
 * Voice: "We've noted a discrepancy" not "Fraud detected!".
 *
 * Returns null when the store is not flagged.
 */
'use client'

import { BannerShell } from './BannerShell'
import { useArbitrageFlag } from '@/lib/api/subscription/hooks/useArbitrage'
import { subscriptionCopy } from '@/lib/copy/subscription'

interface ArbitrageBannerProps {
  storeId: string
}

const copy = subscriptionCopy.arbitrage.banner

/**
 * Returns true when ArbitrageBanner would render (store has an open arbitrage
 * flag and the query has resolved). Used by BannerStack for priority selection.
 */
export function useArbitrageBannerActive(storeId: string): boolean {
  const { flagged, isLoading } = useArbitrageFlag(storeId)
  return !isLoading && flagged
}

export function ArbitrageBanner({ storeId }: ArbitrageBannerProps) {
  const { flagged, isLoading } = useArbitrageFlag(storeId)

  if (isLoading || !flagged) return null

  return (
    <BannerShell
      tone="info"
      heading={copy.heading}
      body={copy.body}
      cta={{
        label: copy.cta,
        href: '/admin/settings/arbitrage',
      }}
    />
  )
}
