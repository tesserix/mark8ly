/**
 * React Query hooks for the arbitrage appeal domain.
 *
 * useArbitrageFlag   — derives flag state from useCurrentPlan() (no extra fetch).
 * useSubmitArbitrageAppeal — mutation; invalidates ['subscription', storeId] on success.
 */
'use client'

import { useMutation, useQueryClient, type UseMutationResult } from '@tanstack/react-query'
import { useCurrentPlan } from './useBilling'
import { submitArbitrageAppeal } from '../arbitrage'
import type { ArbitrageAppealRequest, ArbitrageAppealResponse } from '../schemas/arbitrage'
import type { ArbitrageAuditSummary } from '../schemas/billing'

// ---------------------------------------------------------------------------
// Arbitrage flag derived state
// ---------------------------------------------------------------------------

export interface ArbitrageFlagState {
  /** True when the store currently has an open arbitrage flag. */
  flagged: boolean
  /** ISO date string when the flag was first raised, or null. */
  firstFlaggedAt: string | null
  /**
   * Severity heuristic:
   *   - null   — not flagged
   *   - minor  — mismatch_reason indicates a low-risk discrepancy
   *   - material — any other flagged state
   */
  severity: 'minor' | 'material' | null
  /** The raw latest audit summary from the subscription response, if present. */
  latestAudit: ArbitrageAuditSummary | null
  /** True while the subscription query is in flight. */
  isLoading: boolean
}

function deriveSeverity(
  flagged: boolean,
  audit: ArbitrageAuditSummary | null | undefined,
): 'minor' | 'material' | null {
  if (!flagged) return null
  const reason = audit?.mismatch_reason ?? ''
  // A card-country vs billing-country mismatch without an IP signal is minor.
  const isMinor =
    reason.toLowerCase().includes('card_country') &&
    !reason.toLowerCase().includes('ip_country')
  return isMinor ? 'minor' : 'material'
}

/**
 * Derives the arbitrage flag state from the subscription cache.
 * No extra network request — reads from the same useCurrentPlan cache entry.
 *
 * @param storeId - Pass '' to disable (mirrors useCurrentPlan convention).
 */
export function useArbitrageFlag(storeId: string): ArbitrageFlagState {
  const { data: plan, isLoading } = useCurrentPlan(storeId)

  if (!plan || isLoading) {
    return { flagged: false, firstFlaggedAt: null, severity: null, latestAudit: null, isLoading }
  }

  const flagged = plan.arbitrageFlag

  // latest_arbitrage_audit is not surfaced on CurrentPlan — it lives on
  // SubscriptionResponse. Fall back to null until CurrentPlan is expanded
  // to carry latestArbitrageAudit (TODO: schemas/billing.ts).
  // We avoid storing a null constant so TS control-flow does not narrow it
  // to `never` at the property-access site.
  const noAudit: ArbitrageAuditSummary | null = null

  return {
    flagged,
    firstFlaggedAt: null,
    severity: deriveSeverity(flagged, noAudit),
    latestAudit: noAudit,
    isLoading: false,
  }
}

// ---------------------------------------------------------------------------
// Submit appeal mutation
// ---------------------------------------------------------------------------

/**
 * Mutation to submit an arbitrage appeal.
 *
 * On success: invalidates ['subscription', storeId] so the banner
 * disappears and the status tracker updates.
 */
export function useSubmitArbitrageAppeal(
  storeId: string,
): UseMutationResult<ArbitrageAppealResponse, Error, ArbitrageAppealRequest> {
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: (body: ArbitrageAppealRequest) =>
      submitArbitrageAppeal(storeId, body),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['subscription', storeId] })
    },
  })
}
