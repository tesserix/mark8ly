/**
 * React Query hooks for plan-change operations.
 *
 * Convention:
 *   - useProrationPreview: query, enabled only when storeId + targetPlan +
 *     targetPeriod are all provided AND target differs from current (callers
 *     pass undefined when it would be a no-op). staleTime 10_000 ms per spec.
 *   - useApplyPlanChange: mutation, invalidates ['subscription', storeId] on
 *     success.
 */
'use client'

import {
  useQuery,
  useMutation,
  useQueryClient,
  type UseQueryResult,
  type UseMutationResult,
} from '@tanstack/react-query'
import { getProrationPreview, applyPlanChange } from '../planChange'
import type {
  PreflightReport,
  ApplyPlanChangeResponse,
  ApplyPlanChangeBody,
  BillingPeriod,
} from '../schemas/planChange'
import type { PlanId } from '../schemas/plan'

const PREFLIGHT_STALE_TIME = 10_000

/**
 * Fetches the preflight proration report for a plan change.
 *
 * The query is disabled unless all three parameters are provided. Pass
 * `undefined` for `targetPlan` or `targetPeriod` when the selection hasn't
 * changed from the current plan (no-op case) — this prevents unnecessary
 * network requests.
 */
export function useProrationPreview(
  storeId: string,
  targetPlan: PlanId | undefined,
  targetPeriod: BillingPeriod | undefined,
): UseQueryResult<PreflightReport, Error> {
  return useQuery({
    queryKey: ['subscription', storeId, 'preflight', targetPlan, targetPeriod],
    queryFn: () =>
      getProrationPreview(storeId, targetPlan as PlanId, targetPeriod as BillingPeriod),
    staleTime: PREFLIGHT_STALE_TIME,
    enabled: Boolean(storeId && targetPlan && targetPeriod),
  })
}

/**
 * Mutation that applies a plan change.
 *
 * On success invalidates ['subscription', storeId] so the billing page
 * refetches the current plan. Callers handle the toast + redirect.
 */
export function useApplyPlanChange(
  storeId: string,
): UseMutationResult<ApplyPlanChangeResponse, Error, ApplyPlanChangeBody> {
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: (body: ApplyPlanChangeBody) => applyPlanChange(storeId, body),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['subscription', storeId] })
    },
  })
}
