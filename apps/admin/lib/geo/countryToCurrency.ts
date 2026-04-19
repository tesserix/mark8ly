/**
 * Re-export of the shared country→currency map at
 * `@repo/ui/subscription`. Kept as a local module so existing admin
 * imports (`@/lib/geo/countryToCurrency`) continue to resolve; remove
 * once all call sites use the `@repo/ui/subscription` barrel.
 */

export {
  SUPPORTED_CURRENCIES,
  COUNTRY_TO_CURRENCY,
  CURRENCY_COOKIE_NAME,
  countryToCurrency,
  normalizeCurrency,
  type Currency,
} from '@repo/ui/subscription'
