/**
 * usePricing — React Query hook for the public pricing catalogue.
 *
 * staleTime is 1 hour: pricing data is very stable and changes at most
 * every 6 months per the pricing-review schedule (spec §4.2.3).
 */
import { useQuery } from '@tanstack/react-query'
import { getPricing } from '../pricing'
import type { PricingCatalogue } from '../schemas/plan'
import type { UseQueryResult } from '@tanstack/react-query'

const PRICING_STALE_TIME = 60 * 60 * 1000 // 1 hour

export function usePricing(currency: string): UseQueryResult<PricingCatalogue> {
  return useQuery({
    queryKey: ['pricing', currency],
    queryFn: () => getPricing(currency),
    staleTime: PRICING_STALE_TIME,
  })
}
