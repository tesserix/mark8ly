/**
 * Builds the confirmation sentence for a redeemed promo code (#770).
 *
 * Kept beside the rejection mapping and out of the component so what we
 * CLAIM about an applied discount is testable on its own. The rule it
 * enforces: state only what the response actually contains.
 */
import { subscriptionCopy } from '@/lib/copy/subscription'
import { formatMinorUnits } from '@/lib/format/minorUnits'
import type { ApplyPromoResponse } from '@/lib/api/subscription/schemas/promo'

const copy = subscriptionCopy.promo

/**
 * Renders basis points as a display percentage: 2000 → "20", 1250 → "12.5".
 * Returns null outside (0, 100), which the caller reads as "not a percentage
 * discount we can describe".
 *
 * Mirrors formatPercentBps in the Go win-back cron deliberately: the same
 * number reaches a merchant through two channels and must read the same in
 * both.
 */
function formatPercentBps(bps: number): string | null {
  if (!Number.isFinite(bps) || bps <= 0 || bps >= 10000) return null
  return String(Number((bps / 100).toFixed(2)))
}

/**
 * The confirmation sentence for `res`.
 *
 * Three shapes, in decreasing order of what the response lets us say:
 *
 *  1. A whole percentage AND a month bound — state the discount, how long it
 *     runs, and the new price.
 *  2. A price but no describable terms (a flat-amount code, or one with no
 *     bound on its duration) — state the new price only. Naming a duration
 *     here would be inventing one; `max_duration_months: 0` means the row
 *     sets no bound, not that the discount lasts zero months.
 *  3. No currency — quote no price at all rather than guessing one. The GET
 *     subscription DTO does not carry billing_currency and its schema
 *     defaults the gap to USD, so falling back to that would print an INR
 *     price with a dollar sign.
 *
 * Every shape says the discount starts on the NEXT invoice, because that is
 * what happens: applying a code attaches a Stripe coupon to the
 * subscription's discounts and nothing re-bills the current period.
 */
export function promoAppliedMessage(res: ApplyPromoResponse): string {
  if (!res.currency) {
    return copy.appliedNoPrice
  }

  const price = formatMinorUnits(res.effective_minor, res.currency)
  const percentOff = formatPercentBps(res.percent_off_bps)

  if (percentOff !== null && res.max_duration_months > 0) {
    return copy.appliedForMonths(percentOff, res.max_duration_months, price)
  }

  return copy.applied(price)
}
