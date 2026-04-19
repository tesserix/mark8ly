/**
 * PaymentActionRequiredBanner — shown when a subscription is in the
 * `payment_action_required` state (spec §4.7).
 *
 * CRITICAL DISTINCTION (spec §4.7 + Council finding #3):
 *   `payment_action_required` preserves FULL admin access.
 *   The merchant's bank needs to confirm the payment (3DS/SCA/RBI e-mandate).
 *   This is NOT a failed payment — the charge is pending bank confirmation.
 *   Copy and tone must reflect a calm, non-alarming, informational state.
 *   → Tone: `info` (NOT warning or danger).
 *
 * Countdown logic:
 *   daysLeft = max(1, 14 - daysSinceFlagged)
 *   daysLeft > 7  → T-14 body ("You have 14 days...")
 *   1 < daysLeft <= 7 → T-7 body ("You have 7 days...")
 *   daysLeft === 1  → T-1 body ("You have 1 day...")
 *
 * CTA: opens the Stripe hosted invoice URL in a new tab (rel="noopener noreferrer").
 * If hostedInvoiceUrl is not in the subscription data (backend field not yet
 * shipped), falls back to useCompleteActionUrl to fetch it via the redirect
 * endpoint.
 *
 * Returns null when the subscription is not in `payment_action_required` state.
 */
'use client'

import * as React from 'react'
import { BannerShell } from './BannerShell'
import {
  usePaymentActionRequiredState,
  useCompleteActionUrl,
} from '@/lib/api/subscription/hooks/useDunning'
import { subscriptionCopy } from '@/lib/copy/subscription'

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function pickBody(daysLeft: number): string {
  const copy = subscriptionCopy.banners.paymentActionRequired
  if (daysLeft > 7) return copy.t14Body
  if (daysLeft > 1) return copy.t7Body
  return copy.t1Body
}

function computeDaysLeft(daysSinceFlagged: number): number {
  return Math.max(1, 14 - daysSinceFlagged)
}

// ---------------------------------------------------------------------------
// Props
// ---------------------------------------------------------------------------

interface PaymentActionRequiredBannerProps {
  storeId: string
}

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

/**
 * Returns true when PaymentActionRequiredBanner would render (subscription is
 * in payment_action_required state). Used by BannerStack for priority selection.
 */
export function usePaymentActionRequiredBannerActive(storeId: string): boolean {
  const { isPending } = usePaymentActionRequiredState(storeId)
  return isPending
}

export function PaymentActionRequiredBanner({
  storeId,
}: PaymentActionRequiredBannerProps) {
  const { isPending, hostedInvoiceUrl, daysSinceFlagged } =
    usePaymentActionRequiredState(storeId)
  const completeActionMutation = useCompleteActionUrl(storeId)

  if (!isPending) return null

  const copy = subscriptionCopy.banners.paymentActionRequired
  const daysLeft = computeDaysLeft(daysSinceFlagged)
  const bodyText = pickBody(daysLeft)

  // When hostedInvoiceUrl is available on the subscription data (future backend
  // field), use it directly as an anchor href so the browser opens it without
  // a round-trip. Until the field lands, fall back to the mutation which calls
  // the complete-action redirect endpoint.
  const ctaProps = hostedInvoiceUrl
    ? {
        label: copy.cta,
        href: hostedInvoiceUrl,
        target: '_blank' as const,
        rel: 'noopener noreferrer',
      }
    : {
        label: copy.cta,
        onClick: () => {
          completeActionMutation.mutate()
        },
      }

  return (
    <BannerShell
      tone="info"
      heading={copy.heading}
      body={bodyText}
      cta={ctaProps}
    />
  )
}
