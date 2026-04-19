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
 * Returns the supported currency for the given code, or USD if
 * the input isn't a known supported currency. Use this to validate
 * a cookie value before rendering.
 */
export function normalizeCurrency(value: string | null | undefined): Currency {
  if (!value) return 'USD'
  const upper = value.toUpperCase() as Currency
  return (SUPPORTED_CURRENCIES as readonly string[]).includes(upper)
    ? upper
    : 'USD'
}

/**
 * Canonical cookie name both admin and onboarding write + read.
 * Change here and both apps follow.
 */
export const CURRENCY_COOKIE_NAME = 'mk8_currency'
