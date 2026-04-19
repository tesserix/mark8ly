/**
 * React Query hooks for dunning state — past_due and payment_action_required.
 *
 * Both hooks are DERIVED from useCurrentPlan so they re-use the cached
 * subscription query (queryKey: ['subscription', storeId]) rather than
 * issuing a second API call. This keeps network traffic minimal and
 * ensures both banners react to the same cache invalidation.
 *
 * ── Backend field gaps (TODO) ──────────────────────────────────────────
 * The GET /admin/stores/:storeId/subscription response (SubscriptionResponse
 * in subscription.go) does NOT include:
 *   - `retry_at`           → derived from periodEnd as best-effort proxy
 *   - `last_retry_failed_at` → not available; always null until backend ships
 *   - `first_flagged_at`   → not available; daysSinceFlagged always 0 until
 *                            backend ships this field
 *   - `hosted_invoice_url` → not on the wire DTO; use useCompleteActionUrl
 *                            to fetch it via the complete-action redirect
 *
 * Remove the TODO comments and update the hook types once P6 backend ships
 * these fields on the subscription response.
 * ───────────────────────────────────────────────────────────────────────
 */
'use client'

import { useMutation } from '@tanstack/react-query'
import type { UseMutationResult } from '@tanstack/react-query'
import { useCurrentPlan } from './useBilling'
import { getCompleteActionUrl } from '../dunning'
import type { CurrentPlan } from '../schemas/billing'

// ---------------------------------------------------------------------------
// Past-due state
// ---------------------------------------------------------------------------

export interface PastDueState {
  isPastDue: boolean
  /**
   * Best-effort retry date derived from current_period_end.
   * TODO: replace with `retry_at` once backend ships the field on the
   *       subscription wire DTO.
   */
  retryAt: Date | null
  /**
   * Date of the most recent failed retry attempt.
   * TODO: backend does not yet expose `last_retry_failed_at` on the
   *       subscription DTO. Always null until P6 ships the field.
   */
  lastRetryFailedAt: Date | null
  periodEnd: Date | null
}

function derivePastDueState(plan: CurrentPlan | undefined): PastDueState {
  if (!plan || plan.status !== 'past_due') {
    return {
      isPastDue: false,
      retryAt: null,
      lastRetryFailedAt: null,
      periodEnd: null,
    }
  }

  const periodEnd = plan.periodEnd ? new Date(plan.periodEnd) : null

  return {
    isPastDue: true,
    // TODO: replace with plan.retryAt once backend ships the field.
    retryAt: periodEnd,
    // TODO: replace with plan.lastRetryFailedAt once backend ships the field.
    lastRetryFailedAt: null,
    periodEnd,
  }
}

/**
 * Returns dunning state for past_due subscriptions.
 *
 * Derived from useCurrentPlan — shares its cache and staleTime (30 s).
 * Returns `isPastDue: false` when the subscription is in any other state.
 */
export function usePastDueState(storeId: string): PastDueState {
  const { data: plan } = useCurrentPlan(storeId)
  return derivePastDueState(plan)
}

// ---------------------------------------------------------------------------
// Payment-action-required state
// ---------------------------------------------------------------------------

export interface PaymentActionRequiredState {
  isPending: boolean
  /**
   * The Stripe hosted invoice URL.
   * TODO: backend does not yet expose `hosted_invoice_url` on the
   *       subscription DTO. Always null until P6 ships the field;
   *       use useCompleteActionUrl to fetch it via the redirect endpoint.
   */
  hostedInvoiceUrl: string | null
  /**
   * Date when the subscription first transitioned to payment_action_required.
   * TODO: backend does not yet expose `first_flagged_at` on the subscription
   *       DTO. Always null until P6 ships the field; daysSinceFlagged will
   *       always be 0 in the meantime, which renders the T-14 body copy.
   */
  firstFlaggedAt: Date | null
  /**
   * Days elapsed since the payment action was first flagged.
   * 0 when firstFlaggedAt is null (backend field not yet shipped).
   */
  daysSinceFlagged: number
}

function derivePaymentActionRequiredState(
  plan: CurrentPlan | undefined,
): PaymentActionRequiredState {
  if (!plan || plan.status !== 'payment_action_required') {
    return {
      isPending: false,
      hostedInvoiceUrl: null,
      firstFlaggedAt: null,
      daysSinceFlagged: 0,
    }
  }

  // TODO: replace with plan.firstFlaggedAt once backend ships the field on
  // the subscription wire DTO. Using `as` to prevent TypeScript from narrowing
  // the literal `null` to `never` in the ternary below — the branch becomes
  // reachable once the field is populated.
  const firstFlaggedAt = null as Date | null
  const daysSinceFlagged =
    firstFlaggedAt !== null
      ? Math.floor(
          (Date.now() - firstFlaggedAt.getTime()) / (1_000 * 60 * 60 * 24),
        )
      : 0

  return {
    isPending: true,
    // TODO: replace with plan.hostedInvoiceUrl once backend ships the field.
    hostedInvoiceUrl: null,
    firstFlaggedAt,
    daysSinceFlagged,
  }
}

/**
 * Returns dunning state for payment_action_required subscriptions (§4.7).
 *
 * IMPORTANT (spec §4.7): payment_action_required preserves FULL admin access.
 * The merchant's bank needs to confirm the payment (3DS/SCA/RBI e-mandate).
 * This is NOT a failed payment — it is a pending confirmation.
 *
 * Derived from useCurrentPlan — shares its cache and staleTime (30 s).
 * Returns `isPending: false` when the subscription is in any other state.
 */
export function usePaymentActionRequiredState(
  storeId: string,
): PaymentActionRequiredState {
  const { data: plan } = useCurrentPlan(storeId)
  return derivePaymentActionRequiredState(plan)
}

// ---------------------------------------------------------------------------
// Complete-action URL mutation
// ---------------------------------------------------------------------------

/**
 * Mutation that fetches the Stripe hosted-invoice URL via the complete-action
 * redirect endpoint and navigates to it.
 *
 * Usage:
 *   const { mutate: openInvoice, isPending } = useCompleteActionUrl(storeId)
 *   <button onClick={() => openInvoice()}>Review the invoice</button>
 *
 * On success: opens the hosted invoice URL in a new tab.
 * On error: the mutation's `error` is set; callers handle via `onError`.
 */
export function useCompleteActionUrl(
  storeId: string,
): UseMutationResult<{ url: string }, Error, void> {
  return useMutation({
    mutationFn: () => getCompleteActionUrl(storeId),
    onSuccess: (data) => {
      window.open(data.url, '_blank', 'noopener,noreferrer')
    },
  })
}
