/**
 * React Query hooks for trial state — derived from useCurrentPlan.
 *
 * Convention: derived hooks share the same cache as useCurrentPlan so no
 * additional network calls are issued; all state comes from the one query
 * keyed on ['subscription', storeId].
 *
 * Trial banner thresholds (days since signup):
 *   < 60  → 'none'   (no banner)
 *   60–74 → 'day60'  (30 days remaining nudge)
 *   75–84 → 'day75'  (two-week reminder)
 *   85+   → 'day85'  (five-day notice)
 *
 * daysSinceSignup is computed from CurrentPlan.createdAt (the subscription
 * row's created_at ISO string, which is the signup timestamp).
 * daysUntilTrialEnd is computed from trialEndsAt when available; falls back
 * to (90 - daysSinceSignup) using the 90-day trial length from spec §1/§5.
 */
'use client'

import { useCurrentPlan } from './useBilling'
import type { CurrentPlan } from '../schemas/billing'

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

export type TrialBannerVariant = 'none' | 'day60' | 'day75' | 'day85'

export interface TrialStatus {
  isTrialing: boolean
  daysSinceSignup: number
  daysUntilTrialEnd: number
  trialEndsAt: Date | null
  bannerVariant: TrialBannerVariant
}

// ---------------------------------------------------------------------------
// Constants
// ---------------------------------------------------------------------------

const MS_PER_DAY = 1_000 * 60 * 60 * 24
/** Total trial length per spec §1 / §5. */
const TRIAL_LENGTH_DAYS = 90

// ---------------------------------------------------------------------------
// Pure derivation — exported for unit testing
// ---------------------------------------------------------------------------

export function deriveTrialStatus(
  plan: CurrentPlan | undefined | null,
  now: Date = new Date(),
): TrialStatus {
  const EMPTY: TrialStatus = {
    isTrialing: false,
    daysSinceSignup: 0,
    daysUntilTrialEnd: 0,
    trialEndsAt: null,
    bannerVariant: 'none',
  }

  if (!plan) return EMPTY

  const isTrialing = plan.status === 'trialing'
  if (!isTrialing) return EMPTY

  const createdAt = new Date(plan.createdAt)
  if (isNaN(createdAt.getTime())) return EMPTY

  const daysSinceSignup = Math.max(
    0,
    Math.floor((now.getTime() - createdAt.getTime()) / MS_PER_DAY),
  )

  const trialEndsAt = plan.trialEndsAt ? new Date(plan.trialEndsAt) : null

  const daysUntilTrialEnd =
    trialEndsAt !== null && !isNaN(trialEndsAt.getTime())
      ? Math.max(
          0,
          Math.ceil((trialEndsAt.getTime() - now.getTime()) / MS_PER_DAY),
        )
      : Math.max(0, TRIAL_LENGTH_DAYS - daysSinceSignup)

  const bannerVariant = resolveBannerVariant(daysSinceSignup)

  return {
    isTrialing: true,
    daysSinceSignup,
    daysUntilTrialEnd,
    trialEndsAt,
    bannerVariant,
  }
}

function resolveBannerVariant(daysSinceSignup: number): TrialBannerVariant {
  if (daysSinceSignup >= 85) return 'day85'
  if (daysSinceSignup >= 75) return 'day75'
  if (daysSinceSignup >= 60) return 'day60'
  return 'none'
}

// ---------------------------------------------------------------------------
// Hook
// ---------------------------------------------------------------------------

/**
 * Returns trial status derived from the current subscription plan.
 *
 * Derived from useCurrentPlan — shares its cache and staleTime (30 s).
 * Returns `isTrialing: false` + `bannerVariant: 'none'` for non-trial plans.
 *
 * @param storeId - The store UUID. Pass an empty string to disable the query.
 */
export function useTrialStatus(storeId: string): TrialStatus {
  const { data: plan } = useCurrentPlan(storeId)
  return deriveTrialStatus(plan)
}
