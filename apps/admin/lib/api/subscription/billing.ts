/**
 * Billing API client.
 *
 * Wraps the two admin subscription endpoints:
 *   GET  /api/v1/admin/stores/:storeId/subscription  → current plan
 *   POST /api/v1/admin/stores/:storeId/subscription/portal → Stripe portal URL
 */
import { ApiError, apiClient } from '@/lib/api/client'
import {
  subscriptionResponseSchema,
  portalResponseSchema,
  listInvoicesResponseSchema,
  toCurrentPlan,
  type CurrentPlan,
  type Invoice,
  type PortalResponse,
} from './schemas/billing'

/**
 * Fetch the current subscription for a store.
 *
 * The response is validated with Zod. Throws on network or API errors —
 * callers should handle via React Query's error state.
 *
 * A 404 on this endpoint means the store has no store_subscriptions row.
 * Since #827 onboarding creates that row at signup, so a 404 is a data
 * problem rather than a state the merchant can resolve — the billing page
 * says so, and no longer offers a button to create one. Every OTHER
 * admin page — Shipping, Payments, Products, etc. — mounts the shell
 * BannerStack which calls this via useCurrentPlan, so we must NOT
 * pollute the console with a 404 on every navigation. Returning null
 * lets banner hooks render nothing (they already treat undefined/null
 * data as "no active banner") and silences the devtools noise.
 */
export async function getSubscription(
  storeId: string,
): Promise<CurrentPlan | null> {
  try {
    const raw = await apiClient.get<unknown>(
      `/api/admin/stores/${storeId}/subscription`,
    )
    const parsed = subscriptionResponseSchema.parse(raw)
    return toCurrentPlan(parsed)
  } catch (err) {
    if (err instanceof ApiError && err.status === 404) {
      return null
    }
    throw err
  }
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
/**
 * Fetch up to 25 most-recent invoices for the store's Stripe customer.
 * A store with no Stripe customer yet returns an empty list — since #827 a
 * subscription row deliberately starts without one.
 */
export async function listInvoices(storeId: string): Promise<Invoice[]> {
  const raw = await apiClient.get<unknown>(
    `/api/admin/stores/${storeId}/subscription/invoices`,
  )
  const parsed = listInvoicesResponseSchema.parse(raw)
  return parsed.data
}

export async function openPortal(storeId: string): Promise<PortalResponse> {
  const returnUrl =
    typeof window !== 'undefined'
      ? `${window.location.origin}/settings/billing`
      : '/settings/billing'

  const raw = await apiClient.post<unknown>(
    `/api/admin/stores/${storeId}/subscription/portal`,
    { return_url: returnUrl },
  )
  return portalResponseSchema.parse(raw)
}
