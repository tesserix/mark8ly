/**
 * Maps Cloudflare `CF-IPCountry` (ISO-3166 alpha-2) codes to ISO-4217
 * billing currencies supported by Mark8ly (spec §3.2, 18 currencies).
 *
 * All 27 EU member states map to EUR. Countries not in the map default
 * to USD so the pricing page always has a valid currency to display.
 *
 * Shared between admin and onboarding apps — both apps geo-localize the
 * same way, so this is the single source of truth.
 */

import { SHARED_PRICING_CATALOGUE } from './pricing-data'

export const SUPPORTED_CURRENCIES = [
  'USD',
  'CAD',
  'GBP',
  'AUD',
  'NZD',
  'EUR',
  'INR',
  'SGD',
  'HKD',
  'MYR',
  'IDR',
  'PHP',
  'THB',
  'VND',
  'JPY',
  'KRW',
  'AED',
  'SAR',
  // Additional PPP emerging-market currencies
  'BRL',
  'MXN',
  'ZAR',
  'NGN',
  'KES',
] as const

export type Currency = (typeof SUPPORTED_CURRENCIES)[number]

/**
 * All 27 EU member states that use the Euro as their currency.
 * Source: https://ec.europa.eu/info/business-economy-euro/euro-area/euro-area-countries_en
 */
const EU_MEMBER_STATES: readonly string[] = [
  'AT',
  'BE',
  'CY',
  'DE',
  'EE',
  'ES',
  'FI',
  'FR',
  'GR',
  'HR',
  'IE',
  'IT',
  'LT',
  'LU',
  'LV',
  'MT',
  'NL',
  'PT',
  'SI',
  'SK',
]

export const COUNTRY_TO_CURRENCY: Record<string, Currency> = {
  // Developed markets — USD parity
  US: 'USD',
  CA: 'CAD',
  GB: 'GBP',
  AU: 'AUD',
  NZ: 'NZD',
  SG: 'SGD',
  HK: 'HKD',
  JP: 'JPY',
  KR: 'KRW',

  // Middle East
  AE: 'AED',
  SA: 'SAR',

  // South & South-East Asia — PPP-adjusted
  IN: 'INR',
  MY: 'MYR',
  ID: 'IDR',
  PH: 'PHP',
  TH: 'THB',
  VN: 'VND',

  // Emerging markets
  BR: 'BRL',
  MX: 'MXN',
  ZA: 'ZAR',
  NG: 'NGN',
  KE: 'KES',

  // EU member states all map to EUR
  ...Object.fromEntries(EU_MEMBER_STATES.map((cc) => [cc, 'EUR' as Currency])),
}

/**
 * Maps a `CF-IPCountry` header value to a supported billing currency.
 * Accepts null / undefined / lowercase safely and falls back to USD.
 */
export function countryToCurrency(cc: string | null | undefined): Currency {
  if (!cc) return 'USD'
  const upper = cc.toUpperCase()
  return COUNTRY_TO_CURRENCY[upper] ?? 'USD'
}

/**
 * The subset of `SUPPORTED_CURRENCIES` that can actually be priced: a
 * currency counts as priceable only when EVERY plan in
 * `SHARED_PRICING_CATALOGUE.plans` carries a row for it — a currency present
 * on one plan but missing on another would still mislabel that plan's price.
 *
 * Deliberately scoped to `plans` only, not `proApp`: the add-on is always
 * billed and displayed in USD globally (spec §4.1.2) regardless of this
 * set, so its currency coverage is irrelevant to what counts as
 * "priceable" here — including it would only ever narrow this set for a
 * reason that has nothing to do with plan pricing.
 *
 * Derived from the pricing table itself rather than hand-maintained, so it
 * cannot drift from the prices the way a fourth hand-written list could.
 * `SUPPORTED_CURRENCIES` stays broader on purpose: it drives
 * `COUNTRY_TO_CURRENCY` geo-targeting, which is correct to know that (say)
 * Thailand uses THB even though Mark8ly cannot price in it yet.
 */
export const PRICEABLE_CURRENCIES: ReadonlySet<Currency> = (() => {
  const planCount = SHARED_PRICING_CATALOGUE.plans.length
  const rowCounts = new Map<Currency, number>()
  for (const plan of SHARED_PRICING_CATALOGUE.plans) {
    for (const currency of Object.keys(plan.prices) as Currency[]) {
      rowCounts.set(currency, (rowCounts.get(currency) ?? 0) + 1)
    }
  }
  const priceable = new Set<Currency>()
  for (const [currency, count] of rowCounts) {
    if (count === planCount) priceable.add(currency)
  }
  return priceable
})()

/**
 * Returns the supported currency for the given code, or USD if the input
 * isn't one the pricing table can actually price. Use this to validate a
 * cookie value before rendering — geo-targeting (`COUNTRY_TO_CURRENCY`)
 * may resolve a visitor to a currency with no price row, and displaying a
 * USD amount under that currency's label would misquote the price. `USD`
 * is always priceable, so the fallback is safe.
 */
export function normalizeCurrency(value: string | null | undefined): Currency {
  if (!value) return 'USD'
  const upper = value.toUpperCase() as Currency
  return PRICEABLE_CURRENCIES.has(upper) ? upper : 'USD'
}

/**
 * Canonical cookie name both admin and onboarding write + read.
 * Change here and both apps follow.
 */
export const CURRENCY_COOKIE_NAME = 'mk8_currency'
