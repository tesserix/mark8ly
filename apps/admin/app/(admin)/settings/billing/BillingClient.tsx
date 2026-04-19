'use client'

import { Skeleton } from '@tesserix/web'
import { useToast } from '@/components/feedback/Toaster'
import { ApiError, SubscriptionInactiveError } from '@/lib/api/client'
import { useCurrentPlan } from '@/lib/api/subscription/hooks/useBilling'
import { subscriptionCopy } from '@/lib/copy/subscription'
import { PlanCard } from './PlanCard'
import { InvoicesList } from './InvoicesList'
import { PaymentMethodCard } from './PaymentMethodCard'
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
      <Skeleton className="h-7 w-40 rounded-[6px]" />
      <Skeleton className="h-4 w-72 rounded-[6px]" />
      <Skeleton className="h-4 w-52 rounded-[6px]" />
      <Skeleton className="h-10 w-32 rounded-[6px]" />
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
        className="mt-4 inline-flex h-10 items-center rounded-[6px] border border-[var(--hairline,var(--ink-100))] bg-[var(--background-elevated)] px-5 text-sm font-medium text-[var(--ink-900)] transition-colors hover:bg-[var(--paper-200)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--moss-700)]"
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

    // ApiError: inline panel with retry + transient toast.
    if (error instanceof ApiError) {
      toast.error(error.message ?? copy.loadingError)
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
      <InvoicesList storeId={storeId} />
      <WhiteLabelAppCard plan={plan} storeId={storeId} />
    </div>
  )
}
