/**
 * BannerStack — priority-selective banner slot for the admin shell.
 *
 * Renders AT MOST ONE banner at a time, selected by the highest-priority
 * active state. Priority order (highest first):
 *   1. PaymentActionRequired — time-sensitive 14-day window (§4.7)
 *   2. FailedPayment         — past_due dunning, read-only access
 *   3. Arbitrage             — open geo-pricing flag, 5-day SLA
 *   4. Trial                 — informational, positive state
 *
 * Each banner component already returns null for its own inactive state.
 * BannerStack adds the priority gate so only the highest-ranking active
 * banner ever mounts — the others are never rendered.
 *
 * Accessibility: the active banner is wrapped in a landmark region so
 * screen-reader users can jump directly to system notifications.
 *
 * Performance: useMemo on the active-banner key so identity is stable
 * across re-renders that don't change the active state.
 */
'use client'

import * as React from 'react'
import { usePaymentActionRequiredBannerActive } from './PaymentActionRequiredBanner'
import { useFailedPaymentBannerActive } from './FailedPaymentBanner'
import { useArbitrageBannerActive } from './ArbitrageBanner'
import { useTrialBannerActive } from './TrialBanner'
import { PaymentActionRequiredBanner } from './PaymentActionRequiredBanner'
import { FailedPaymentBanner } from './FailedPaymentBanner'
import { ArbitrageBanner } from './ArbitrageBanner'
import { TrialBanner } from './TrialBanner'

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

interface BannerStackProps {
  storeId: string
}

type BannerKey =
  | 'payment_action_required'
  | 'failed_payment'
  | 'arbitrage'
  | 'trial'
  | null

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

export function BannerStack({ storeId }: BannerStackProps) {
  const isPaymentActionRequired = usePaymentActionRequiredBannerActive(storeId)
  const isFailedPayment = useFailedPaymentBannerActive(storeId)
  const isArbitrage = useArbitrageBannerActive(storeId)
  const isTrial = useTrialBannerActive(storeId)

  const activeBanner = React.useMemo<BannerKey>(() => {
    if (isPaymentActionRequired) return 'payment_action_required'
    if (isFailedPayment) return 'failed_payment'
    if (isArbitrage) return 'arbitrage'
    if (isTrial) return 'trial'
    return null
  }, [isPaymentActionRequired, isFailedPayment, isArbitrage, isTrial])

  if (activeBanner === null) return null

  return (
    <div role="region" aria-label="System notification">
      {activeBanner === 'payment_action_required' && (
        <PaymentActionRequiredBanner storeId={storeId} />
      )}
      {activeBanner === 'failed_payment' && (
        <FailedPaymentBanner storeId={storeId} />
      )}
      {activeBanner === 'arbitrage' && (
        <ArbitrageBanner storeId={storeId} />
      )}
      {activeBanner === 'trial' && (
        <TrialBanner storeId={storeId} />
      )}
    </div>
  )
}
