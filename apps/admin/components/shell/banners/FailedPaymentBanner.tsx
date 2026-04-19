/**
 * FailedPaymentBanner — shown when a subscription is in the `past_due` state.
 *
 * Spec §16 (dunning flow):
 *   - Status `past_due` → storefront live, admin fully editable.
 *   - Stripe Smart Retries on days 1, 3, 5, 7.
 *   - Banner tone: `warning` (amber-bronze left border).
 *   - CTA: "Add a new card" → opens Stripe Customer Portal.
 *
 * IMPORTANT distinction (spec §4.7):
 *   `past_due` = payment DID fail; dunning in progress.
 *   `payment_action_required` = payment PENDING bank confirmation; full access.
 *   These are two different states with different copy and tone.
 *
 * Returns null when the subscription is not in `past_due` state.
 */
'use client'

import * as React from 'react'
import { BannerShell } from './BannerShell'
import { usePastDueState } from '@/lib/api/subscription/hooks/useDunning'
import { useOpenPortal } from '@/lib/api/subscription/hooks/useBilling'
import { subscriptionCopy } from '@/lib/copy/subscription'
import { formatBillingDate } from '@/lib/format/date'

// ---------------------------------------------------------------------------
// Props
// ---------------------------------------------------------------------------

interface FailedPaymentBannerProps {
  storeId: string
}

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

export function FailedPaymentBanner({ storeId }: FailedPaymentBannerProps) {
  const { isPastDue, retryAt } = usePastDueState(storeId)
  const openPortal = useOpenPortal(storeId)

  if (!isPastDue) return null

  const copy = subscriptionCopy.banners.failedPayment
  const retryDateStr = retryAt ? formatBillingDate(retryAt) : 'a future date'
  const bodyText = copy.bodyTemplate(retryDateStr)

  function handleCtaClick() {
    openPortal.mutate()
  }

  return (
    <BannerShell
      tone="warning"
      heading={copy.heading}
      body={bodyText}
      cta={{
        label: copy.cta,
        onClick: handleCtaClick,
      }}
    />
  )
}
