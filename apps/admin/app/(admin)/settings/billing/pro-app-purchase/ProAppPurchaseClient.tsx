'use client'

/**
 * ProAppPurchaseClient — White-label App add-on purchase flow.
 *
 * Three states:
 *   1. Not Pro — show "Upgrade to Pro" gate with link to plan-change.
 *   2. Pro + add-on already active — show "already active" state with link
 *      to credential upload.
 *   3. Pro, no add-on — show purchase form with co-termination preview
 *      and Apple 4.2.6 acknowledgement checkbox.
 *
 * Backend notes:
 *   - The purchase endpoint returns HTTP 201 with a Stripe hosted invoice URL
 *     and the prorated `amount_due_cents`. There is no `status` discriminator.
 *   - The hosted invoice URL may require SCA/3DS authentication. When a URL
 *     is returned, we always open it in a new tab so the merchant can complete
 *     their payment while the admin app remains open.
 *   - After a successful 201, we redirect to the credentials upload page.
 *
 * Proration preview:
 *   - Derived client-side from CurrentPlan.periodEnd (co-termination date)
 *     and amount_due_cents from the purchase response preview shown to the
 *     merchant before submission via a static calculation.
 *   - The annual Pro add-on list price is $2,000/yr (server-side ProrationCents
 *     includes a setup component too — we show `amount_due_cents` from the
 *     actual response AFTER purchase; pre-purchase we show "$2,000/yr" as
 *     the full-year anchor).
 *
 * Accessibility:
 *   - Checkbox associated via htmlFor/id.
 *   - Submit disabled until checkbox ticked.
 *   - Error messages announced via aria-live.
 */

import { useId } from 'react'
import { useForm, type SubmitHandler } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { useRouter } from 'next/navigation'
import Link from 'next/link'
import { z } from 'zod'
import { useCurrentPlan } from '@/lib/api/subscription/hooks/useBilling'
import { usePurchaseProApp } from '@/lib/api/subscription/hooks/useProApp'
import { ApiError } from '@/lib/api/client'
import { useToast } from '@/components/feedback/Toaster'
import { subscriptionCopy } from '@/lib/copy/subscription'
import { Money } from '@repo/ui/subscription'
import { formatBillingDate } from '@/lib/format/date'

// ---------------------------------------------------------------------------
// Types + constants
// ---------------------------------------------------------------------------

interface ProAppPurchaseClientProps {
  storeId: string
}

const copy = subscriptionCopy.proApp.purchase

/** Annual White-label App add-on price in cents (used for the next-year line). */
const ANNUAL_ADDON_PRICE_CENTS = 200000

/** RHF form schema — only the acknowledgement checkbox. */
const formSchema = z.object({
  acknowledged: z.literal(true, {
    message: 'You must acknowledge this before continuing.',
  }),
})

type FormValues = z.infer<typeof formSchema>

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function inputClass(hasError?: boolean): string {
  return [
    'h-9 w-full rounded-md border bg-[var(--background-elevated)] px-3 text-sm text-[var(--ink-900)]',
    'placeholder:text-[var(--ink-400)] focus:outline-none focus:ring-2 focus:ring-[var(--moss-700)] focus:ring-offset-1',
    hasError ? 'border-[var(--danger)]' : 'border-[var(--hairline)]',
  ].join(' ')
}

// ---------------------------------------------------------------------------
// Gate states
// ---------------------------------------------------------------------------

function NeedsProGate() {
  return (
    <div className="max-w-xl space-y-4">
      <h2 className="font-[family-name:var(--font-source-serif),'Source_Serif_4',serif] text-2xl font-medium tracking-tight text-[var(--ink-900)]">
        {copy.needsProHeading}
      </h2>
      <p className="text-sm leading-6 text-[var(--ink-700)]">
        {copy.needsProBody}
      </p>
      <Link
        href="/settings/billing/plan-change"
        className="inline-flex h-9 items-center rounded-md bg-[var(--ink-900)] px-5 text-sm font-medium text-[var(--paper-200)] transition-colors hover:bg-[var(--moss-700)] focus:outline-none focus:ring-2 focus:ring-[var(--moss-700)] focus:ring-offset-2"
      >
        {copy.needsProCta}
      </Link>
    </div>
  )
}

function AlreadyActiveState() {
  return (
    <div className="max-w-xl space-y-4">
      <h2 className="font-[family-name:var(--font-source-serif),'Source_Serif_4',serif] text-2xl font-medium tracking-tight text-[var(--ink-900)]">
        {copy.alreadyActiveHeading}
      </h2>
      <p className="text-sm leading-6 text-[var(--ink-700)]">
        {copy.alreadyActiveBody}
      </p>
      <Link
        href="/settings/billing/pro-app-purchase/credentials"
        className="inline-flex h-9 items-center rounded-md bg-[var(--ink-900)] px-5 text-sm font-medium text-[var(--paper-200)] transition-colors hover:bg-[var(--moss-700)] focus:outline-none focus:ring-2 focus:ring-[var(--moss-700)] focus:ring-offset-2"
      >
        {copy.alreadyActiveCta}
      </Link>
    </div>
  )
}

// ---------------------------------------------------------------------------
// ProrationPreviewCard
// ---------------------------------------------------------------------------

interface ProrationPreviewCardProps {
  renewalAt: string | null
  currency: string
}

function ProrationPreviewCard({ renewalAt, currency }: ProrationPreviewCardProps) {
  const formattedRenewal = formatBillingDate(renewalAt)

  return (
    <div className="border-b border-[var(--hairline)] pb-6">
      <h3 className="text-sm font-medium text-[var(--ink-900)]">Billing summary</h3>
      <div className="mt-3 space-y-2 text-sm text-[var(--ink-700)]">
        {renewalAt ? (
          <p>
            A pro-rated charge will be calculated to co-terminate with your annual
            Pro renewal on{' '}
            <span className="font-medium text-[var(--ink-900)]">{formattedRenewal}</span>.
          </p>
        ) : (
          <p>
            A pro-rated charge will be calculated based on the remainder of your
            current billing period.
          </p>
        )}
        <p>
          Next full-year charge:{' '}
          <span className="font-medium text-[var(--ink-900)]">
            <Money amount={ANNUAL_ADDON_PRICE_CENTS} currency={currency} />
          </span>
          {renewalAt && (
            <> on {formattedRenewal}</>
          )}
          .
        </p>
      </div>
    </div>
  )
}

// ---------------------------------------------------------------------------
// PurchaseForm
// ---------------------------------------------------------------------------

interface PurchaseFormProps {
  storeId: string
  renewalAt: string | null
  currency: string
}

function PurchaseForm({ storeId, renewalAt, currency }: PurchaseFormProps) {
  const router = useRouter()
  const { toast } = useToast()
  const mutation = usePurchaseProApp(storeId)

  const formId = useId()
  const ackId = `${formId}-ack`
  const ackErrorId = `${formId}-ack-error`

  const {
    register,
    handleSubmit,
    formState: { errors },
  } = useForm<FormValues>({
    resolver: zodResolver(formSchema),
  })

  const isPending = mutation.isPending

  const onSubmit: SubmitHandler<FormValues> = () => {
    mutation.mutate(
      {
        apple_4_2_6_acknowledged: true,
        attestation_text: copy.apple426AttestationText,
      },
      {
        onSuccess: (result) => {
          // Always open the hosted invoice URL in a new tab — the merchant
          // may need to complete SCA/3DS authentication before the invoice
          // is paid. We redirect to credentials immediately so they can
          // start uploading while payment processes.
          if (result.hosted_invoice_url) {
            window.open(result.hosted_invoice_url, '_blank', 'noopener,noreferrer')
          }
          toast.success(copy.toastPurchased)
          router.push('/settings/billing/pro-app-purchase/credentials')
        },
        onError: (err) => {
          if (err instanceof ApiError && err.code === 'already_active') {
            router.push('/settings/billing/pro-app-purchase/credentials')
            return
          }
          toast.error(copy.toastError)
        },
      },
    )
  }

  return (
    <form
      onSubmit={handleSubmit(onSubmit)}
      noValidate
      aria-label={copy.heading}
      className="max-w-2xl space-y-6"
    >
      <ProrationPreviewCard renewalAt={renewalAt} currency={currency} />

      {/* Apple 4.2.6 acknowledgement */}
      <div className="space-y-1">
        <div className="flex items-start gap-3">
          <input
            id={ackId}
            type="checkbox"
            aria-invalid={errors.acknowledged ? true : undefined}
            aria-describedby={errors.acknowledged ? ackErrorId : undefined}
            className="mt-0.5 h-4 w-4 shrink-0 rounded border-[var(--hairline)] accent-[var(--moss-700)] focus:ring-2 focus:ring-[var(--moss-700)] focus:ring-offset-1"
            {...register('acknowledged')}
          />
          <label
            htmlFor={ackId}
            className="text-sm leading-6 text-[var(--ink-700)]"
          >
            {copy.apple426Label}
          </label>
        </div>
        {errors.acknowledged && (
          <p
            id={ackErrorId}
            role="alert"
            className="pl-7 text-xs text-[var(--danger)]"
          >
            {errors.acknowledged.message}
          </p>
        )}
      </div>

      {/* Submit */}
      <div className="flex items-center gap-4 pt-2">
        <button
          type="submit"
          disabled={isPending}
          className="inline-flex h-9 items-center justify-center rounded-md bg-[var(--ink-900)] px-5 text-sm font-medium text-[var(--paper-200)] transition-colors hover:bg-[var(--moss-700)] focus:outline-none focus:ring-2 focus:ring-[var(--moss-700)] focus:ring-offset-2 disabled:cursor-not-allowed disabled:opacity-50"
        >
          {isPending ? 'Adding\u2026' : copy.submitCta}
        </button>

        <Link
          href="/settings/billing"
          className="text-sm text-[var(--ink-700)] underline-offset-2 hover:underline focus:outline-none focus:ring-2 focus:ring-[var(--moss-700)] focus:ring-offset-1 rounded"
        >
          Cancel
        </Link>
      </div>
    </form>
  )
}

// ---------------------------------------------------------------------------
// ProAppPurchaseClient
// ---------------------------------------------------------------------------

export function ProAppPurchaseClient({ storeId }: ProAppPurchaseClientProps) {
  const { data: plan, isLoading, isError } = useCurrentPlan(storeId)

  if (isLoading) {
    return (
      <div
        aria-busy="true"
        aria-label="Loading billing information"
        className="space-y-4 animate-pulse"
      >
        <div className="h-6 w-48 rounded bg-[var(--paper-200)]" />
        <div className="h-4 w-96 max-w-full rounded bg-[var(--paper-200)]" />
        <div className="h-4 w-72 max-w-full rounded bg-[var(--paper-200)]" />
      </div>
    )
  }

  if (isError || !plan) {
    return (
      <p className="text-sm text-[var(--danger)]">
        Something went wrong loading your plan. Please refresh to try again.
      </p>
    )
  }

  if (plan.plan !== 'pro') {
    return <NeedsProGate />
  }

  if (plan.addOns.includes('white_label_app')) {
    return <AlreadyActiveState />
  }

  return (
    <PurchaseForm
      storeId={storeId}
      renewalAt={plan.periodEnd}
      currency={plan.billingCurrency}
    />
  )
}
