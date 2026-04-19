/**
 * Plan-change API client.
 *
 * Wraps the two admin plan-change endpoints:
 *   GET  /api/v1/admin/stores/:storeId/subscription/change-plan/preflight
 *   POST /api/v1/admin/stores/:storeId/subscription/change-plan
 *
 * NOTE: query params use `target_period` (not `billing_period`) per the Go handler.
 */
import { apiClient } from '@/lib/api/client'
import {
  preflightReportSchema,
  applyPlanChangeResponseSchema,
  type PreflightReport,
  type ApplyPlanChangeResponse,
  type ApplyPlanChangeBody,
} from './schemas/planChange'
import type { PlanId } from './schemas/plan'
import type { BillingPeriod } from './schemas/planChange'

/**
 * Fetch a read-only preflight report before committing a plan change.
 *
 * Pass `requestedCurrency` to validate the merchant's billing currency
 * against the locked subscription currency (optional — most callers omit it).
 */
export async function getProrationPreview(
  storeId: string,
  targetPlan: PlanId,
  targetPeriod: BillingPeriod,
  requestedCurrency?: string,
): Promise<PreflightReport> {
  const params = new URLSearchParams({
    target_plan: targetPlan,
    target_period: targetPeriod,
  })

  if (requestedCurrency) {
    params.set('requested_currency', requestedCurrency)
  }

  const raw = await apiClient.get<unknown>(
    `/api/v1/admin/stores/${storeId}/subscription/change-plan/preflight?${params.toString()}`,
  )

  return preflightReportSchema.parse(raw)
}

/**
 * Apply a plan change.
 *
 * For upgrades the change is immediate; for downgrades it is scheduled for
 * period end. The `result` field in the response discriminates between the
 * two cases.
 */
export async function applyPlanChange(
  storeId: string,
  body: ApplyPlanChangeBody,
): Promise<ApplyPlanChangeResponse> {
  const raw = await apiClient.post<unknown>(
    `/api/v1/admin/stores/${storeId}/subscription/change-plan`,
    body as Record<string, unknown>,
  )

  return applyPlanChangeResponseSchema.parse(raw)
}
