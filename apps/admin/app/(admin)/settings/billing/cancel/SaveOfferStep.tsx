'use client'

/**
 * SaveOfferStep — save-offer step (step 3, conditional).
 *
 * Shown only when the merchant's reason is in SAVE_OFFER_ELIGIBLE_REASONS
 * (§15.1: too_expensive, missing_features, technical_issues, other).
 * Skipped for going_out_of_business and switching_to.
 *
 * Copy is verbatim from spec §15.1 / plan Task 10 Step 1 — do not paraphrase.
 *
 * Accept → calls useRevertCancellation, shows restored toast, redirects to billing.
 * Decline → calls onDecline, wizard advances to FinalConfirmStep.
 */
import { useRouter } from 'next/navigation'
import { useRevertCancellation } from '@/lib/api/subscription/hooks/useCancellation'
import { useToast } from '@/components/feedback/Toaster'
import { cancellationCopy } from '@/lib/copy/cancellation'

const copy = cancellationCopy.saveOfferStep
const toastReverted = cancellationCopy.toastReverted
const toastError = cancellationCopy.toastError

interface SaveOfferStepProps {
  storeId: string
  onDecline: () => void
}

export function SaveOfferStep({ storeId, onDecline }: SaveOfferStepProps) {
  const router = useRouter()
  const { toast } = useToast()
  const revert = useRevertCancellation(storeId)

  function handleAccept() {
    revert.mutate(undefined, {
      onSuccess: () => {
        toast.success(toastReverted)
        router.push('/settings/billing')
      },
      onError: () => {
        toast.error(toastError)
      },
    })
  }

  return (
    <section aria-labelledby="save-offer-heading">
      <h1
        id="save-offer-heading"
        className="mb-4 font-serif text-2xl font-normal text-[var(--ink-900)]"
      >
        {copy.title}
      </h1>

      <p className="mb-8 text-sm leading-relaxed text-[var(--ink-700)]">
        {copy.body}
      </p>

      <div
        className="mb-8 rounded-md border border-[var(--hairline,var(--ink-100))] bg-[var(--background-elevated)] p-5"
      >
        <p className="text-sm font-medium text-[var(--ink-900)]">
          50% off your next 3 months
        </p>
        <p className="mt-1 text-sm text-[var(--ink-700)]">
          Your plan continues at half price for the next three billing cycles,
          then returns to the standard rate.
        </p>
      </div>

      <div className="flex flex-col gap-3 sm:flex-row sm:gap-4">
        <button
          type="button"
          onClick={handleAccept}
          disabled={revert.isPending}
          className="inline-flex h-10 items-center rounded-md bg-[var(--moss-700)] px-6 text-sm font-medium text-white transition-colors hover:bg-[var(--moss-800,var(--moss-700))] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--moss-700)] disabled:opacity-50"
        >
          {revert.isPending ? cancellationCopy.processing : copy.acceptCta}
        </button>

        <button
          type="button"
          onClick={onDecline}
          disabled={revert.isPending}
          className="inline-flex h-10 items-center rounded-md border border-[var(--hairline,var(--ink-100))] bg-[var(--background-elevated)] px-6 text-sm font-medium text-[var(--ink-900)] transition-colors hover:bg-[var(--paper-200)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--moss-700)] disabled:opacity-50"
        >
          {copy.declineCta}
        </button>
      </div>
    </section>
  )
}
