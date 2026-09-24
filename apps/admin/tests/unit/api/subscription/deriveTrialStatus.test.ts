/**
 * deriveTrialStatus unit tests.
 *
 * These did not exist, which is how the banner shipped contradicting itself
 * in a single sentence:
 *
 *   "5 days left in your trial
 *    Your trial ends 31 December 2027. Add a payment method today to avoid
 *    losing access."
 *
 * The heading came from daysSinceSignup — which assumes every trial is
 * exactly 90 days from the subscription row's created_at — while the body
 * rendered the real trialEndsAt. TrialBanner.test.tsx mocks useTrialStatus
 * outright, so it asserted that a given variant renders a given heading and
 * never that the variant was the right one.
 *
 * Same defect #353 fixed on the server, where the reminder cron used to
 * "work backwards from a fixed trial length and bucket on created_at".
 */

import { describe, it, expect } from 'vitest'
import { deriveTrialStatus } from '@/lib/api/subscription/hooks/useTrial'

const NOW = new Date('2026-09-24T00:00:00Z')

function daysFrom(base: Date, days: number): string {
  return new Date(base.getTime() + days * 86_400_000).toISOString()
}

/** A trialing plan created `signedUpDaysAgo` ago, ending on `endsAt`. */
function plan(signedUpDaysAgo: number, endsAt: string | null) {
  return {
    status: 'trialing',
    createdAt: daysFrom(NOW, -signedUpDaysAgo),
    trialEndsAt: endsAt,
  } as never
}

describe('deriveTrialStatus banner selection', () => {
  it('counts down to the real end, not to 90 days after signup', () => {
    // The Bondi Store: signed up long ago, trial extended to 2027-12-31.
    // daysSinceSignup is ~200 (>= 85, the old day85 threshold) while the
    // trial genuinely has over a year to run.
    const status = deriveTrialStatus(plan(200, '2027-12-31T00:00:00Z'), NOW)

    expect(status.isTrialing).toBe(true)
    expect(status.daysUntilTrialEnd).toBeGreaterThan(400)
    expect(
      status.bannerVariant,
      'an extended trial must not be told it has 5 days left',
    ).toBe('none')
  })

  it('never shows a heading that contradicts daysUntilTrialEnd', () => {
    // The property the bug violated: the heading's claim and the body's date
    // describe the same trial, so they cannot disagree.
    const ceilings: Record<string, number> = { day85: 5, day75: 15, day60: 30 }

    for (const signedUpDaysAgo of [1, 30, 60, 75, 85, 120, 200, 400]) {
      for (const endsInDays of [-1, 0, 3, 5, 10, 15, 20, 30, 45, 200, 500]) {
        const status = deriveTrialStatus(
          plan(signedUpDaysAgo, daysFrom(NOW, endsInDays)),
          NOW,
        )
        if (status.bannerVariant === 'none') continue
        expect(
          status.daysUntilTrialEnd,
          `variant ${status.bannerVariant} claims <=${ceilings[status.bannerVariant]} days ` +
            `but the trial has ${status.daysUntilTrialEnd}`,
        ).toBeLessThanOrEqual(ceilings[status.bannerVariant])
      }
    }
  })

  it('still fires at each milestone of an ordinary 90-day trial', () => {
    // The behaviour that was already correct, pinned so the fix does not
    // silence the banner it was meant to correct.
    expect(deriveTrialStatus(plan(60, daysFrom(NOW, 30)), NOW).bannerVariant).toBe('day60')
    expect(deriveTrialStatus(plan(75, daysFrom(NOW, 15)), NOW).bannerVariant).toBe('day75')
    expect(deriveTrialStatus(plan(85, daysFrom(NOW, 5)), NOW).bannerVariant).toBe('day85')
    expect(deriveTrialStatus(plan(30, daysFrom(NOW, 60)), NOW).bannerVariant).toBe('none')
  })

  it('falls back to the 90-day assumption only when there is no trialEndsAt', () => {
    // Null trialEndsAt is the one case where created_at is all there is.
    expect(deriveTrialStatus(plan(86, null), NOW).bannerVariant).toBe('day85')
    expect(deriveTrialStatus(plan(10, null), NOW).bannerVariant).toBe('none')
  })

  it('returns nothing for a plan that is not trialing', () => {
    const closed = { status: 'store_closed', createdAt: daysFrom(NOW, -200), trialEndsAt: null } as never
    expect(deriveTrialStatus(closed, NOW).isTrialing).toBe(false)
    expect(deriveTrialStatus(closed, NOW).bannerVariant).toBe('none')
  })
})
