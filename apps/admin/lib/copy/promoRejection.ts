/**
 * Maps a machine-readable promo rejection reason onto the sentence the
 * merchant reads (#770).
 *
 * A separate module from the component so the mapping can be tested without
 * rendering anything — the sentences ARE the feature, and a test that only
 * checks the happy path would miss all of it.
 *
 * The switch is exhaustive over PromoRejectReason with no default arm, so
 * adding a ninth public reason to the schema fails the type-check here
 * rather than silently falling back to the one sentence that says the least.
 */
import { subscriptionCopy } from '@/lib/copy/subscription'
import type { PromoRejectReason } from '@/lib/api/subscription/schemas/promo'

const copy = subscriptionCopy.promo.rejected

/** Context two of the reasons need in order to name the merchant's own plan. */
export interface PromoRejectionContext {
  /** Plan slug, e.g. "starter". Rendered into the wrong_plan and floor sentences. */
  plan: string
}

/**
 * The sentence for `reason`.
 *
 * `wrong_plan` and `below_absolute_floor` name the plan because that is the
 * fact that makes them actionable: without it, "does not apply to your plan"
 * gives the merchant nothing to change.
 */
export function promoRejectionMessage(
  reason: PromoRejectReason,
  ctx: PromoRejectionContext,
): string {
  switch (reason) {
    case 'invalid_or_expired':
      return copy.invalidOrExpired
    case 'max_redemptions_reached':
      return copy.maxRedemptionsReached
    case 'max_per_email_reached':
      return copy.maxPerEmailReached
    case 'wrong_plan':
      return copy.wrongPlan(ctx.plan)
    case 'annual_only':
      return copy.annualOnly
    case 'below_absolute_floor':
      return copy.belowAbsoluteFloor(ctx.plan)
    case 'currency_not_covered':
      return copy.currencyNotCovered
    case 'unknown_discount_type':
      return copy.unknownDiscountType
  }
}

/**
 * Whether the refusal means the merchant should change plan or billing
 * period, which the panel turns into a link to the plan-change screen.
 *
 * These two are the only reasons a merchant can resolve themselves without
 * talking to us. Every other reason either needs support or needs nothing.
 */
export function promoRejectionSuggestsPlanChange(
  reason: PromoRejectReason,
): boolean {
  return reason === 'wrong_plan' || reason === 'annual_only'
}
