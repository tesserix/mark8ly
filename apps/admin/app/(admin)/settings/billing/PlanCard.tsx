'use client'

import Link from 'next/link'
import { PlanBadge } from '@repo/ui/subscription'
import { SubscriptionStatusBadge } from '@repo/ui/subscription'
import type { SubscriptionStatus } from '@repo/ui/subscription'
import type { CurrentPlan } from '@/lib/api/subscription/schemas/billing'
import { subscriptionCopy } from '@/lib/copy/subscription'
import { formatBillingDate } from '@/lib/format/date'
import { useRevertCancellation } from '@/lib/api/subscription/hooks/useCancellation'
import { useToast } from '@/components/feedback/Toaster'

interface PlanCardProps {
  plan: CurrentPlan
  storeId: string
}

const copy = subscriptionCopy.billing

// Map raw status strings to the typed SubscriptionStatus enum. Unknown
// strings fall back to 'active' so the badge never crashes.
function toStatusBadge(status: string): SubscriptionStatus {
  const known: SubscriptionStatus[] = [
    'signup',
    'trialing',
    'active',
    'past_due',
    'payment_action_required',
    'cancel_scheduled',
    'expired',
    'store_closed',
    'pending_hard_delete',
  ]
  return (known as string[]).includes(status)
    ? (status as SubscriptionStatus)
    : 'active'
}

/**
 * CancelScheduledRow — shown in place of the normal CTA row when
 * status === 'cancel_scheduled'.
 *
 * Editorial voice: calm, factual. Not "Uh-oh!", not urgency language.
 * Single primary CTA: "Restore subscription".
 */
interface CancelScheduledRowProps {
  periodEnd: string | null
  storeId: string
}

function CancelScheduledRow({ periodEnd, storeId }: CancelScheduledRowProps) {
  const { toast } = useToast()
  const revert = useRevertCancellation(storeId)

  const formattedDate = periodEnd ? formatBillingDate(periodEnd) : null
  const noteText = formattedDate
    ? copy.cancelScheduledNote(formattedDate)
    : copy.cancelScheduled

  function handleRestore() {
    revert.mutate(undefined, {
      onSuccess: () => {
        toast.success(copy.toastRestored)
      },
      onError: () => {
        toast.error(copy.toastRestoreError)
      },
    })
  }

  return (
    <div className="mt-6 space-y-4">
      {/* Editorial inline note — screen readers hear this on state change */}
      <p
        aria-live="polite"
        className="text-sm text-[var(--ink-700)]"
      >
        {noteText}
      </p>

      <div className="flex flex-wrap items-center justify-end gap-3">
        <button
          type="button"
          onClick={handleRestore}
          disabled={revert.isPending}
          className="inline-flex h-10 items-center rounded-[6px] bg-[var(--ink-900)] px-5 text-sm font-medium text-white transition-colors hover:bg-[var(--moss-700)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--moss-700)] disabled:cursor-not-allowed disabled:opacity-60"
        >
          {revert.isPending ? 'Restoring\u2026' : copy.restoreCta}
        </button>
      </div>

      {revert.isError && (
        <p
          role="alert"
          className="text-sm text-[var(--danger,#7a1a1a)]"
        >
          {copy.toastRestoreError}
        </p>
      )}
    </div>
  )
}

/**
 * PlanCard — shows the merchant's current plan, subscription status,
 * and renewal date. Hairline-only, no bordered card.
 *
 * Design:
 *   - Source Serif 4 H2 ("Current plan")
 *   - PlanBadge + SubscriptionStatusBadge inline
 *   - Renewal line below badges
 *   - When status === 'cancel_scheduled':
 *       editorial note + "Restore subscription" primary CTA
 *   - Otherwise:
 *       CTA row right-aligned: Change plan (non-Pro) | Contact sales (Pro),
 *       Cancel subscription secondary
 */
export function PlanCard({ plan, storeId }: PlanCardProps) {
  const renewalDate = plan.periodEnd ? formatBillingDate(plan.periodEnd) : null
  const trialDate = plan.trialEndsAt
    ? formatBillingDate(plan.trialEndsAt)
    : null

  const isCancelScheduled = plan.status === 'cancel_scheduled'

  const renewalLine = (() => {
    if (isCancelScheduled && renewalDate) {
      // The cancel-scheduled note is rendered separately inside
      // CancelScheduledRow; suppress the duplicate renewal line.
      return null
    }
    if (trialDate) {
      return copy.trialEnds(trialDate)
    }
    if (renewalDate) {
      return copy.renewsOn(renewalDate)
    }
    return null
  })()

  const isPro = plan.plan === 'pro'

  return (
    <section
      aria-labelledby="plan-card-heading"
      className="border-b border-[var(--hairline,var(--ink-100))] pb-10"
    >
      <h2
        id="plan-card-heading"
        className="font-[family-name:var(--font-source-serif),'Source_Serif_4',serif] text-2xl font-medium tracking-tight text-[var(--ink-900)]"
      >
        {copy.planCardHeading}
      </h2>

      <p className="mt-1 text-sm text-[var(--ink-700)]">
        {copy.planCardDescription}
      </p>

      <div className="mt-5 flex flex-wrap items-center gap-2">
        <PlanBadge
          plan={plan.plan}
          addon={
            plan.addOns.includes('white_label_app')
              ? 'white_label_app'
              : undefined
          }
        />
        <SubscriptionStatusBadge status={toStatusBadge(plan.status)} />
      </div>

      {renewalLine && (
        <p className="mt-3 text-sm text-[var(--ink-700)]">{renewalLine}</p>
      )}

      {isCancelScheduled ? (
        <CancelScheduledRow periodEnd={plan.periodEnd} storeId={storeId} />
      ) : (
        <div className="mt-6 flex flex-wrap items-center justify-end gap-3">
          {isPro ? (
            <Link
              href="/settings/billing/pro-contact"
              className="inline-flex h-10 items-center rounded-[6px] bg-[var(--ink-900)] px-5 text-sm font-medium text-white transition-colors hover:bg-[var(--ink-900)]/90 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--moss-700)]"
            >
              {copy.contactSales}
            </Link>
          ) : (
            <Link
              href="/settings/billing/plan-change"
              className="inline-flex h-10 items-center rounded-[6px] bg-[var(--ink-900)] px-5 text-sm font-medium text-white transition-colors hover:bg-[var(--ink-900)]/90 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--moss-700)]"
            >
              {copy.changePlan}
            </Link>
          )}

          <Link
            href="/settings/billing/cancel"
            className="inline-flex h-10 items-center rounded-[6px] border border-[var(--hairline,var(--ink-100))] bg-[var(--background-elevated)] px-5 text-sm font-medium text-[var(--ink-900)] transition-colors hover:bg-[var(--paper-200)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--moss-700)]"
          >
            {copy.cancelSubscription}
          </Link>
        </div>
      )}
    </section>
  )
}
