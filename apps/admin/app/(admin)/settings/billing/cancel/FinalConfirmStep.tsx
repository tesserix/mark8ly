'use client'

/**
 * FinalConfirmStep — final step (step 4) of the cancellation wizard.
 *
 * Fires useSubmitCancellation on mount with the collected reason + feedback.
 * While pending: hairline "Processing…" placeholder.
 * On success: shows the confirmed period_end date + back-to-billing CTA.
 * On error: inline error message + retry button.
 */
import { useEffect, useRef } from 'react'
import { useRouter } from 'next/navigation'
import { useSubmitCancellation } from '@/lib/api/subscription/hooks/useCancellation'
import { useToast } from '@/components/feedback/Toaster'
import { cancellationCopy } from '@/lib/copy/cancellation'
import { formatBillingDate } from '@/lib/format/date'

const copy = cancellationCopy.finalStep

interface FinalConfirmStepProps {
  storeId: string
  reason: string
  feedback: string
}

export function FinalConfirmStep({ storeId, reason, feedback }: FinalConfirmStepProps) {
  const router = useRouter()
  const { toast } = useToast()
  const submit = useSubmitCancellation(storeId)
  const firedRef = useRef(false)

  useEffect(() => {
    if (firedRef.current) return
    firedRef.current = true

    submit.mutate(
      { survey_reason: reason || undefined, accept_save_offer: false },
      {
        onSuccess: () => {
          toast.success(cancellationCopy.toastCancelled)
        },
        onError: () => {
          // Error is surfaced inline; toast is supplementary.
        },
      },
    )
    // Only fire once on mount — exhaustive-deps is intentionally not satisfied.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  // ── Loading ──────────────────────────────────────────────────────────────
  if (submit.isPending) {
    return (
      <section aria-live="polite" aria-label="Processing cancellation">
        <div className="h-8 w-48 animate-pulse rounded-[6px] bg-[var(--paper-200)]" />
        <div className="mt-3 h-4 w-72 animate-pulse rounded-[6px] bg-[var(--paper-200)]" />
        <p className="sr-only">{cancellationCopy.processing}</p>
      </section>
    )
  }

  // ── Error ────────────────────────────────────────────────────────────────
  if (submit.isError) {
    return (
      <section aria-labelledby="final-error-heading" role="alert">
        <h1
          id="final-error-heading"
          className="mb-4 font-serif text-2xl font-normal text-[var(--ink-900)]"
        >
          Something went wrong
        </h1>

        <p className="mb-6 text-sm text-[var(--ink-700)]">
          {cancellationCopy.toastError}
        </p>

        <button
          type="button"
          onClick={() => {
            firedRef.current = false
            submit.mutate({ survey_reason: reason || undefined, accept_save_offer: false })
          }}
          className="inline-flex h-10 items-center rounded-[6px] bg-[var(--ink-900)] px-6 text-sm font-medium text-[var(--paper-200)] transition-colors hover:bg-[var(--ink-700)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--moss-700)]"
        >
          {cancellationCopy.retryLabel}
        </button>
      </section>
    )
  }

  // ── Success ──────────────────────────────────────────────────────────────
  const periodEndLabel = submit.data?.cancels_at
    ? formatBillingDate(submit.data.cancels_at)
    : ''

  return (
    <section aria-labelledby="final-heading">
      <h1
        id="final-heading"
        className="mb-4 font-serif text-2xl font-normal text-[var(--ink-900)]"
      >
        {copy.title}
      </h1>

      <p className="mb-8 text-sm leading-relaxed text-[var(--ink-700)]">
        {copy.body(periodEndLabel)}
      </p>

      <div className="flex flex-col gap-3 sm:flex-row sm:gap-4">
        <button
          type="button"
          onClick={() => router.push('/settings/billing')}
          className="inline-flex h-10 items-center rounded-[6px] bg-[var(--ink-900)] px-6 text-sm font-medium text-[var(--paper-200)] transition-colors hover:bg-[var(--ink-700)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--moss-700)]"
        >
          {copy.doneCta}
        </button>
      </div>
    </section>
  )
}
