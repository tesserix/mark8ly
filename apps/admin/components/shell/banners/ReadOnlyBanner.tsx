/**
 * ReadOnlyBanner — shown for the terminal billing states that make the
 * admin read-only: `expired`, `store_closed` and `pending_hard_delete`.
 *
 * These three were the ONLY billing states with no banner. The result was
 * that a merchant whose trial lapsed got no explanation anywhere in the
 * product — writes failed, and (until marketplace-api#919) reads failed too,
 * surfacing as a generic "Something went wrong" error boundary. The state
 * the merchant most needs explaining was the one state nothing explained.
 *
 * Priority: highest in BannerStack. Every other banner describes a state you
 * can still trade in; these describe one you cannot.
 *
 * Tone `warning` rather than anything louder, and the copy stays calm. The
 * merchant has just lost access to their own shop; urgency language on top
 * of that reads as gloating.
 *
 * Returns null for every other status, including `past_due` and
 * `payment_action_required` — those have their own banners and, importantly,
 * their own DIFFERENT access levels. Do not merge them.
 */
'use client'

import * as React from 'react'
import { BannerShell } from './BannerShell'
import { useCurrentPlan, useOpenPortal } from '@/lib/api/subscription/hooks/useBilling'
import { subscriptionCopy } from '@/lib/copy/subscription'

// ---------------------------------------------------------------------------
// Props
// ---------------------------------------------------------------------------

interface ReadOnlyBannerProps {
  storeId: string
}

/**
 * The statuses readonly.RequireActive treats as read-only, mirrored from
 * services/marketplace-api/internal/subscription/readonly. Kept as a literal
 * union so adding a state server-side without adding it here is a type error
 * at the call site rather than a silently missing banner.
 */
export type ReadOnlyStatus = 'expired' | 'store_closed' | 'pending_hard_delete'

const READ_ONLY_STATUSES: readonly ReadOnlyStatus[] = [
  'expired',
  'store_closed',
  'pending_hard_delete',
]

export function isReadOnlyStatus(status: string | undefined): status is ReadOnlyStatus {
  return READ_ONLY_STATUSES.includes(status as ReadOnlyStatus)
}

// ---------------------------------------------------------------------------
// Hook
// ---------------------------------------------------------------------------

/**
 * Returns the read-only status when the subscription is in one, else null.
 * Used by BannerStack to determine priority without double-rendering.
 */
export function useReadOnlyStatus(storeId: string): ReadOnlyStatus | null {
  const { data: plan } = useCurrentPlan(storeId)
  const status = plan?.status
  return isReadOnlyStatus(status) ? status : null
}

export function useReadOnlyBannerActive(storeId: string): boolean {
  return useReadOnlyStatus(storeId) !== null
}

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

function copyFor(status: ReadOnlyStatus) {
  const copy = subscriptionCopy.banners.readOnly
  switch (status) {
    case 'expired':
      return copy.expired
    case 'store_closed':
      return copy.storeClosed
    case 'pending_hard_delete':
      return copy.pendingHardDelete
  }
}

export function ReadOnlyBanner({ storeId }: ReadOnlyBannerProps) {
  const status = useReadOnlyStatus(storeId)
  const openPortal = useOpenPortal(storeId)

  if (status === null) return null

  const copy = copyFor(status)

  return (
    <div data-testid="read-only-banner" data-status={status}>
      <BannerShell
        tone="warning"
        heading={copy.heading}
        body={copy.body}
        cta={{
          label: subscriptionCopy.banners.readOnly.cta,
          onClick: () => openPortal.mutate(),
        }}
      />
    </div>
  )
}
