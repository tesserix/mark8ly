/**
 * Zod schemas for the plan-change endpoints (§4.4 + §4.5).
 *
 * Source of truth:
 *   services/marketplace-api/internal/handlers/admin/subscription_change_plan.go
 *   services/marketplace-api/internal/subscription/planchange/preflight.go
 *   services/marketplace-api/internal/subscription/planchange/planchange.go
 *
 * DISCOVERY: The preflight endpoint returns `PreflightReport` (not a simple
 * proration preview). It contains a `decision` enum, plan/period identifiers,
 * `effective_at`, and a `current_plan_diff` object. There is no
 * `proration_amount_minor` or `is_upgrade` boolean — proration is populated
 * later via Stripe webhook. The POST response uses `result` (not `status`)
 * and includes `effective_at` + `stripe_updated`.
 *
 * Query param for both GET and POST is `target_period` (not `billing_period`).
 */

import { z } from 'zod'
import { planIdSchema } from './plan'

// ---------------------------------------------------------------------------
// Shared enums
// ---------------------------------------------------------------------------

export const billingPeriodSchema = z.enum(['monthly', 'annual'])

export type BillingPeriod = z.infer<typeof billingPeriodSchema>

export const preflightDecisionSchema = z.enum([
  'allow_upgrade_now',
  'allow_downgrade_at_period_end',
  'allow_period_switch',
  'no_op',
  'block_read_only',
  'block_currency',
  'block_store_count',
])

export type PreflightDecision = z.infer<typeof preflightDecisionSchema>

// ---------------------------------------------------------------------------
// Preflight — GET /api/v1/admin/stores/:storeId/subscription/change-plan/preflight
// ---------------------------------------------------------------------------

/**
 * Feature-limit delta between current plan and target plan.
 * A value of -1 means "Unlimited / Negotiated" — do not render as a number.
 */
export const planDiffSchema = z.object({
  stores_delta: z.number().int(),
  images_per_product_delta: z.number().int(),
  campaign_emails_delta: z.number().int(),
})

export type PlanDiff = z.infer<typeof planDiffSchema>

export const storeInfoSchema = z.object({
  store_id: z.string(),
  name: z.string(),
  slug: z.string(),
  status: z.string(),
  in_flight_orders: z.number().int(),
  is_soft_deleted: z.boolean(),
})

export type StoreInfo = z.infer<typeof storeInfoSchema>

/**
 * PreflightReport — the full preflight response from the Go handler.
 *
 * Note: `effective_at` is a zero-value RFC3339 string for blocking decisions.
 * It is optional so callers don't need to guard against empty/zero values from
 * older API versions.
 */
export const preflightReportSchema = z.object({
  decision: preflightDecisionSchema,

  current_plan: planIdSchema.or(z.string()),
  current_period: billingPeriodSchema.or(z.string()),
  target_plan: planIdSchema.or(z.string()),
  target_period: billingPeriodSchema.or(z.string()),

  /** RFC3339 timestamp. Zero value ("0001-01-01T...") for blocking decisions. */
  effective_at: z.string().optional(),

  current_plan_diff: planDiffSchema,

  // Store-count quota fields — only populated for block_store_count decision.
  store_count: z.number().int().optional(),
  target_store_limit: z.number().int().optional(),
  stores: z.array(storeInfoSchema).optional(),
})

export type PreflightReport = z.infer<typeof preflightReportSchema>

// ---------------------------------------------------------------------------
// Apply plan change — POST /api/v1/admin/stores/:storeId/subscription/change-plan
// ---------------------------------------------------------------------------

export const planChangeResultSchema = z.enum([
  'upgrade_committed',
  'downgrade_scheduled',
  'period_switch_committed',
])

export type PlanChangeResult = z.infer<typeof planChangeResultSchema>

/**
 * Response from POST change-plan.
 *
 * `result` is "upgrade_committed" | "downgrade_scheduled" | "period_switch_committed".
 * `effective_at` is RFC3339.
 * `stripe_updated` is false for scheduled downgrades (Stripe is updated at
 *   period end).
 */
export const applyPlanChangeResponseSchema = z.object({
  result: planChangeResultSchema.or(z.string()),
  effective_at: z.string(),
  stripe_updated: z.boolean(),
})

export type ApplyPlanChangeResponse = z.infer<typeof applyPlanChangeResponseSchema>

// ---------------------------------------------------------------------------
// Request body
// ---------------------------------------------------------------------------

export const applyPlanChangeBodySchema = z.object({
  target_plan: planIdSchema,
  target_period: billingPeriodSchema,
  reason: z.string().optional(),
})

export type ApplyPlanChangeBody = z.infer<typeof applyPlanChangeBodySchema>

// ---------------------------------------------------------------------------
// Derived helpers
// ---------------------------------------------------------------------------

/**
 * Returns true when the decision means the change will take effect immediately
 * (upgrade or period switch).
 */
export function isImmediateChange(decision: PreflightDecision): boolean {
  return (
    decision === 'allow_upgrade_now' || decision === 'allow_period_switch'
  )
}

/**
 * Returns true when the decision is a blocking error (not actionable).
 */
export function isBlockingDecision(decision: PreflightDecision): boolean {
  return (
    decision === 'block_read_only' ||
    decision === 'block_currency' ||
    decision === 'block_store_count'
  )
}
