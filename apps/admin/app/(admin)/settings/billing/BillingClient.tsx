'use client'

import { useEffect } from 'react'
import { Skeleton } from '@tesserix/web'
import { useToast } from '@/components/feedback/Toaster'
import { ApiError, SubscriptionInactiveError } from '@/lib/api/client'
import {
  useCurrentPlan,
} from '@/lib/api/subscription/hooks/useBilling'
import { subscriptionCopy } from '@/lib/copy/subscription'
import { PlanCard } from './PlanCard'
import { InvoicesList } from './InvoicesList'
import { PaymentMethodCard } from './PaymentMethodCard'
import { PromoCodeCard } from './PromoCodeCard'
import { WhiteLabelAppCard } from './WhiteLabelAppCard'

interface BillingClientProps {
  storeId: string
}

const copy = subscriptionCopy.billing

// ─── Skeleton ────────────────────────────────────────────────────────────────

function PanelSkeleton() {
  return (
    <div
      role="presentation"
      aria-label={copy.loadingAriaLabel}
      className="space-y-3 border-b border-[var(--hairline,var(--ink-100))] pb-10"
    >
      <Skeleton className="h-7 w-40 rounded-md" />
      <Skeleton className="h-4 w-72 rounded-md" />
      <Skeleton className="h-4 w-52 rounded-md" />
      <Skeleton className="h-10 w-32 rounded-md" />
    </div>
  )
}

// ─── Error panel ──────────────────────────────────────────────────────────────

interface ErrorPanelProps {
  message: string
  onRetry: () => void
}

function ErrorPanel({ message, onRetry }: ErrorPanelProps) {
  return (
    <div
      role="alert"
      className="border-b border-[var(--hairline,var(--ink-100))] pb-10"
    >
      <p className="text-sm text-[var(--ink-700)]">{message}</p>
      <button
        type="button"
        onClick={onRetry}
        className="mt-4 inline-flex h-10 items-center rounded-md border border-[var(--hairline,var(--ink-100))] bg-[var(--background-elevated)] px-5 text-sm font-medium text-[var(--ink-900)] transition-colors hover:bg-[var(--paper-200)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--moss-700)]"
      >
        {copy.retryLabel}
      </button>
    </div>
  )
}

// ─── Main client ──────────────────────────────────────────────────────────────

/**
 * BillingClient — renders all billing panels with React Query.
 *
 * Loading: hairline-bordered skeleton blocks.
 * SubscriptionInactiveError: editorial status explainer (billing page still
 *   shows plan data from the query, the interceptor has already redirected).
 * ApiError: inline error panel with retry.
 */
export function BillingClient({ storeId }: BillingClientProps) {
  const { data: plan, isLoading, error, refetch } = useCurrentPlan(storeId)
  const { toast } = useToast()

  const isNotFound = error instanceof ApiError && error.status === 404

  // Fire the ApiError toast as a side-effect, not during render. Calling
  // toast.error() inline on every render triggered an infinite re-render loop
  // because the toast store update re-rendered this component. 404 gets its
  // own panel below rather than a toast.
  useEffect(() => {
    if (error instanceof ApiError && error.status !== 404) {
      toast.error(error.message ?? copy.loadingError)
    }
  }, [error, toast])

  if (isLoading) {
    return (
      <div className="space-y-10">
        <PanelSkeleton />
        <PanelSkeleton />
        <PanelSkeleton />
      </div>
    )
  }

  if (error) {
    // SubscriptionInactiveError: the apiClient interceptor already redirected.
    // Show an editorial explainer while the redirect happens.
    if (error instanceof SubscriptionInactiveError) {
      const status = error.status
      return (
        <div className="space-y-10">
          <p className="text-sm text-[var(--ink-700)]">
            {copy.inactiveBanner(status)}
          </p>
        </div>
      )
    }

    // 404: no subscription row for this store.
    //
    // This used to be an expected state with a "Set up billing" button, back
    // when nothing created the row at signup and the merchant had to press it
    // themselves — which is also what made the 90-day trial start on that
    // press (#827). Onboarding now creates the row, so a 404 here is OUR bug,
    // not a step the merchant has left undone, and asking them to run our
    // data migration would misreport whose problem it is.
    if (isNotFound) {
      return (
        <div className="space-y-10">
          <ErrorPanel message={copy.missingSubscription} onRetry={() => void refetch()} />
        </div>
      )
    }

    // ApiError: inline panel with retry. Toast is fired in the effect above.
    if (error instanceof ApiError) {
      return (
        <div className="space-y-10">
          <ErrorPanel
            message={copy.loadingError}
            onRetry={() => void refetch()}
          />
        </div>
      )
    }

    // Unknown error — generic panel.
    return (
      <div className="space-y-10">
        <ErrorPanel
          message={copy.loadingError}
          onRetry={() => void refetch()}
        />
      </div>
    )
  }

  if (!plan) {
    return null
  }

  return (
    <div className="space-y-10">
      <PlanCard plan={plan} storeId={storeId} />
      <PaymentMethodCard plan={plan} storeId={storeId} />
      {/* Above the invoice history: a merchant arriving from the day-30
          win-back email (#727) came here to redeem a code, and the panel
          they came for should not be below a table. */}
      <PromoCodeCard plan={plan} storeId={storeId} />
      <InvoicesList storeId={storeId} />
      <WhiteLabelAppCard plan={plan} storeId={storeId} />
    </div>
  )
}
