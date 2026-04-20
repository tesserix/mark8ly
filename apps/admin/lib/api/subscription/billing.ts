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
 * A 404 on this endpoint is not an error: it means the store predates
 * the v2.3 signup pipeline and has no store_subscriptions row yet. The
 * billing page has its own bootstrap flow to remediate. Every OTHER
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
 * Idempotently initialise the store_subscriptions row for a store that
 * predates the v2.3 signup pipeline. The admin billing page calls this
 * when getSubscription() 404s so the merchant can proceed without
 * platform intervention.
 */
export async function bootstrapSubscription(
  storeId: string,
): Promise<CurrentPlan> {
  const raw = await apiClient.post<unknown>(
    `/api/admin/stores/${storeId}/subscription/bootstrap`,
    null,
  )
  const parsed = subscriptionResponseSchema.parse(raw)
  return toCurrentPlan(parsed)
}

/**
 * Fetch up to 25 most-recent invoices for the store's Stripe customer.
 * Pre-bootstrap stores return an empty list (no Stripe customer yet).
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
