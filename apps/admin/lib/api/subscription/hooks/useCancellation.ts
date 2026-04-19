/**
 * React Query hooks for the cancellation flow.
 *
 * useSubmitCancellation — schedules cancellation at period end.
 * useRevertCancellation — accepts the save offer, reverting cancel_scheduled → active.
 *
 * Both hooks invalidate ['subscription', storeId] on success so the plan
 * card and status badge reflect the new state immediately.
 */
'use client'

import { useMutation, useQueryClient } from '@tanstack/react-query'
import type { UseMutationResult } from '@tanstack/react-query'
import { submitCancellation, revertCancellation } from '../cancellation'
import type { CancelRequest, CancelResponse } from '../schemas/cancellation'

/**
 * Submits a cancellation request (accept_save_offer: false by default).
 *
 * @param storeId - The store UUID. Required.
 */
export function useSubmitCancellation(
  storeId: string,
): UseMutationResult<CancelResponse, Error, CancelRequest> {
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: (body: CancelRequest) => submitCancellation(storeId, body),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['subscription', storeId] })
    },
  })
}

/**
 * Accepts the save offer — reverses cancel_scheduled → active.
 *
 * No variables needed: the request body is fixed ({accept_save_offer: true}).
 */
export function useRevertCancellation(
  storeId: string,
): UseMutationResult<CancelResponse, Error, void> {
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: () => revertCancellation(storeId),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['subscription', storeId] })
    },
  })
}
