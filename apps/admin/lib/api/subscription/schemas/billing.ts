/**
 * Zod schemas for the billing/subscription wire types.
 *
 * Source of truth: services/marketplace-api/internal/handlers/admin/subscription.go
 *
 * The Go handler (SubscriptionResponse / toSubscriptionResponse) populates:
 *   id, store_id, plan, status, current_period_start, current_period_end,
 *   cancel_at_period_end, stripe_subscription_id, created_at,
 *   arbitrage_flag, latest_arbitrage_audit.
 *
 * Fields like billing_currency, add_ons, trial_ends_at, next_invoice_amount_minor,
 * payment_method_brand, payment_method_last4 are NOT in the current Go DTO.
 * They are defined here with safe `.optional()` / nullable defaults so the
 * schema can be tightened when the backend ships those fields without a
 * breaking change in the frontend.
 */
import { z } from 'zod'

// ---------------------------------------------------------------------------
// Subscription status — mirrors SubscriptionStatus in SubscriptionStatusBadge
// ---------------------------------------------------------------------------

export const subscriptionStatusSchema = z.enum([
  'signup',
  'trialing',
  'active',
  'past_due',
  'payment_action_required',
  'cancel_scheduled',
  'expired',
  'store_closed',
  'pending_hard_delete',
])

export type SubscriptionStatus = z.infer<typeof subscriptionStatusSchema>

// ---------------------------------------------------------------------------
// Arbitrage audit summary (returned on the GET subscription endpoint, §18.8.1)
// ---------------------------------------------------------------------------

export const arbitrageAuditSummarySchema = z.object({
  card_country: z.string(),
  billing_country: z.string(),
  ip_country: z.string(),
  resolution: z.string(),
  flagged_at: z.string(),
  mismatch_reason: z.string(),
})

export type ArbitrageAuditSummary = z.infer<typeof arbitrageAuditSummarySchema>

// ---------------------------------------------------------------------------
// GET /api/v1/admin/stores/:storeId/subscription
// ---------------------------------------------------------------------------

export const subscriptionResponseSchema = z.object({
  /** UUID */
  id: z.string(),
  /** UUID */
  store_id: z.string(),
  /** Plan slug: starter | growth | scale | pro */
  plan: z.string(),
  /**
   * Subscription status. The backend may return legacy values like "active",
   * "trialing", "past_due", "cancelled", "incomplete" from the Stripe mapping.
   * We passthrough unknown strings so old tenants don't crash the UI.
   */
  status: subscriptionStatusSchema.or(z.string()),
  current_period_start: z.string().nullable().optional(),
  current_period_end: z.string().nullable().optional(),
  cancel_at_period_end: z.boolean(),
  stripe_subscription_id: z.string().nullable().optional(),
  created_at: z.string(),
  arbitrage_flag: z.boolean(),
  latest_arbitrage_audit: arbitrageAuditSummarySchema.nullable().optional(),

  // ─── Fields not yet in the Go DTO ─────────────────────────────────────
  // Safe defaults so code that reads them doesn't crash; remove the
  // .optional() once the backend ships each field.

  /** ISO 4217 — e.g. "USD", "GBP". Backend does not yet populate this. */
  billing_currency: z.string().optional(),

  /** Add-on IDs active on this subscription, e.g. ["white_label_app"]. */
  add_ons: z.array(z.string()).optional(),

  /** ISO date string; null when not in trial. */
  trial_ends_at: z.string().nullable().optional(),

  /** Amount due on the next invoice, in minor units (cents). */
  next_invoice_amount_minor: z.number().int().nullable().optional(),

  /** Stripe card brand, e.g. "visa", "mastercard". */
  payment_method_brand: z.string().nullable().optional(),

  /** Last 4 digits of the card on file. */
  payment_method_last4: z.string().nullable().optional(),
})

export type SubscriptionResponse = z.infer<typeof subscriptionResponseSchema>

/**
 * CurrentPlan — the domain type the UI actually works with.
 * Normalises nullable period fields to a consistent shape.
 */
export interface CurrentPlan {
  id: string
  storeId: string
  plan: string
  status: string
  periodStart: string | null
  periodEnd: string | null
  cancelAtPeriodEnd: boolean
  arbitrageFlag: boolean
  billingCurrency: string
  addOns: string[]
  trialEndsAt: string | null
  nextInvoiceAmountMinor: number | null
  paymentMethodBrand: string | null
  paymentMethodLast4: string | null
}

export function toCurrentPlan(raw: SubscriptionResponse): CurrentPlan {
  return {
    id: raw.id,
    storeId: raw.store_id,
    plan: raw.plan,
    status: raw.status,
    periodStart: raw.current_period_start ?? null,
    periodEnd: raw.current_period_end ?? null,
    cancelAtPeriodEnd: raw.cancel_at_period_end,
    arbitrageFlag: raw.arbitrage_flag,
    billingCurrency: raw.billing_currency ?? 'USD',
    addOns: raw.add_ons ?? [],
    trialEndsAt: raw.trial_ends_at ?? null,
    nextInvoiceAmountMinor: raw.next_invoice_amount_minor ?? null,
    paymentMethodBrand: raw.payment_method_brand ?? null,
    paymentMethodLast4: raw.payment_method_last4 ?? null,
  }
}

// ---------------------------------------------------------------------------
// POST /api/v1/admin/stores/:storeId/subscription/portal
// Returns { url: string } — the Go handler writes gin.H{"url": url}
// ---------------------------------------------------------------------------

export const portalResponseSchema = z.object({
  /** Stripe Customer Portal session URL. */
  url: z.string().url(),
})

export type PortalResponse = z.infer<typeof portalResponseSchema>
