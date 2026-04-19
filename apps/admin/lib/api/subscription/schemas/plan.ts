/**
 * Zod schemas for the public pricing catalogue (spec §2 + §3.2).
 *
 * Plans: starter | studio | pro
 * All monetary amounts are in minor units (cents / paise) to match the
 * backend money representation.
 */
import { z } from 'zod'

// ---------------------------------------------------------------------------
// Currency
// ---------------------------------------------------------------------------

export const currencySchema = z.enum([
  'USD',
  'GBP',
  'EUR',
  'INR',
  'CAD',
  'AUD',
  'SGD',
  'NZD',
  'MYR',
  'THB',
  'PHP',
  'IDR',
  'VND',
])

export type Currency = z.infer<typeof currencySchema>

// ---------------------------------------------------------------------------
// Pricing periods
// ---------------------------------------------------------------------------

export const monthlyPricingSchema = z.object({
  /** Amount in minor units (e.g. 1900 = $19.00). */
  amount: z.number().int().nonnegative(),
  currency: currencySchema,
  per: z.literal('month'),
})

export type MonthlyPricing = z.infer<typeof monthlyPricingSchema>

export const annualPricingSchema = z.object({
  /** Total annual amount in minor units (e.g. 18200 = $182.00). */
  amount: z.number().int().nonnegative(),
  currency: currencySchema,
  per: z.literal('year'),
  /**
   * Monthly-equivalent in minor units (annual ÷ 12), used for
   * display copy: "equivalent to $X/mo".
   */
  monthlyEquivalent: z.number().int().nonnegative(),
})

export type AnnualPricing = z.infer<typeof annualPricingSchema>

// ---------------------------------------------------------------------------
// Features
// ---------------------------------------------------------------------------

export const featureSchema = z.object({
  label: z.string(),
  included: z.boolean(),
  /** Whether to visually highlight this row on the pricing grid. */
  highlight: z.boolean().optional(),
})

export type Feature = z.infer<typeof featureSchema>

// ---------------------------------------------------------------------------
// Add-ons
// ---------------------------------------------------------------------------

export const addOnIdSchema = z.literal('white_label_app')

export type AddOnId = z.infer<typeof addOnIdSchema>

export const addOnSchema = z.object({
  id: addOnIdSchema,
  name: z.string(),
  /** Monthly price in minor units (USD-denominated globally; no PPP). */
  priceMonthlyMinor: z.number().int().nonnegative(),
  currency: currencySchema,
})

export type AddOn = z.infer<typeof addOnSchema>

// ---------------------------------------------------------------------------
// Plan
// ---------------------------------------------------------------------------

export const planIdSchema = z.enum(['starter', 'studio', 'pro'])

export type PlanId = z.infer<typeof planIdSchema>

export const planSchema = z.object({
  id: planIdSchema,
  name: z.string(),
  tagline: z.string(),
  /** Whether this plan should be visually highlighted on the pricing grid. */
  featured: z.boolean().optional(),
  pricing: z.object({
    monthly: monthlyPricingSchema,
    annual: annualPricingSchema,
  }),
  addOns: z.array(addOnSchema).optional(),
  features: z.array(featureSchema),
})

export type Plan = z.infer<typeof planSchema>

// ---------------------------------------------------------------------------
// Pricing catalogue
// ---------------------------------------------------------------------------

export const pricingCatalogueSchema = z.object({
  currency: currencySchema,
  plans: z.array(planSchema),
})

export type PricingCatalogue = z.infer<typeof pricingCatalogueSchema>
