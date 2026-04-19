/**
 * Billing API client.
 *
 * Wraps the two admin subscription endpoints:
 *   GET  /api/v1/admin/stores/:storeId/subscription  → current plan
 *   POST /api/v1/admin/stores/:storeId/subscription/portal → Stripe portal URL
 */
import { apiClient } from '@/lib/api/client'
import {
  subscriptionResponseSchema,
  portalResponseSchema,
  toCurrentPlan,
  type CurrentPlan,
  type PortalResponse,
} from './schemas/billing'

/**
 * Fetch the current subscription for a store.
 *
 * The response is validated with Zod. Throws on network or API errors —
 * callers should handle via React Query's error state.
 */
export async function getSubscription(storeId: string): Promise<CurrentPlan> {
  const raw = await apiClient.get<unknown>(
    `/api/v1/admin/stores/${storeId}/subscription`,
  )
  const parsed = subscriptionResponseSchema.parse(raw)
  return toCurrentPlan(parsed)
}

/**
 * Open a Stripe Customer Portal session for the given store.
 *
 * Returns the portal URL. The caller is responsible for redirecting
 * (`window.location.href = data.url`). Stripe requires a full-page redirect,
 * not a new tab.
 *
 * Note: the Go handler returns `{ url: string }` (not `portal_url`).
 */
export async function openPortal(storeId: string): Promise<PortalResponse> {
  const returnUrl =
    typeof window !== 'undefined'
      ? `${window.location.origin}/settings/billing`
      : '/settings/billing'

  const raw = await apiClient.post<unknown>(
    `/api/v1/admin/stores/${storeId}/subscription/portal`,
    { return_url: returnUrl },
  )
  return portalResponseSchema.parse(raw)
}
