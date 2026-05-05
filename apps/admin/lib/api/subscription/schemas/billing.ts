/**
 * Zod schemas for the billing/subscription wire types.
 *
 * Source of truth: services/marketplace-api/internal/handlers/admin/subscription.go
 *
 * Currently populated by the Go handler:
 *   id, store_id, plan, status, current_period_start, current_period_end,
 *   cancel_at_period_end, stripe_subscription_id, created_at,
 *   arbitrage_flag, latest_arbitrage_audit,
 *   payment_method_type/brand/last4,
 *   has_default_payment_method, trial_ends_at, days_remaining_in_trial,
 *   trial_cta, feature_limits, min_plan_for_feature.
 *
 * Fields like billing_currency, add_ons, next_invoice_amount_minor are NOT
 * yet in the Go DTO. Defined with `.optional()` so the schema can be
 * tightened when the backend ships them without a breaking change here.
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

export const appealStatusSchema = z.enum([
  'pending',
  'under_review',
  'resolved',
  'rejected',
])

export type AppealStatus = z.infer<typeof appealStatusSchema>

export const arbitrageAuditSummarySchema = z.object({
  card_country: z.string(),
  billing_country: z.string(),
  ip_country: z.string(),
  resolution: z.string(),
  flagged_at: z.string(),
  mismatch_reason: z.string(),
  // ─── Appeal fields (added by P8 backend; optional until P8 ships) ──────
  /** Status of an in-progress or completed appeal. */
  appeal_status: appealStatusSchema.nullable().optional(),
  /** ISO date string when the appeal was submitted. */
  appeal_submitted_at: z.string().nullable().optional(),
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

  /** Payment method kind: "card" | "link" | null. Renders conditionally. */
  payment_method_type: z.string().nullable().optional(),

  /** Stripe card brand (visa, mastercard, ...) or "link" for Stripe Link. */
  payment_method_brand: z.string().nullable().optional(),

  /** Card last 4 digits, or Link account email for Type=link. */
  payment_method_last4: z.string().nullable().optional(),

  // ─── Trial banner state (migration 087) ────────────────────────────────
  // These four fields drive the in-admin trial countdown banner. They are
  // populated only while status is 'signup' or 'trialing'; on active /
  // expired / past_due the trial_* fields are absent.

  /**
   * Whether the Stripe customer has a default payment method on file.
   * Mirrored from invoice_settings.default_payment_method by the
   * customer.updated webhook handler. Drives the trial reminder cron's
   * cadence and the banner's CTA.
   */
  has_default_payment_method: z.boolean().default(false),

  /** Days remaining in trial (0 ≤ n ≤ 90). Omitted when not in trial. */
  days_remaining_in_trial: z.number().int().nullable().optional(),

  /**
   * UI banner CTA. The frontend uses this to choose copy + button label
   * without re-deriving the same logic from days/PM/plan triplets.
   * Omitted (null) when banner should be hidden.
   */
  trial_cta: z
    .enum(['pick_plan', 'add_card', 'billing_imminent', 'all_set'])
    .nullable()
    .optional(),

  // ─── Plan feature gating (plangate.AllFeatureLimits) ───────────────────
  // The admin UI uses these maps to disable gated controls and render
  // accurate "upgrade to {plan}" tooltips, so we never drift from the
  // server-side gate (plangate.RequireFeature).

  /**
   * Per-feature limits for the current plan. Sentinel values:
   *   -1 = Unlimited
   *   -2 = Negotiated (Pro — contact sales)
   *    0 = Disabled (gated off)
   *   otherwise: numeric cap (e.g. 5 stores, 25 images).
   */
  feature_limits: z.record(z.string(), z.number().int()).default({}),

  /**
   * For each feature key, the lowest plan that enables it. Frontend
   * uses this to render "Upgrade to Studio" / "Upgrade to Pro" CTAs.
   */
  min_plan_for_feature: z.record(z.string(), z.string()).default({}),

  /**
   * White-label app lifecycle state. Populated by P13/P14 backend once the
   * add-on is active. Optional until those plans ship.
   */
  white_label_app: z
    .object({
      lifecycle_state: z
        .enum([
          'pending_credentials',
          'building',
          'submitted_apple',
          'submitted_google',
          'live_apple',
          'live_google',
          'live_both',
          'update_needed',
          'paused',
        ])
        .nullable()
        .optional(),
      updated_at: z.string().nullable().optional(),
    })
    .nullable()
    .optional(),
})

export type SubscriptionResponse = z.infer<typeof subscriptionResponseSchema>

export type WhiteLabelAppLifecycleState =
  | 'pending_credentials'
  | 'building'
  | 'submitted_apple'
  | 'submitted_google'
  | 'live_apple'
  | 'live_google'
  | 'live_both'
  | 'update_needed'
  | 'paused'

/**
 * CurrentPlan — the domain type the UI actually works with.
 * Normalises nullable period fields to a consistent shape.
 */
export type TrialCTA = 'pick_plan' | 'add_card' | 'billing_imminent' | 'all_set'

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
  /** ISO date string: when the subscription record was created (signup timestamp). */
  createdAt: string
  trialEndsAt: string | null
  /** Days remaining in trial; null when not in trial. */
  daysRemainingInTrial: number | null
  /** UI banner state — null hides the banner. */
  trialCTA: TrialCTA | null
  /** Whether a default payment method is on file in Stripe. */
  hasDefaultPaymentMethod: boolean
  nextInvoiceAmountMinor: number | null
  paymentMethodType: 'card' | 'link' | null
  paymentMethodBrand: string | null
  paymentMethodLast4: string | null
  /**
   * Per-feature limits for the current plan. Sentinel values:
   *   -1 = Unlimited, -2 = Negotiated (contact sales), 0 = Disabled.
   * Use the helper `featureLimit` below to read defensively.
   */
  featureLimits: Record<string, number>
  /** For each feature, the lowest plan that enables it (e.g. "studio"). */
  minPlanForFeature: Record<string, string>
  /** White-label app lifecycle info. Null until P13/P14 backend ships. */
  whiteLabelApp: {
    lifecycleState: WhiteLabelAppLifecycleState | null
    updatedAt: string | null
  } | null
}

// featureLimit / featureEnabled / featureUnlimited — typed helpers around
// the sentinel ints so callers don't have to remember which negative value
// means what. Mirrors plangate.go's Disabled/Unlimited/Negotiated constants.
export const FEATURE_DISABLED = 0
export const FEATURE_UNLIMITED = -1
export const FEATURE_NEGOTIATED = -2

export function featureLimit(plan: CurrentPlan, feature: string): number {
  return plan.featureLimits[feature] ?? FEATURE_DISABLED
}

export function featureEnabled(plan: CurrentPlan, feature: string): boolean {
  return featureLimit(plan, feature) !== FEATURE_DISABLED
}

export function featureUnlimited(plan: CurrentPlan, feature: string): boolean {
  const v = featureLimit(plan, feature)
  return v === FEATURE_UNLIMITED || v === FEATURE_NEGOTIATED
}

/** Returns the lowest plan that enables the feature (e.g. "studio") for upgrade CTAs. */
export function minPlanForFeature(plan: CurrentPlan, feature: string): string {
  return plan.minPlanForFeature[feature] ?? 'pro'
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
    createdAt: raw.created_at,
    trialEndsAt: raw.trial_ends_at ?? null,
    daysRemainingInTrial: raw.days_remaining_in_trial ?? null,
    trialCTA: raw.trial_cta ?? null,
    hasDefaultPaymentMethod: raw.has_default_payment_method,
    nextInvoiceAmountMinor: raw.next_invoice_amount_minor ?? null,
    paymentMethodType:
      raw.payment_method_type === 'card' || raw.payment_method_type === 'link'
        ? raw.payment_method_type
        : null,
    paymentMethodBrand: raw.payment_method_brand ?? null,
    paymentMethodLast4: raw.payment_method_last4 ?? null,
    featureLimits: raw.feature_limits ?? {},
    minPlanForFeature: raw.min_plan_for_feature ?? {},
    whiteLabelApp: raw.white_label_app
      ? {
          lifecycleState:
            (raw.white_label_app.lifecycle_state as WhiteLabelAppLifecycleState | null | undefined) ??
            null,
          updatedAt: raw.white_label_app.updated_at ?? null,
        }
      : null,
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

// ---------------------------------------------------------------------------
// GET /api/admin/stores/:storeId/subscription/invoices
// Returns { data: InvoiceDTO[] } — up to 25 most-recent Stripe invoices.
// ---------------------------------------------------------------------------

export const invoiceSchema = z.object({
  id: z.string(),
  number: z.string().default(''),
  /** ISO-8601 timestamp from Stripe's `created` unix epoch. */
  created_at: z.string(),
  /** Minor units (cents/paise/sen/etc.). Zero-decimal currencies already in base units per Stripe. */
  amount_paid: z.number().int(),
  amount_due: z.number().int(),
  /** Lowercase ISO 4217 — e.g. 'usd', 'inr'. */
  currency: z.string(),
  /** Stripe status: paid | open | void | draft | uncollectible. Unknown values passthrough. */
  status: z.string(),
  hosted_invoice_url: z.string().default(''),
  invoice_pdf: z.string().default(''),
  period_start: z.string(),
  period_end: z.string(),
})

export type Invoice = z.infer<typeof invoiceSchema>

export const listInvoicesResponseSchema = z.object({
  data: z.array(invoiceSchema),
})

export type ListInvoicesResponse = z.infer<typeof listInvoicesResponseSchema>
