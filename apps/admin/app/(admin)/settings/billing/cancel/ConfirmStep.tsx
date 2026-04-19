'use client'

/**
 * ConfirmStep — first step of the cancellation wizard.
 *
 * Displays:
 *   - Subscription end date from useCurrentPlan.
 *   - Two CTAs: "Keep my subscription" (back to billing) and "Continue" (advance).
 */
import { useRouter } from 'next/navigation'
import { useCurrentPlan } from '@/lib/api/subscription/hooks/useBilling'
import { cancellationCopy } from '@/lib/copy/cancellation'
import { formatBillingDate } from '@/lib/format/date'

const copy = cancellationCopy.confirmStep

interface ConfirmStepProps {
  storeId: string
  onContinue: () => void
}

export function ConfirmStep({ storeId, onContinue }: ConfirmStepProps) {
  const router = useRouter()
  const { data: plan, isLoading } = useCurrentPlan(storeId)

  const periodEndLabel = plan?.periodEnd
    ? formatBillingDate(plan.periodEnd)
    : isLoading
      ? '\u2026'
      : 'the end of your billing period'

  const bodyText = copy.body.replace('{periodEnd}', periodEndLabel)

  return (
    <section aria-labelledby="confirm-heading">
      <h1
        id="confirm-heading"
        className="mb-4 font-serif text-2xl font-normal text-[var(--ink-900)]"
      >
        {copy.title}
      </h1>

      <p className="mb-8 text-sm leading-relaxed text-[var(--ink-700)]">
        {bodyText}
      </p>

      <div className="flex flex-col gap-3 sm:flex-row sm:gap-4">
        <button
          type="button"
          onClick={onContinue}
          className="inline-flex h-10 items-center rounded-[6px] bg-[var(--ink-900)] px-6 text-sm font-medium text-[var(--paper-200)] transition-colors hover:bg-[var(--ink-700)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--moss-700)]"
        >
          {copy.continueCta}
        </button>

        <button
          type="button"
          onClick={() => router.push('/settings/billing')}
          className="inline-flex h-10 items-center rounded-[6px] border border-[var(--hairline,var(--ink-100))] bg-[var(--background-elevated)] px-6 text-sm font-medium text-[var(--ink-900)] transition-colors hover:bg-[var(--paper-200)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--moss-700)]"
        >
          {copy.keepCta}
        </button>
      </div>
    </section>
  )
}
