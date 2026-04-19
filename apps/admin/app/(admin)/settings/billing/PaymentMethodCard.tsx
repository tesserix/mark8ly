'use client'

import { useToast } from '@/components/feedback/Toaster'
import { useOpenPortal } from '@/lib/api/subscription/hooks/useBilling'
import type { CurrentPlan } from '@/lib/api/subscription/schemas/billing'
import { subscriptionCopy } from '@/lib/copy/subscription'
import { formatBillingDate } from '@/lib/format/date'

interface PaymentMethodCardProps {
  plan: CurrentPlan
  storeId: string
}

const copy = subscriptionCopy.billing

/**
 * PaymentMethodCard — shows the card on file and provides a portal CTA
 * to update it.
 *
 * When no payment method is on file: editorial "No payment method" line + Add CTA.
 * Design: hairline at bottom only, no bordered card.
 */
export function PaymentMethodCard({ plan, storeId }: PaymentMethodCardProps) {
  const openPortal = useOpenPortal(storeId)
  const { toast } = useToast()

  const hasBrand = Boolean(plan.paymentMethodBrand)
  const hasLast4 = Boolean(plan.paymentMethodLast4)
  const hasCard = hasBrand && hasLast4

  const renewalDate = plan.periodEnd ? formatBillingDate(plan.periodEnd) : null

  function handleOpenPortal() {
    openPortal.mutate(undefined, {
      onError: (err) => {
        toast.error(err.message ?? copy.loadingError)
      },
    })
  }

  return (
    <section
      aria-labelledby="payment-method-heading"
      className="border-b border-[var(--hairline,var(--ink-100))] pb-10"
    >
      <h2
        id="payment-method-heading"
        className="font-[family-name:var(--font-source-serif),'Source_Serif_4',serif] text-2xl font-medium tracking-tight text-[var(--ink-900)]"
      >
        {copy.paymentMethodHeading}
      </h2>

      <div className="mt-4 space-y-1">
        {hasCard ? (
          <>
            <p className="text-sm text-[var(--ink-900)]">
              {copy.cardEnding(plan.paymentMethodBrand!, plan.paymentMethodLast4!)}
            </p>
            {renewalDate && (
              <p className="text-sm text-[var(--ink-700)]">
                {copy.renewsAutomatically(renewalDate)}
              </p>
            )}
          </>
        ) : (
          <p className="text-sm text-[var(--ink-700)]">
            {copy.noPaymentMethod}
          </p>
        )}
      </div>

      <div className="mt-6">
        <button
          type="button"
          onClick={handleOpenPortal}
          disabled={openPortal.isPending}
          className="inline-flex h-10 items-center rounded-[6px] border border-[var(--hairline,var(--ink-100))] bg-[var(--background-elevated)] px-5 text-sm font-medium text-[var(--ink-900)] transition-colors hover:bg-[var(--paper-200)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--moss-700)] disabled:cursor-not-allowed disabled:opacity-50"
          aria-busy={openPortal.isPending}
        >
          {openPortal.isPending
            ? copy.openingPortal
            : hasCard
              ? copy.updateCard
              : copy.addPaymentMethod}
        </button>
      </div>
    </section>
  )
}
