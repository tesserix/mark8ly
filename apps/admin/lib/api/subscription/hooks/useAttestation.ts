/**
 * React Query hooks for the US/CA business-entity attestation domain (§19.3.1).
 *
 * useAttestation     — derives attestation state from the subscription cache.
 * useSignAttestation — mutation; invalidates ['subscription', storeId] on success.
 */
'use client'

import {
  useMutation,
  useQueryClient,
  type UseMutationResult,
} from '@tanstack/react-query'
import { useCurrentPlan } from './useBilling'
import { signAttestation } from '../attestation'
import type { AttestationRequest, AttestationResponse } from '../schemas/attestation'

// ---------------------------------------------------------------------------
// Derived attestation state
// ---------------------------------------------------------------------------

export interface AttestationState {
  /** True when an attestation has been signed for this store. */
  isSigned: boolean
  /**
   * ISO date string from `accepted_at` carried on the subscription response.
   * null until the backend ships this field.
   */
  acceptedAt: string | null
  /** True while the subscription query is loading. */
  isLoading: boolean
}

/**
 * Derives attestation state from the subscription cache entry.
 *
 * The subscription GET response may carry `attestation_accepted_at`.
 * Until the backend ships this field, isSigned defaults to false.
 *
 * @param storeId - Pass '' to disable (mirrors useCurrentPlan convention).
 */
export function useAttestation(storeId: string): AttestationState {
  const { data: plan, isLoading } = useCurrentPlan(storeId)

  if (!plan || isLoading) {
    return { isSigned: false, acceptedAt: null, isLoading }
  }

  // Access extended fields via type assertion — not yet on CurrentPlan interface.
  const extended = plan as unknown as {
    attestation_accepted_at?: string | null
  }

  const acceptedAt = extended.attestation_accepted_at ?? null

  return {
    isSigned: Boolean(acceptedAt),
    acceptedAt,
    isLoading: false,
  }
}

// ---------------------------------------------------------------------------
// Sign attestation mutation
// ---------------------------------------------------------------------------

/**
 * Mutation to sign the US/CA business-entity attestation.
 *
 * On success: invalidates ['subscription', storeId] so the form switches
 * to the "Attestation on file" state.
 *
 * @param storeId - The store UUID.
 */
export function useSignAttestation(
  storeId: string,
): UseMutationResult<AttestationResponse, Error, AttestationRequest> {
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: (body: AttestationRequest) => signAttestation(storeId, body),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['subscription', storeId] })
    },
  })
}
