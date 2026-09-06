'use client'

import { useState } from 'react'
import Link from 'next/link'
import type { CurrentPlan } from '@/lib/api/subscription/schemas/billing'
import { PromoRejectedError } from '@/lib/api/subscription/promo'
import { useApplyPromo } from '@/lib/api/subscription/hooks/usePromo'
import type { ApplyPromoResponse } from '@/lib/api/subscription/schemas/promo'
import {
  promoRejectionMessage,
  promoRejectionSuggestsPlanChange,
} from '@/lib/copy/promoRejection'
import { promoAppliedMessage } from '@/lib/copy/promoApplied'
import { subscriptionCopy } from '@/lib/copy/subscription'

interface PromoCodeCardProps {
  plan: CurrentPlan
  storeId: string
}

const copy = subscriptionCopy.promo

/**
 * Outcome of the last submission. Held locally rather than read off the
 * mutation so a refusal and a transport failure stay distinguishable: React
 * Query reports both as `isError`, and showing "that code is invalid" for a
 * dropped connection would tell the merchant their working code is broken.
 */
type Outcome =
  | { kind: 'applied'; response: ApplyPromoResponse }
  | { kind: 'rejected'; message: string; suggestsPlanChange: boolean }
  | { kind: 'failed'; message: string }

/**
 * PromoCodeCard — the merchant-facing redemption field.
 *
 * Lives on billing settings rather than in the cancel flow. The cancel
 * flow's save offer redeems its code for the merchant automatically (see
 * cancel.applySaveOfferDiscount) and never asks them to type one, so a field
 * there would have nothing to do. Billing settings is also the surface a
 * merchant arriving cold from the day-30 win-back email can reach: their
 * subscription is expired by then, and readonly.DefaultAllowlist lets
 * POST /subscription/* through in every read-only state precisely so they
 * can recover.
 *
 * Design: hairline-only section, matching PlanCard and InvoicesList.
 */
export function PromoCodeCard({ plan, storeId }: PromoCodeCardProps) {
  const [code, setCode] = useState('')
  const [outcome, setOutcome] = useState<Outcome | null>(null)
  const applyPromo = useApplyPromo(storeId)

  function handleSubmit(event: React.FormEvent) {
    event.preventDefault()

    const trimmed = code.trim()
    if (trimmed.length === 0) {
      setOutcome({ kind: 'failed', message: copy.emptyCode })
      return
    }

    setOutcome(null)
    applyPromo.mutate(trimmed, {
      onSuccess: (response) => {
        setCode('')
        setOutcome({ kind: 'applied', response })
      },
      onError: (error) => {
        if (error instanceof PromoRejectedError) {
          setOutcome({
            kind: 'rejected',
            message: promoRejectionMessage(error.reason, { plan: plan.plan }),
            suggestsPlanChange: promoRejectionSuggestsPlanChange(error.reason),
          })
          return
        }
        // Not a refusal: a 401, a read-only 402, a dropped connection. The
        // code was never judged, so say so instead of blaming it.
        setOutcome({ kind: 'failed', message: copy.networkError })
      },
    })
  }

  const isPending = applyPromo.isPending

  return (
    <section
      aria-labelledby="promo-card-heading"
      className="border-b border-[var(--hairline,var(--ink-100))] pb-10"
    >
      <h2
        id="promo-card-heading"
        className="font-serif text-2xl font-medium tracking-tight text-[var(--ink-900)]"
      >
        {copy.heading}
      </h2>

      <p className="mt-1 max-w-xl text-sm leading-6 text-[var(--ink-700)]">
        {copy.description}
      </p>

      <form onSubmit={handleSubmit} className="mt-5 flex flex-wrap items-end gap-3">
        <div className="flex flex-col gap-1.5">
          <label
            htmlFor="promo-code-input"
            className="text-sm font-medium text-[var(--ink-900)]"
          >
            {copy.inputLabel}
          </label>
          <input
            id="promo-code-input"
            name="promo_code"
            type="text"
            autoComplete="off"
            spellCheck={false}
            value={code}
            onChange={(event) => setCode(event.target.value)}
            disabled={isPending}
            placeholder={copy.inputPlaceholder}
            // The outcome region is the accessible description so a screen
            // reader reaching the field again after a refusal hears WHY,
            // not just the label.
            aria-describedby={outcome ? 'promo-code-outcome' : undefined}
            aria-invalid={outcome?.kind === 'rejected' ? true : undefined}
            className="h-10 w-64 rounded-md border border-[var(--hairline,var(--ink-100))] bg-[var(--background-elevated)] px-3 text-sm text-[var(--ink-900)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--moss-700)] disabled:cursor-not-allowed disabled:opacity-60"
          />
        </div>

        <button
          type="submit"
          disabled={isPending}
          aria-busy={isPending}
          className="inline-flex h-10 items-center rounded-md bg-[var(--ink-900)] px-5 text-sm font-medium text-white transition-colors hover:bg-[var(--ink-900)]/90 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--moss-700)] disabled:cursor-not-allowed disabled:opacity-60"
        >
          {isPending ? copy.submitInProgress : copy.submitCta}
        </button>
      </form>

      {/* One live region for every outcome, so a screen reader hears the
          result whichever way it went. role="status" rather than "alert" for
          the success case; a refusal adds role="alert" below. */}
      <div id="promo-code-outcome" aria-live="polite" className="mt-4">
        {outcome?.kind === 'applied' && (
          <p role="status" className="text-sm text-[var(--moss-700)]">
            {promoAppliedMessage(outcome.response)}
          </p>
        )}

        {outcome?.kind === 'rejected' && (
          <div role="alert" className="space-y-2">
            <p className="max-w-xl text-sm leading-6 text-[var(--ink-700)]">
              {outcome.message}
            </p>
            {outcome.suggestsPlanChange && (
              <Link
                href="/settings/billing/plan-change"
                className="inline-flex text-sm font-medium text-[var(--moss-700)] underline underline-offset-4 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--moss-700)]"
              >
                {subscriptionCopy.billing.changePlan}
              </Link>
            )}
          </div>
        )}

        {outcome?.kind === 'failed' && (
          <p role="alert" className="text-sm text-[var(--danger,#7a1a1a)]">
            {outcome.message}
          </p>
        )}
      </div>
    </section>
  )
}
