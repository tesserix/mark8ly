/**
 * AppealStatus — horizontal stepper + SLA countdown for an in-progress
 * arbitrage appeal. Full implementation for Task 21.
 *
 * Visibility rules:
 *   - Store not flagged + no prior audit → returns null.
 *   - Store flagged, no appeal submitted → shows "Submit an appeal" CTA.
 *   - Appeal exists → renders stepper + SLA countdown.
 *
 * Stepper steps:
 *   Step 1: Submitted    — always active when appeal exists
 *   Step 2: Under review — active when appeal_status = 'under_review' or beyond
 *   Step 3: Resolved     — active on 'resolved'; shown with danger accent on 'rejected'
 *
 * SLA: 5 business days. Countdown shown while status is 'pending' or 'under_review'.
 */
'use client'

import Link from 'next/link'
import { useArbitrageFlag } from '@/lib/api/subscription/hooks/useArbitrage'
import { businessDaysElapsed } from '@/lib/format/businessDays'
import { subscriptionCopy } from '@/lib/copy/subscription'
import type { AppealStatus as AppealStatusValue } from '@/lib/api/subscription/schemas/billing'

const copy = subscriptionCopy.arbitrage.appeal

const SLA_DAYS = 5

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

interface AppealStatusProps {
  storeId: string
}

type StepState = 'inactive' | 'active' | 'declined'

interface Step {
  label: string
  state: StepState
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function resolveSteps(status: AppealStatusValue): Step[] {
  const submitted: Step = { label: 'Submitted', state: 'active' }

  const underReview: Step = {
    label: 'Under review',
    state:
      status === 'under_review' || status === 'resolved' || status === 'rejected'
        ? 'active'
        : 'inactive',
  }

  let resolved: Step
  if (status === 'resolved') {
    resolved = { label: 'Resolved', state: 'active' }
  } else if (status === 'rejected') {
    resolved = { label: 'Declined', state: 'declined' }
  } else {
    resolved = { label: 'Resolved', state: 'inactive' }
  }

  return [submitted, underReview, resolved]
}

function computeSlaMessage(
  status: AppealStatusValue,
  submittedAt: string | null | undefined,
): string | null {
  if (status === 'resolved' || status === 'rejected') return null

  if (!submittedAt) return null

  const elapsed = businessDaysElapsed(new Date(submittedAt))
  const remaining = Math.max(0, SLA_DAYS - elapsed)

  if (elapsed >= SLA_DAYS) {
    return 'Your review has been pending longer than usual. We\u2019ll follow up shortly.'
  }

  return `We aim to respond within ${SLA_DAYS} business days. ${remaining} day${remaining === 1 ? '' : 's'} remaining.`
}

// ---------------------------------------------------------------------------
// Sub-components
// ---------------------------------------------------------------------------

function StepDot({ state }: { state: StepState }) {
  if (state === 'active') {
    return (
      <span
        className="flex h-3 w-3 flex-shrink-0 rounded-full bg-[var(--moss-700)]"
        aria-hidden="true"
      />
    )
  }
  if (state === 'declined') {
    return (
      <span
        className="flex h-3 w-3 flex-shrink-0 rounded-full bg-[var(--danger,#8B0000)]"
        aria-hidden="true"
      />
    )
  }
  return (
    <span
      className="flex h-3 w-3 flex-shrink-0 rounded-full border border-[var(--ink-300)] bg-transparent"
      aria-hidden="true"
    />
  )
}

function Stepper({ steps }: { steps: Step[] }) {
  return (
    <ol
      aria-label="Appeal progress"
      className="flex items-center gap-0"
    >
      {steps.map((step, idx) => (
        <li key={step.label} className="flex items-center">
          <span className="flex items-center gap-2">
            <StepDot state={step.state} />
            <span
              className={
                step.state === 'active'
                  ? 'text-sm font-medium text-[var(--ink-900)]'
                  : step.state === 'declined'
                  ? 'text-sm font-medium text-[var(--danger,#8B0000)]'
                  : 'text-sm text-[var(--ink-400)]'
              }
            >
              {step.label}
            </span>
          </span>
          {idx < steps.length - 1 && (
            <span
              className="mx-3 h-px w-8 bg-[var(--hairline,var(--ink-100))]"
              aria-hidden="true"
            />
          )}
        </li>
      ))}
    </ol>
  )
}

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

export function AppealStatus({ storeId }: AppealStatusProps) {
  const { flagged, latestAudit, isLoading } = useArbitrageFlag(storeId)

  if (isLoading) return null
  if (!flagged && !latestAudit) return null

  // Store is flagged but no appeal has been submitted yet.
  const appealStatus = latestAudit?.appeal_status ?? null
  const submittedAt = latestAudit?.appeal_submitted_at ?? null

  if (!appealStatus) {
    // Flagged but no appeal in progress — show submit CTA.
    return (
      <div className="space-y-3">
        <p
          className="text-sm text-[var(--ink-700)]"
          style={{ fontFamily: "'Source Sans 3', sans-serif" }}
        >
          {copy.status.pending}
        </p>
        <Link
          href="/settings/arbitrage"
          className="inline-flex h-9 items-center rounded-[6px] bg-[var(--ink-900)] px-4 text-sm font-medium text-white transition-colors hover:bg-[var(--ink-900)]/90 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--moss-700)]"
        >
          Submit an appeal
        </Link>
      </div>
    )
  }

  const steps = resolveSteps(appealStatus)
  const slaMessage = computeSlaMessage(appealStatus, submittedAt)

  return (
    <div
      className="space-y-4"
      data-testid="appeal-status-tracker"
      aria-label="Appeal status"
    >
      <Stepper steps={steps} />

      {slaMessage && (
        <p
          role="status"
          aria-live="polite"
          className="text-sm text-[var(--ink-700)]"
          style={{ fontFamily: "'Source Sans 3', sans-serif" }}
          data-testid="appeal-sla-message"
        >
          {slaMessage}
        </p>
      )}

      {appealStatus === 'rejected' && (
        <p
          className="text-sm text-[var(--ink-700)]"
          style={{ fontFamily: "'Source Sans 3', sans-serif" }}
          data-testid="appeal-rejected-message"
        >
          {copy.status.rejected}
        </p>
      )}

      {appealStatus === 'resolved' && (
        <p
          className="text-sm text-[var(--moss-700)]"
          style={{ fontFamily: "'Source Sans 3', sans-serif" }}
          data-testid="appeal-resolved-message"
        >
          {copy.status.resolved}
        </p>
      )}
    </div>
  )
}
