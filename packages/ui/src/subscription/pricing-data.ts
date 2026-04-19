/**
 * Per-currency headline prices for Mark8ly's three public plans
 * (Starter / Studio / Pro) plus the Pro+App add-on. All amounts are
 * in minor units (cents / paise / subunits).
 *
 * The Money primitive consumes this data and formats via Intl.
 *
 * This file is the SINGLE SOURCE OF TRUTH for headline pricing across
 * admin `/pricing` AND onboarding `/#pricing`. To change a price,
 * edit it here and both apps update.
 *
 * USD is guaranteed to exist for every plan + add-on. All other
 * currencies are optional fallbacks — any missing entry falls back
 * to USD at render time.
 */

import type { Currency } from './country-map'

export type PlanId = 'starter' | 'studio' | 'pro'

export interface PlanPrice {
  /** Monthly billing: charged each month in minor units. */
  monthly: number
  /** Annual billing: charged once a year in minor units. */
  annual: number
  /** Annual cost expressed as a per-month equivalent for display. */
  annualMonthlyEquivalent: number
}

export interface SharedPlan {
  id: PlanId
  prices: Partial<Record<Currency, PlanPrice>>
}

export interface SharedAddOn {
  prices: Partial<Record<Currency, PlanPrice>>
}

export interface SharedPricingCatalogue {
  plans: SharedPlan[]
  proApp: SharedAddOn
}

/**
 * Ten currencies cover >95% of Mark8ly's target markets. Adding a
 * new currency means appending its prices here + extending
 * `SUPPORTED_CURRENCIES` in country-map.ts. Currencies present in
 * country-map but missing here fall back to USD on render.
 */
export const SHARED_PRICING_CATALOGUE: SharedPricingCatalogue = {
  plans: [
    {
      id: 'starter',
      prices: {
        USD: { monthly: 2900, annual: 27600, annualMonthlyEquivalent: 2300 },
        GBP: { monthly: 2300, annual: 21600, annualMonthlyEquivalent: 1800 },
        EUR: { monthly: 2700, annual: 25200, annualMonthlyEquivalent: 2100 },
        CAD: { monthly: 3900, annual: 37200, annualMonthlyEquivalent: 3100 },
        AUD: { monthly: 4500, annual: 43200, annualMonthlyEquivalent: 3600 },
        NZD: { monthly: 4800, annual: 45600, annualMonthlyEquivalent: 3800 },
        INR: { monthly: 249900, annual: 2399900, annualMonthlyEquivalent: 199900 },
        SGD: { monthly: 3900, annual: 37200, annualMonthlyEquivalent: 3100 },
        AED: { monthly: 10700, annual: 102000, annualMonthlyEquivalent: 8500 },
        JPY: { monthly: 440000, annual: 4200000, annualMonthlyEquivalent: 350000 },
      },
    },
    {
      id: 'studio',
      prices: {
        USD: { monthly: 7900, annual: 75600, annualMonthlyEquivalent: 6300 },
        GBP: { monthly: 6300, annual: 59400, annualMonthlyEquivalent: 4950 },
        EUR: { monthly: 7300, annual: 69600, annualMonthlyEquivalent: 5800 },
        CAD: { monthly: 10700, annual: 100800, annualMonthlyEquivalent: 8400 },
        AUD: { monthly: 12200, annual: 115200, annualMonthlyEquivalent: 9600 },
        NZD: { monthly: 13100, annual: 124800, annualMonthlyEquivalent: 10400 },
        INR: { monthly: 649900, annual: 6239900, annualMonthlyEquivalent: 519900 },
        SGD: { monthly: 10700, annual: 100800, annualMonthlyEquivalent: 8400 },
        AED: { monthly: 29000, annual: 276000, annualMonthlyEquivalent: 23000 },
        JPY: { monthly: 1190000, annual: 11400000, annualMonthlyEquivalent: 950000 },
      },
    },
    {
      id: 'pro',
      prices: {
        // Pro: annual = $99/mo-equivalent; monthly = $119/mo (+20% premium).
        USD: { monthly: 11900, annual: 118800, annualMonthlyEquivalent: 9900 },
        GBP: { monthly: 9500, annual: 94800, annualMonthlyEquivalent: 7900 },
        EUR: { monthly: 10900, annual: 108000, annualMonthlyEquivalent: 9000 },
        CAD: { monthly: 16200, annual: 160800, annualMonthlyEquivalent: 13400 },
        AUD: { monthly: 18500, annual: 183600, annualMonthlyEquivalent: 15300 },
        NZD: { monthly: 19900, annual: 198000, annualMonthlyEquivalent: 16500 },
        INR: { monthly: 989900, annual: 9839900, annualMonthlyEquivalent: 819900 },
        SGD: { monthly: 16200, annual: 160800, annualMonthlyEquivalent: 13400 },
        AED: { monthly: 43700, annual: 435000, annualMonthlyEquivalent: 36250 },
        JPY: { monthly: 1790000, annual: 17900000, annualMonthlyEquivalent: 1491666 },
      },
    },
  ],
  proApp: {
    prices: {
      USD: { monthly: 4900, annual: 47400, annualMonthlyEquivalent: 3950 },
      GBP: { monthly: 3900, annual: 37200, annualMonthlyEquivalent: 3100 },
      EUR: { monthly: 4500, annual: 43200, annualMonthlyEquivalent: 3600 },
      CAD: { monthly: 6700, annual: 64800, annualMonthlyEquivalent: 5400 },
      AUD: { monthly: 7600, annual: 73200, annualMonthlyEquivalent: 6100 },
      NZD: { monthly: 8200, annual: 78000, annualMonthlyEquivalent: 6500 },
      INR: { monthly: 399900, annual: 3839900, annualMonthlyEquivalent: 319900 },
      SGD: { monthly: 6700, annual: 64800, annualMonthlyEquivalent: 5400 },
      AED: { monthly: 18000, annual: 172500, annualMonthlyEquivalent: 14375 },
      JPY: { monthly: 740000, annual: 7080000, annualMonthlyEquivalent: 590000 },
    },
  },
}

/**
 * Returns the price for `plan` in `currency`, or the USD fallback
 * when the currency isn't covered in the catalogue. USD is always
 * present — the fallback is safe.
 */
export function getPlanPrice(plan: SharedPlan, currency: Currency): PlanPrice {
  return plan.prices[currency] ?? plan.prices.USD!
}

/**
 * Returns the Pro+App add-on price in `currency`, or the USD fallback.
 */
export function getAddOnPrice(addOn: SharedAddOn, currency: Currency): PlanPrice {
  return addOn.prices[currency] ?? addOn.prices.USD!
}
