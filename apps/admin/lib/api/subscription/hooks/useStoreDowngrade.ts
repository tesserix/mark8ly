/**
 * React Query hooks for the store-downgrade block flow.
 *
 * Convention:
 *   - useDowngradeBlockList: query, enabled when storeId + all params present.
 *     staleTime 30_000 ms (billing data cadence).
 *   - useCloseStore / useDeleteStore: mutations, both invalidate ['stores']
 *     and ['subscription', storeId] on success so callers always see fresh data.
 */
'use client'

import {
  useQuery,
  useMutation,
  useQueryClient,
  type UseQueryResult,
  type UseMutationResult,
} from '@tanstack/react-query'
import {
  getDowngradeBlockList,
  closeStore,
  deleteStore,
  type GetDowngradeBlockListParams,
} from '../stores'
import type { DowngradeBlockList, StoreActionResponse } from '../schemas/stores'

const BLOCK_LIST_STALE_TIME = 30_000

/**
 * Fetch the list of over-cap stores blocking a downgrade.
 *
 * @param storeId - The primary store UUID for the subscription context.
 * @param params  - Target plan, billing period, and how many over cap.
 */
export function useDowngradeBlockList(
  storeId: string,
  params: GetDowngradeBlockListParams,
): UseQueryResult<DowngradeBlockList, Error> {
  const { targetPlan, billingPeriod, overBy } = params

  return useQuery({
    queryKey: ['stores', 'downgrade-block', storeId, targetPlan, billingPeriod, overBy],
    queryFn: () => getDowngradeBlockList(storeId, params),
    staleTime: BLOCK_LIST_STALE_TIME,
    enabled: Boolean(storeId && targetPlan && billingPeriod && overBy > 0),
  })
}

/**
 * Mutation to close a store.
 *
 * Invalidates ['stores'] and ['subscription', storeId] on success so
 * the stores list and billing page both refresh.
 */
export function useCloseStore(
  storeId: string,
): UseMutationResult<StoreActionResponse, Error, void> {
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: () => closeStore(storeId),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['stores'] })
      void queryClient.invalidateQueries({ queryKey: ['subscription'] })
    },
  })
}

/**
 * Mutation to delete a store (soft-delete, 60-day grace period).
 *
 * Invalidates ['stores'] and ['subscription'] on success.
 */
export function useDeleteStore(
  storeId: string,
): UseMutationResult<StoreActionResponse, Error, void> {
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: () => deleteStore(storeId),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['stores'] })
      void queryClient.invalidateQueries({ queryKey: ['subscription'] })
    },
  })
}
