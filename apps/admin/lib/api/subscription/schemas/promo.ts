/**
 * Zod schemas for the promo-code redemption wire types.
 *
 * Source of truth:
 *   services/marketplace-api/internal/handlers/admin/promo.go
 *   services/marketplace-api/internal/promo/public_reason.go
 *
 * Request:  POST /api/admin/stores/:storeId/subscription/apply-promo
 *   Body:   { code: string }
 *
 * The body carries no email. The Go handler counts the per-email redemption
 * cap against the subscription's own billing address; a client-supplied
 * address would let one merchant spend another's cap.
 *
 * Response 200: { stripe_coupon_id, effective_minor, currency,
 *                 percent_off_bps, max_duration_months }
 * Response 422: { error: "promo_invalid_or_expired", message, reason }
 */
import { z } from 'zod'

// ---------------------------------------------------------------------------
// Request
// ---------------------------------------------------------------------------

export const applyPromoRequestSchema = z.object({
  /**
   * The code the merchant typed. Sent as-is: the server upper-cases and
   * trims before its constant-time comparison, so normalising here would
   * only risk drifting from that.
   */
  code: z.string().min(1),
})

export type ApplyPromoRequest = z.infer<typeof applyPromoRequestSchema>

// ---------------------------------------------------------------------------
// Response
// ---------------------------------------------------------------------------

export const applyPromoResponseSchema = z.object({
  /** Stripe Coupon backing the code, or "" when the code has none. */
  stripe_coupon_id: z.string().default(''),
  /** Recurring price after the discount, in minor units. */
  effective_minor: z.number().int(),
  /**
   * Lower-case ISO 4217 for `effective_minor`. Comes from the server because
   * the GET subscription DTO does not populate billing_currency — the admin
   * schema defaults that missing field to USD, which would print an INR
   * price with a dollar sign.
   */
  currency: z.string().default(''),
  /** Percentage discount in basis points (2000 = 20%). 0 = not a percentage code. */
  percent_off_bps: z.number().int().default(0),
  /** Months the discount runs for. 0 means the row states no bound — never "zero months". */
  max_duration_months: z.number().int().default(0),
})

export type ApplyPromoResponse = z.infer<typeof applyPromoResponseSchema>

// ---------------------------------------------------------------------------
// Rejection reasons
// ---------------------------------------------------------------------------

/**
 * The machine-readable reasons the server will return on a 422.
 *
 * This is the PUBLIC set, and it is one smaller than the validator's internal
 * set: `not_found` and `expired` both arrive as `invalid_or_expired`. That
 * merge is deliberate on the server — telling the two apart would confirm
 * which code strings are real to anyone typing them — so there is no client
 * copy for either, and asking for one is asking to undo it.
 */
export const PROMO_REJECT_REASONS = [
  'invalid_or_expired',
  'max_redemptions_reached',
  'max_per_email_reached',
  'wrong_plan',
  'annual_only',
  'below_absolute_floor',
  'currency_not_covered',
  'unknown_discount_type',
] as const

export type PromoRejectReason = (typeof PROMO_REJECT_REASONS)[number]

/**
 * Narrows an unknown `reason` string from an error body.
 *
 * An unrecognised reason — an older server, or a reason added after this
 * build — falls back to `invalid_or_expired`, whose copy is the one sentence
 * that is safe to show without knowing anything.
 */
export function toPromoRejectReason(reason: unknown): PromoRejectReason {
  return (PROMO_REJECT_REASONS as readonly string[]).includes(reason as string)
    ? (reason as PromoRejectReason)
    : 'invalid_or_expired'
}
