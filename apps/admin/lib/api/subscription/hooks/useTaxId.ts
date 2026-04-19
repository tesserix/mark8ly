/**
 * React Query hooks for the Tax-ID domain.
 *
 * useTaxIdStatus  — derives tax-ID state from the subscription cache.
 *                   No separate network request; reads embedded fields on
 *                   CurrentPlan (tax_id_status, tax_id_pause_reason, etc.)
 *                   which the subscription GET response carries once the
 *                   backend ships those fields (safe defaults applied in
 *                   schemas/billing.ts).
 *
 * useSubmitTaxId  — mutation. Invalidates ['subscription', storeId] on
 *                   success so the page re-renders with the updated status.
 */
'use client'

import {
  useMutation,
  useQueryClient,
  type UseMutationResult,
} from '@tanstack/react-query'
import { useCurrentPlan } from './useBilling'
import { submitTaxId } from '../taxId'
import type { TaxIdSubmitRequest, TaxIdSubmitResponse, TaxIdStatus, ClockPauseReason } from '../schemas/taxId'

// ---------------------------------------------------------------------------
// Derived tax-ID state
// ---------------------------------------------------------------------------

export interface TaxIdState {
  /** Current validation status, or 'not_submitted' when not yet filed. */
  status: TaxIdStatus
  /**
   * Reason the 90-day trial clock is paused, when status === 'clock_paused'.
   * null in all other states.
   */
  pauseReason: ClockPauseReason | null
  /** Validation error code when status === 'rejected'. */
  validationCode: string | null
  /** ISO date string of the last review, or null. */
  reviewedAt: string | null
  /** True while the subscription query is loading. */
  isLoading: boolean
}

/**
 * Derives the tax-ID state from the subscription cache entry.
 *
 * The subscription GET response embeds tax-ID fields. Until the backend
 * ships them, safe defaults (not_submitted, no pause) are returned.
 *
 * @param storeId - Pass '' to disable (mirrors useCurrentPlan convention).
 */
export function useTaxIdStatus(storeId: string): TaxIdState {
  const { data: plan, isLoading } = useCurrentPlan(storeId)

  if (!plan || isLoading) {
    return {
      status: 'not_submitted',
      pauseReason: null,
      validationCode: null,
      reviewedAt: null,
      isLoading,
    }
  }

  // Access extended fields via type assertion — they are defined in
  // embeddedTaxIdStateSchema but not yet on the CurrentPlan interface.
  // Cast via unknown to avoid implicit-any while keeping strict types.
  const extended = plan as unknown as {
    tax_id_status?: TaxIdStatus
    tax_id_pause_reason?: ClockPauseReason
    tax_id_validation_code?: string
    tax_id_reviewed_at?: string
  }

  return {
    status: extended.tax_id_status ?? 'not_submitted',
    pauseReason: extended.tax_id_pause_reason ?? null,
    validationCode: extended.tax_id_validation_code ?? null,
    reviewedAt: extended.tax_id_reviewed_at ?? null,
    isLoading: false,
  }
}

// ---------------------------------------------------------------------------
// Submit mutation
// ---------------------------------------------------------------------------

/**
 * Mutation to submit or resubmit a tax ID.
 *
 * On success: invalidates ['subscription', storeId] so the status badge
 * and form state update automatically.
 *
 * @param storeId - The store UUID.
 */
export function useSubmitTaxId(
  storeId: string,
): UseMutationResult<TaxIdSubmitResponse, Error, TaxIdSubmitRequest> {
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: (body: TaxIdSubmitRequest) => submitTaxId(storeId, body),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['subscription', storeId] })
    },
  })
}
