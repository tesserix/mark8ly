/**
 * BannerStack — priority-selective banner slot for the admin shell.
 *
 * Renders AT MOST ONE banner at a time, selected by the highest-priority
 * active state. Priority order (highest first):
 *   1. ReadOnly              — expired / store_closed / pending_hard_delete
 *   2. PaymentActionRequired — time-sensitive 14-day window (§4.7)
 *   3. FailedPayment         — past_due dunning, read-only access
 *   4. Trial                 — informational, positive state
 *
 * ReadOnly outranks everything: the others describe states you can still
 * trade in, and it describes one you cannot. It was also, until now, the
 * only group of billing states with no banner at all — so the merchant who
 * most needed an explanation was the one who got none.
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
import { useReadOnlyBannerActive } from './ReadOnlyBanner'
import { usePaymentActionRequiredBannerActive } from './PaymentActionRequiredBanner'
import { useFailedPaymentBannerActive } from './FailedPaymentBanner'
import { useTrialBannerActive } from './TrialBanner'
import { ReadOnlyBanner } from './ReadOnlyBanner'
import { PaymentActionRequiredBanner } from './PaymentActionRequiredBanner'
import { FailedPaymentBanner } from './FailedPaymentBanner'
import { TrialBanner } from './TrialBanner'

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

interface BannerStackProps {
  storeId: string
}

type BannerKey =
  | 'read_only'
  | 'payment_action_required'
  | 'failed_payment'
  | 'trial'
  | null

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

export function BannerStack({ storeId }: BannerStackProps) {
  const isReadOnly = useReadOnlyBannerActive(storeId)
  const isPaymentActionRequired = usePaymentActionRequiredBannerActive(storeId)
  const isFailedPayment = useFailedPaymentBannerActive(storeId)
  const isTrial = useTrialBannerActive(storeId)

  const activeBanner = React.useMemo<BannerKey>(() => {
    if (isReadOnly) return 'read_only'
    if (isPaymentActionRequired) return 'payment_action_required'
    if (isFailedPayment) return 'failed_payment'
    if (isTrial) return 'trial'
    return null
  }, [isReadOnly, isPaymentActionRequired, isFailedPayment, isTrial])

  if (activeBanner === null) return null

  return (
    <div role="region" aria-label="System notification">
      {activeBanner === 'read_only' && (
        <ReadOnlyBanner storeId={storeId} />
      )}
      {activeBanner === 'payment_action_required' && (
        <PaymentActionRequiredBanner storeId={storeId} />
      )}
      {activeBanner === 'failed_payment' && (
        <FailedPaymentBanner storeId={storeId} />
      )}
      {activeBanner === 'trial' && (
        <TrialBanner storeId={storeId} />
      )}
    </div>
  )
}
