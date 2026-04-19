/**
 * React Query hooks for the Pro+App add-on purchase domain.
 *
 * Hooks:
 *   usePurchaseProApp(storeId) — mutation: purchases the add-on, invalidates
 *     ['subscription', storeId] on success.
 *
 * Note: There is no server-side "quote" endpoint. Proration context is
 * derived from the CurrentPlan already in the React Query cache via
 * useCurrentPlan(storeId). No useProAppQuote hook is needed.
 */
'use client'

import { useMutation, useQueryClient } from '@tanstack/react-query'
import type { UseMutationResult } from '@tanstack/react-query'
import { purchaseProApp } from '../proApp'
import type {
  PurchaseProAppRequest,
  PurchaseProAppResponse,
} from '../schemas/proApp'

/**
 * Mutation hook for purchasing the White-label App add-on.
 *
 * On success invalidates the subscription query so the plan card and
 * any add-on gates re-read the updated state.
 *
 * @param storeId - The store UUID.
 */
export function usePurchaseProApp(
  storeId: string,
): UseMutationResult<PurchaseProAppResponse, Error, PurchaseProAppRequest> {
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: (body: PurchaseProAppRequest) => purchaseProApp(storeId, body),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['subscription', storeId] })
    },
  })
}
