/**
 * React Query hooks for the billing/subscription domain.
 *
 * Convention (Pattern C):
 *   - queryKey always includes storeId to prevent cross-store cache leaks.
 *   - staleTime 30_000 ms (30 s) for billing data per plan spec.
 *   - Every mutation invalidates the ['subscription'] prefix.
 */
'use client'

import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import type { UseQueryResult, UseMutationResult } from '@tanstack/react-query'
import {
  bootstrapSubscription,
  getSubscription,
  listInvoices,
  openPortal,
} from '../billing'
import type {
  CurrentPlan,
  Invoice,
  PortalResponse,
} from '../schemas/billing'

const BILLING_STALE_TIME = 30_000

/**
 * Fetches the current subscription for a store.
 *
 * @param storeId - The store UUID. Pass an empty string to disable the query.
 */
export function useCurrentPlan(
  storeId: string,
): UseQueryResult<CurrentPlan, Error> {
  return useQuery({
    queryKey: ['subscription', storeId],
    queryFn: () => getSubscription(storeId),
    staleTime: BILLING_STALE_TIME,
    enabled: Boolean(storeId),
  })
}

/**
 * Initialises the subscription row for a store that predates the v2.3
 * signup pipeline. On success seeds the ['subscription', storeId] cache
 * with the new row so the UI immediately swaps from the "not found" CTA
 * to the normal billing view.
 */
export function useBootstrapSubscription(
  storeId: string,
): UseMutationResult<CurrentPlan, Error, void> {
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: () => bootstrapSubscription(storeId),
    onSuccess: (data) => {
      queryClient.setQueryData<CurrentPlan>(['subscription', storeId], data)
    },
  })
}

/**
 * Fetches up to 25 most-recent invoices for the store's Stripe customer.
 * Pre-bootstrap stores resolve to an empty array (no Stripe customer yet).
 */
export function useInvoices(storeId: string): UseQueryResult<Invoice[], Error> {
  return useQuery({
    queryKey: ['subscription', storeId, 'invoices'],
    queryFn: () => listInvoices(storeId),
    staleTime: BILLING_STALE_TIME,
    enabled: Boolean(storeId),
  })
}

/**
 * Opens a Stripe Customer Portal session and redirects the browser.
 *
 * On success the browser is redirected to the portal URL (full-page, per
 * Stripe best practice). On error the error is surfaced to the caller's
 * onError handler.
 */
export function useOpenPortal(
  storeId: string,
): UseMutationResult<PortalResponse, Error, void> {
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: () => openPortal(storeId),
    onSuccess: (data) => {
      // Invalidate subscription cache so the plan card refreshes on return.
      void queryClient.invalidateQueries({ queryKey: ['subscription'] })
      window.location.href = data.url
    },
  })
}
