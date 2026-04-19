/**
 * React Query mutation hook for the Pro contact-sales form.
 *
 * On success:             result.data = { status: 'submitted', ticket_id, fallback: undefined }
 * On 404 (stub not live): result.data = { status: 'submitted', ticket_id: '', fallback: 'mailto' }
 *
 * The component reads `result.data.fallback` to decide between:
 *   - Redirect to /admin/settings/billing + success toast
 *   - Open mailto + mailtoFallback toast
 *
 * Design decision: the fallback signal lives here, not in the component, so
 * the form has no try/catch and no knowledge of the ProContactEndpointMissingError
 * class. The hook surfaces a clean, typed result.
 */
'use client'

import { useMutation, useQueryClient, type UseMutationResult } from '@tanstack/react-query'
import { submitProContact, ProContactEndpointMissingError } from '../proContact'
import type { ProContactRequest } from '../schemas/proContact'

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

export type ProContactFallback = 'mailto' | undefined

export interface ProContactResult {
  status: 'submitted'
  ticket_id: string
  /** Set to 'mailto' when the backend endpoint is not yet live. */
  fallback: ProContactFallback
}

// ---------------------------------------------------------------------------
// Hook
// ---------------------------------------------------------------------------

/**
 * Submits a Pro contact-sales enquiry for the given store.
 *
 * @param storeId - The store UUID. The mutation is no-op if storeId is empty.
 */
export function useSubmitProContact(
  storeId: string,
): UseMutationResult<ProContactResult, Error, ProContactRequest> {
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: async (body: ProContactRequest): Promise<ProContactResult> => {
      try {
        const response = await submitProContact(storeId, body)
        return { ...response, fallback: undefined }
      } catch (err) {
        if (err instanceof ProContactEndpointMissingError) {
          // Backend not yet shipped — surface the mailto fallback signal.
          return { status: 'submitted', ticket_id: '', fallback: 'mailto' }
        }
        throw err
      }
    },
    onSuccess: () => {
      // Invalidate subscription prefix in case the backend eventually
      // creates a subscription record or modifies the store's plan state.
      void queryClient.invalidateQueries({ queryKey: ['subscription', storeId] })
    },
  })
}
