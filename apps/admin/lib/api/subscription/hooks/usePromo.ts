/**
 * React Query hook for promo-code redemption.
 *
 * Invalidates ['subscription', storeId] on success so the plan card reflects
 * the new price without a reload.
 */
'use client'

import { useMutation, useQueryClient } from '@tanstack/react-query'
import type { UseMutationResult } from '@tanstack/react-query'
import { applyPromo } from '../promo'
import type { ApplyPromoResponse } from '../schemas/promo'

/**
 * Redeems a promo code against the store's subscription.
 *
 * The error is deliberately left as `Error` rather than narrowed here: the
 * mutation rejects with PromoRejectedError for a refused code and ApiError
 * for everything else, and the component has to tell those apart to pick the
 * right sentence. Narrowing to one of them here would hide the other.
 *
 * @param storeId - The store UUID. Required.
 */
export function useApplyPromo(
  storeId: string,
): UseMutationResult<ApplyPromoResponse, Error, string> {
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: (code: string) => applyPromo(storeId, code),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['subscription', storeId] })
    },
  })
}
