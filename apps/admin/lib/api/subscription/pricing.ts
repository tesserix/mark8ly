/**
 * Pricing API client — public endpoint (unauthenticated).
 *
 * Fetches the plan catalogue for a given billing currency from
 * GET /api/v1/public/pricing?currency={currency}.
 *
 * On 404 or network error the function transparently falls back to the
 * hand-rolled static catalogue in pricing-fallback.ts so the pricing page
 * remains functional before the backend endpoint ships.
 */
import { apiClient, ApiError } from '@/lib/api/client'
import { pricingCatalogueSchema } from './schemas/plan'
import type { PricingCatalogue, Currency } from './schemas/plan'
import { LOCAL_PRICING_FALLBACK } from './pricing-fallback'

const SUPPORTED_CURRENCIES = Object.keys(LOCAL_PRICING_FALLBACK) as Currency[]

/**
 * Normalise to an uppercase Currency key, falling back to 'USD' if the
 * input is not a supported currency (handles lowercase, unknown codes, etc.).
 */
function normaliseCurrency(raw: string): Currency {
  const upper = raw.toUpperCase() as Currency
  return SUPPORTED_CURRENCIES.includes(upper) ? upper : 'USD'
}

/**
 * Fetch the pricing catalogue for `currency`.
 *
 * - On success: validates the response with Zod and returns the catalogue.
 * - On 404 or network failure: returns the local static fallback.
 * - On Zod parse failure: throws — callers should treat this as a data error.
 */
export async function getPricing(currency: string): Promise<PricingCatalogue> {
  const normalisedCurrency = normaliseCurrency(currency)

  try {
    const raw = await apiClient.get<unknown>(
      `/api/v1/public/pricing?currency=${normalisedCurrency}`,
    )
    return pricingCatalogueSchema.parse(raw)
  } catch (err) {
    if (err instanceof ApiError && err.status === 404) {
      return LOCAL_PRICING_FALLBACK[normalisedCurrency]
    }

    // Network-level failure (status 0) also falls back.
    if (err instanceof ApiError && err.status === 0) {
      return LOCAL_PRICING_FALLBACK[normalisedCurrency]
    }

    // Any other ApiError (500, etc.) — surface to caller.
    throw err
  }
}
