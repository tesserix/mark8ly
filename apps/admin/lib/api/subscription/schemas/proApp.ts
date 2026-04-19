/**
 * Zod schemas for the Pro+App add-on purchase wire types.
 *
 * Source of truth:
 *   services/marketplace-api/internal/billing/appaddon/handler.go
 *
 * Key deviations from initial spec guess:
 *   - Request uses `apple_4_2_6_acknowledged` (bool) + `attestation_text` (string),
 *     NOT `accept_terms`.
 *   - Response returns `hosted_invoice_url`, `stripe_invoice_id`, `amount_due_cents`,
 *     `currency` on HTTP 201. There is no `status: 'purchased' | 'payment_action_required'`
 *     discriminator — a 201 always means the Stripe invoice was created. The merchant
 *     may still need to authenticate the payment via the hosted invoice URL.
 *   - No separate "get quote" endpoint exists. Proration context is derived client-side
 *     from the CurrentPlan data (periodEnd, plan).
 */
import { z } from 'zod'

// ---------------------------------------------------------------------------
// POST /api/v1/admin/stores/:storeId/subscription/add-on/white-label-app
// ---------------------------------------------------------------------------

export const purchaseProAppRequestSchema = z.object({
  /**
   * Must be true. The handler returns 400 apple_4_2_6_ack_required if false.
   * This is the merchant's Apple App Store guideline 4.2.6 acknowledgement.
   */
  apple_4_2_6_acknowledged: z.literal(true),
  /**
   * Verbatim attestation text shown to the merchant in the UI.
   * Must be non-empty — backend returns 400 attestation_text_required otherwise.
   */
  attestation_text: z.string().min(1),
})

export type PurchaseProAppRequest = z.infer<typeof purchaseProAppRequestSchema>

export const purchaseProAppResponseSchema = z.object({
  /**
   * Stripe-hosted invoice URL. The merchant may be redirected here if their
   * bank requires Strong Customer Authentication (SCA). If Stripe auto-collects
   * without SCA, the invoice is immediately paid and this URL is still returned
   * for reference (receipt).
   */
  hosted_invoice_url: z.string().url(),
  stripe_invoice_id: z.string(),
  /** Prorated amount in cents (USD). */
  amount_due_cents: z.number().int().nonnegative(),
  currency: z.string(),
})

export type PurchaseProAppResponse = z.infer<typeof purchaseProAppResponseSchema>

// ---------------------------------------------------------------------------
// Error codes the handler can return (for typed error handling in the UI)
// ---------------------------------------------------------------------------

export type PurchaseProAppErrorCode =
  | 'invalid_body'
  | 'apple_4_2_6_ack_required'
  | 'attestation_text_required'
  | 'pro_plan_required'
  | 'already_active'
  | 'stripe_customer_missing'
  | 'subscription_lookup_failed'
  | 'stripe_invoice_failed'
  | 'attestation_record_failed'
