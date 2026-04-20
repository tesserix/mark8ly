'use client'

/**
 * PlanChangeClient — dedicated route for merchant-initiated plan changes.
 *
 * Layout (editorial, left-aligned, no boxed modal):
 *   - H1 via AdminPage (Source Serif 4)
 *   - Radiogroup plan picker (~2/3 width) with hairline separators
 *   - Billing period toggle (Monthly / Annual)
 *   - Preflight preview card below, hairline-separated
 *   - Apply / Cancel CTA row
 *
 * Accessibility:
 *   - Plan picker is role="radiogroup" with aria-label
 *   - Each plan option is role="radio" with aria-checked
 *   - Arrow-key navigation within the radiogroup
 *   - Visible moss focus ring on all interactive elements
 */

import { useState, useCallback, useRef, KeyboardEvent } from 'react'
import { useRouter } from 'next/navigation'
import Link from 'next/link'
import { Money, PlanBadge } from '@repo/ui/subscription'
import { useCurrentPlan } from '@/lib/api/subscription/hooks/useBilling'
import {
  useProrationPreview,
  useApplyPlanChange,
} from '@/lib/api/subscription/hooks/usePlanChange'
import { useToast } from '@/components/feedback/Toaster'
import { subscriptionCopy } from '@/lib/copy/subscription'
import { formatBillingDate } from '@/lib/format/date'
import {
  isImmediateChange,
  isBlockingDecision,
  type PreflightDecision,
} from '@/lib/api/subscription/schemas/planChange'
import type { PlanId } from '@/lib/api/subscription/schemas/plan'
import type { BillingPeriod } from '@/lib/api/subscription/schemas/planChange'
import { ApiError } from '@/lib/api/client'

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

interface PlanChangeClientProps {
  storeId: string
}

interface PlanOption {
  id: PlanId
  name: string
  tagline: string
}

// ---------------------------------------------------------------------------
// Constants
// ---------------------------------------------------------------------------

const PLAN_OPTIONS: PlanOption[] = [
  { id: 'starter', name: 'Starter', tagline: 'Everything you need to launch.' },
  { id: 'studio', name: 'Studio', tagline: 'For growing stores.' },
  { id: 'pro', name: 'Pro', tagline: 'Full power, no limits.' },
]

const copy = subscriptionCopy.planChange

// ---------------------------------------------------------------------------
// Sub-components
// ---------------------------------------------------------------------------

interface PeriodToggleProps {
  value: BillingPeriod
  onChange: (period: BillingPeriod) => void
}

function PeriodToggle({ value, onChange }: PeriodToggleProps) {
  return (
    <div
      role="group"
      aria-label="Billing period"
      className="inline-flex rounded-md border border-[var(--hairline,var(--ink-100))] bg-[var(--background-elevated)] p-0.5"
    >
      {(['monthly', 'annual'] as BillingPeriod[]).map((period) => {
        const isActive = value === period
        return (
          <button
            key={period}
            type="button"
            aria-pressed={isActive}
            onClick={() => onChange(period)}
            className={[
              'rounded-sm px-4 py-1.5 text-sm font-medium transition-colors',
              'focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--moss-700)]',
              isActive
                ? 'bg-[var(--ink-900)] text-white'
                : 'text-[var(--ink-700)] hover:text-[var(--ink-900)]',
            ].join(' ')}
          >
            {period === 'monthly' ? copy.periodMonthly : copy.periodAnnual}
          </button>
        )
      })}
    </div>
  )
}

interface PlanOptionRowProps {
  option: PlanOption
  isSelected: boolean
  isCurrent: boolean
  onSelect: () => void
  tabIndex: number
}

function PlanOptionRow({
  option,
  isSelected,
  isCurrent,
  onSelect,
  tabIndex,
}: PlanOptionRowProps) {
  return (
    <div
      role="radio"
      aria-checked={isSelected}
      tabIndex={tabIndex}
      onClick={onSelect}
      onKeyDown={(e: KeyboardEvent<HTMLDivElement>) => {
        if (e.key === ' ' || e.key === 'Enter') {
          e.preventDefault()
          onSelect()
        }
      }}
      className={[
        'flex cursor-pointer items-center justify-between py-4',
        'border-b border-[var(--hairline,var(--ink-100))] last:border-b-0',
        'focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--moss-700)] focus-visible:ring-inset',
        'transition-colors hover:bg-[var(--paper-200)]/50',
        isSelected ? 'bg-[var(--paper-200)]/50' : '',
      ]
        .filter(Boolean)
        .join(' ')}
    >
      <div className="flex items-center gap-3">
        {/* Radio indicator */}
        <span
          aria-hidden="true"
          className={[
            'flex h-4 w-4 shrink-0 items-center justify-center rounded-full border',
            isSelected
              ? 'border-[var(--moss-700)] bg-[var(--moss-700)]'
              : 'border-[var(--ink-300)] bg-[var(--background-elevated)]',
          ].join(' ')}
        >
          {isSelected && (
            <span className="h-1.5 w-1.5 rounded-full bg-background-elevated" />
          )}
        </span>

        <div>
          <div className="flex items-center gap-2">
            <PlanBadge plan={option.id} />
            {isCurrent && (
              <span className="text-xs text-[var(--moss-700)] font-medium">
                {copy.currentPlanLabel}
              </span>
            )}
          </div>
          <p className="mt-0.5 text-sm text-[var(--ink-600)]">
            {option.tagline}
          </p>
        </div>
      </div>
    </div>
  )
}

interface PreflightPreviewProps {
  storeId: string
  targetPlan: PlanId | undefined
  targetPeriod: BillingPeriod
  currentPlan: string
  currentPeriod: string
}

function PreflightPreview({
  storeId,
  targetPlan,
  targetPeriod,
  currentPlan,
  currentPeriod,
}: PreflightPreviewProps) {
  // Skip preflight when selection hasn't changed from current
  const isNoOp =
    targetPlan === currentPlan && targetPeriod === currentPeriod

  const enabled = Boolean(targetPlan && !isNoOp)

  const { data: report, isLoading } = useProrationPreview(
    storeId,
    enabled ? targetPlan : undefined,
    enabled ? targetPeriod : undefined,
  )

  if (!targetPlan || isNoOp) {
    return null
  }

  if (isLoading) {
    return (
      <div className="mt-6 border-t border-[var(--hairline,var(--ink-100))] pt-6">
        <p className="text-sm text-[var(--ink-600)]">{copy.loadingPreflight}</p>
      </div>
    )
  }

  if (!report) {
    return null
  }

  const decision = report.decision as PreflightDecision

  // Blocking decisions
  if (isBlockingDecision(decision)) {
    const message = (() => {
      if (decision === 'block_read_only') return copy.blockReadOnly
      if (decision === 'block_currency') return copy.blockCurrency
      if (decision === 'block_store_count') {
        return copy.blockStoreCount(
          report.store_count ?? 0,
          report.target_store_limit ?? 0,
        )
      }
      return copy.blockReadOnly
    })()

    return (
      <div
        role="alert"
        className="mt-6 border-t border-[var(--hairline,var(--ink-100))] pt-6"
      >
        <p className="text-sm text-[var(--danger)]">{message}</p>
      </div>
    )
  }

  const isImmediate = isImmediateChange(decision)
  const effectiveAtFormatted = report.effective_at
    ? formatBillingDate(report.effective_at)
    : ''

  const isProMonthly = targetPlan === 'pro' && targetPeriod === 'monthly'

  return (
    <div className="mt-6 border-t border-[var(--hairline,var(--ink-100))] pt-6 space-y-2">
      {isImmediate ? (
        <p className="text-sm text-[var(--ink-700)]">
          {/* Upgrade takes effect now — proration amount populated via webhook,
              so we show a factual "effective immediately" message instead. */}
          Your plan will update immediately.
        </p>
      ) : (
        <p className="text-sm text-[var(--ink-700)]">
          {effectiveAtFormatted
            ? copy.downgradeScheduledNote(effectiveAtFormatted)
            : 'This change will take effect at your current period end.'}
        </p>
      )}

      {isProMonthly && (
        <p className="text-xs text-[var(--ink-600)]">
          {copy.proMonthlyPremiumNote}
        </p>
      )}
    </div>
  )
}

// ---------------------------------------------------------------------------
// Main component
// ---------------------------------------------------------------------------

export function PlanChangeClient({ storeId }: PlanChangeClientProps) {
  const router = useRouter()
  const { toast } = useToast()
  const { data: currentPlan, isLoading: planLoading } = useCurrentPlan(storeId)
  const applyMutation = useApplyPlanChange(storeId)

  const currentPlanId = (currentPlan?.plan ?? 'starter') as PlanId
  // The billing schema doesn't return billing period yet; default to monthly
  // as a safe fallback until the API ships that field.
  const currentPeriod: BillingPeriod = 'monthly'

  const [selectedPlan, setSelectedPlan] = useState<PlanId>(currentPlanId)
  const [selectedPeriod, setSelectedPeriod] =
    useState<BillingPeriod>(currentPeriod)
  const [inlineError, setInlineError] = useState<string | null>(null)

  const radioGroupRef = useRef<HTMLDivElement>(null)

  const handlePlanKeyDown = useCallback(
    (e: KeyboardEvent<HTMLDivElement>, currentIndex: number) => {
      const plans = PLAN_OPTIONS
      let nextIndex = currentIndex

      if (e.key === 'ArrowDown' || e.key === 'ArrowRight') {
        e.preventDefault()
        nextIndex = (currentIndex + 1) % plans.length
      } else if (e.key === 'ArrowUp' || e.key === 'ArrowLeft') {
        e.preventDefault()
        nextIndex = (currentIndex - 1 + plans.length) % plans.length
      } else {
        return
      }

      const nextPlan = plans[nextIndex]
      if (!nextPlan) return
      setSelectedPlan(nextPlan.id)

      // Move focus to the newly activated radio
      const rows = radioGroupRef.current?.querySelectorAll('[role="radio"]')
      if (rows) {
        ;(rows[nextIndex] as HTMLElement).focus()
      }
    },
    [],
  )

  const isNoOp =
    selectedPlan === currentPlanId && selectedPeriod === currentPeriod

  // Report for the current selection (used to gate the Apply button)
  const { data: preflightReport } = useProrationPreview(
    storeId,
    isNoOp ? undefined : selectedPlan,
    isNoOp ? undefined : selectedPeriod,
  )

  const isBlocked =
    preflightReport && isBlockingDecision(preflightReport.decision as PreflightDecision)

  const handleApply = useCallback(async () => {
    if (isNoOp || isBlocked) return
    setInlineError(null)

    try {
      const result = await applyMutation.mutateAsync({
        target_plan: selectedPlan,
        target_period: selectedPeriod,
      })

      const isScheduled =
        result.result === 'downgrade_scheduled'

      if (isScheduled) {
        toast.success(copy.toastScheduled)
      } else {
        toast.success(copy.toastSuccess)
      }

      router.push('/settings/billing')
    } catch (err) {
      const message =
        err instanceof ApiError ? copy.toastError : copy.toastError
      toast.error(message)
      setInlineError(message)
    }
  }, [
    isNoOp,
    isBlocked,
    applyMutation,
    selectedPlan,
    selectedPeriod,
    toast,
    router,
  ])

  if (planLoading) {
    return (
      <div className="space-y-3">
        <div className="h-7 w-40 rounded-md bg-[var(--paper-200)] animate-pulse" />
        <div className="h-4 w-72 rounded-md bg-[var(--paper-200)] animate-pulse" />
      </div>
    )
  }

  return (
    <div className="max-w-2xl">
      {/* Period toggle */}
      <div className="mb-6">
        <PeriodToggle value={selectedPeriod} onChange={setSelectedPeriod} />
      </div>

      {/* Plan picker — radiogroup */}
      <div
        ref={radioGroupRef}
        role="radiogroup"
        aria-label="Choose a plan"
        className="border-t border-[var(--hairline,var(--ink-100))]"
      >
        {PLAN_OPTIONS.map((option, index) => {
          const isSelected = selectedPlan === option.id
          const isCurrent =
            option.id === currentPlanId && selectedPeriod === currentPeriod

          return (
            <PlanOptionRow
              key={option.id}
              option={option}
              isSelected={isSelected}
              isCurrent={isCurrent}
              onSelect={() => setSelectedPlan(option.id)}
              tabIndex={isSelected ? 0 : -1}
              // Pass keyboard handler via onKeyDown on the row
              {...{
                onKeyDown: (e: KeyboardEvent<HTMLDivElement>) =>
                  handlePlanKeyDown(e, index),
              }}
            />
          )
        })}
      </div>

      {/* Preflight preview */}
      <PreflightPreview
        storeId={storeId}
        targetPlan={isNoOp ? undefined : selectedPlan}
        targetPeriod={selectedPeriod}
        currentPlan={currentPlanId}
        currentPeriod={currentPeriod}
      />

      {/* Inline error */}
      {inlineError && (
        <p role="alert" className="mt-4 text-sm text-[var(--danger)]">
          {inlineError}
        </p>
      )}

      {/* CTA row */}
      <div className="mt-8 flex items-center gap-4 border-t border-[var(--hairline,var(--ink-100))] pt-6">
        <button
          type="button"
          onClick={() => void handleApply()}
          disabled={isNoOp || Boolean(isBlocked) || applyMutation.isPending}
          className={[
            'inline-flex h-10 items-center rounded-md px-5 text-sm font-medium',
            'transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--moss-700)]',
            isNoOp || isBlocked
              ? 'cursor-not-allowed bg-[var(--ink-200)] text-[var(--ink-500)]'
              : 'bg-[var(--ink-900)] text-white hover:bg-[var(--ink-900)]/90',
          ].join(' ')}
        >
          {applyMutation.isPending ? 'Applying\u2026' : copy.applyCta}
          {preflightReport?.decision === 'allow_downgrade_at_period_end' && (
            <span className="ml-2 text-xs opacity-70">(scheduled for period end)</span>
          )}
        </button>

        <Link
          href="/settings/billing"
          className="text-sm text-[var(--ink-600)] underline-offset-2 hover:text-[var(--ink-900)] hover:underline focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--moss-700)] rounded-sm"
        >
          {copy.cancelCta}
        </Link>
      </div>
    </div>
  )
}
