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
 * The banner variant is chosen from daysUntilTrialEnd, so a trial whose end
 * has been moved counts down to the date the merchant is actually shown.
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

  const bannerVariant = resolveBannerVariant(daysUntilTrialEnd)

  return {
    isTrialing: true,
    daysSinceSignup,
    daysUntilTrialEnd,
    trialEndsAt,
    bannerVariant,
  }
}

/**
 * Chooses the banner from days REMAINING, not days since signup.
 *
 * The variant names are milestones of the default 90-day trial — day60 is
 * "30 days left", day75 is "2 weeks left", day85 is "5 days left" — and the
 * thresholds below are those same milestones expressed the way the copy
 * actually reads.
 *
 * It used to switch on daysSinceSignup, which silently assumed every trial
 * is exactly TRIAL_LENGTH_DAYS long starting at the subscription row's
 * created_at. Any trial whose end had been moved therefore got a countdown
 * computed from a date nobody had changed, while the BODY of the same banner
 * rendered the real trialEndsAt. The result was a banner contradicting
 * itself in one sentence:
 *
 *   "5 days left in your trial
 *    Your trial ends 31 December 2027. Add a payment method today to avoid
 *    losing access."
 *
 * This is the same defect #353 fixed on the server: the reminder cron used
 * to "work backwards from a fixed trial length and bucket on created_at,
 * which meant an operator-extended trial kept its original reminder schedule
 * and got nothing before its real end". The cron was corrected to work from
 * the effective end; this banner was not, and kept the old assumption.
 */
function resolveBannerVariant(daysUntilTrialEnd: number): TrialBannerVariant {
  if (daysUntilTrialEnd <= 5) return 'day85'
  if (daysUntilTrialEnd <= 15) return 'day75'
  if (daysUntilTrialEnd <= 30) return 'day60'
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
